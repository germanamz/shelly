package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AgentEventData carries metadata about an agent lifecycle event.
type AgentEventData struct {
	Prefix        string // Display prefix (e.g. "🤖", "📝").
	Parent        string // Name of the parent agent (empty for top-level).
	Summary       string // Completion summary (populated on agent_end events).
	ProviderLabel string // Provider display label (e.g. "anthropic/claude-sonnet-4").
	Task          string // Delegation task description (populated on agent_start events).
}

// orchestrationToolBox builds a ToolBox containing the built-in orchestration
// tools (list_agents, delegate) for the given agent.
func orchestrationToolBox(a *Agent) *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(
		listAgentsTool(a),
		delegateTool(a),
	)

	if a.interactiveDelegations != nil {
		tb.Register(answerDelegationQuestionsTool(a))
	}

	return tb
}

// --- list_agents ---

func listAgentsTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "list_agents",
		Description: "List all available agents that can be delegated to",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			entries := a.registry.List()

			// Filter out self using configName (registry key).
			var filtered []Entry
			for _, e := range entries {
				if !strings.EqualFold(e.Name, a.configName) {
					filtered = append(filtered, e)
				}
			}

			data, err := json.Marshal(filtered)
			if err != nil {
				return "", fmt.Errorf("list_agents: %w", err)
			}

			return string(data), nil
		},
	}
}

// --- delegate ---

type delegateTask struct {
	Agent   string `json:"agent"`
	Task    string `json:"task"`
	Context string `json:"context"`
	TaskID  string `json:"task_id"`
	Mode    string `json:"mode"` // "" | "blocking" | "interactive"
}

type delegateInput struct {
	Tasks []delegateTask `json:"tasks"`
}

type delegateResult struct {
	Agent      string            `json:"agent"`
	Result     string            `json:"result,omitempty"`
	Completion *CompletionResult `json:"completion,omitempty"`
	Error      string            `json:"error,omitempty"`
	Warning    string            `json:"warning,omitempty"`
}

func delegateTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "delegate",
		Description: "Delegate tasks to other agents. Accepts one or more tasks; all run concurrently. Use the context field to pass relevant background information so agents do not need to re-explore. Pass task_id on each task to automatically claim and update task board entries.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"agent":{"type":"string","description":"Name of the agent"},"task":{"type":"string","description":"The task to delegate"},"context":{"type":"string","description":"Background context for the agent: relevant file contents, decisions, constraints, or any info the agent needs to complete the task without re-exploring."},"task_id":{"type":"string","description":"Optional task board ID. When provided, the task is auto-claimed for the child agent and its status is updated based on the completion result."},"mode":{"type":"string","enum":["blocking","interactive"],"description":"Delegation mode. 'interactive' returns immediately when children ask questions. Default is blocking."}},"required":["agent","task","context"]},"description":"List of agent tasks to run concurrently"}},"required":["tasks"]}`),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var di delegateInput
			if err := json.Unmarshal(input, &di); err != nil {
				return "", fmt.Errorf("delegate: invalid input: %w", err)
			}

			if len(di.Tasks) == 0 {
				return "[]", nil
			}

			for _, t := range di.Tasks {
				if strings.EqualFold(t.Agent, a.configName) {
					return "", fmt.Errorf("delegate: self-delegation is not allowed")
				}
			}

			if a.depth >= a.delegation.maxDepth {
				return "", fmt.Errorf("delegate: max delegation depth %d reached", a.delegation.maxDepth)
			}

			// Check if any task requests interactive mode.
			hasInteractive := false
			for _, t := range di.Tasks {
				if t.Mode == "interactive" {
					hasInteractive = true
					break
				}
			}

			if hasInteractive {
				return runInteractiveDelegate(ctx, a, di.Tasks)
			}

			results := make([]delegateResult, len(di.Tasks))

			var wg sync.WaitGroup
			for i, t := range di.Tasks {
				wg.Go(func() {
					results[i] = runDelegateTask(ctx, a, t)
				})
			}

			wg.Wait()

			data, err := json.Marshal(results)
			if err != nil {
				return "", fmt.Errorf("delegate: %w", err)
			}

			return string(data), nil
		},
	}
}

// runInteractiveDelegate spawns all children concurrently with InteractionChannels
// wired to the parent's DelegationRegistry shared queue. It returns as soon as
// every child has either completed or asked a question.
func runInteractiveDelegate(ctx context.Context, a *Agent, tasks []delegateTask) (string, error) {
	reg := a.interactiveDelegations
	if reg == nil {
		return "", fmt.Errorf("delegate: interactive mode requires interaction_mode: interactive in config")
	}

	type childInfo struct {
		delegationID string
		task         delegateTask
		questionCh   chan PendingQuestion
		doneCh       chan delegateResult
	}

	children := make([]childInfo, len(tasks))

	// Spawn all children.
	for i, t := range tasks {
		delegationID := reg.NextDelegationID()
		questionCh := make(chan PendingQuestion, 1)
		doneCh := make(chan delegateResult, 1)

		child, err := buildInteractiveDelegateChild(a, t, delegationID, questionCh)
		if err != nil {
			children[i] = childInfo{delegationID: delegationID, task: t, questionCh: questionCh, doneCh: doneCh}
			doneCh <- delegateResult{Agent: t.Agent, Error: err.Error()}
			continue
		}

		childCtx, childCancel := context.WithCancel(ctx)
		pd := &PendingDelegation{
			ID:         delegationID,
			Agent:      t.Agent,
			Task:       t.Task,
			QuestionCh: questionCh,
			AnswerCh:   child.interaction.answerCh,
			DoneCh:     doneCh,
			Cancel:     childCancel,
		}
		if err := reg.Register(pd); err != nil {
			childCancel()
			children[i] = childInfo{delegationID: delegationID, task: t, questionCh: questionCh, doneCh: doneCh}
			doneCh <- delegateResult{Agent: t.Agent, Error: err.Error()}
			continue
		}

		children[i] = childInfo{delegationID: delegationID, task: t, questionCh: questionCh, doneCh: doneCh}

		// Run child in background goroutine. Use runChildWithHandoff directly
		// since buildInteractiveDelegateChild already configured the child.
		go func() {
			defer childCancel()
			if dr := claimTaskOrFail(a.delegation.taskBoard, t.TaskID, child.name, t.Agent); dr != nil {
				doneCh <- *dr
				return
			}
			doneCh <- runChildWithHandoff(childCtx, a, child, t, 0)
		}()
	}

	// Wait for each child to either complete or ask a question.
	results := make([]interactiveDelegateResult, len(tasks))
	var wg sync.WaitGroup
	for i, ci := range children {
		wg.Go(func() {
			results[i] = waitForChildEvent(ctx, reg, ci.delegationID, ci.task.Agent, ci.questionCh, ci.doneCh, a.delegation.questionTimeout)
		})
	}
	wg.Wait()

	data, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("delegate: %w", err)
	}
	return string(data), nil
}

// propagateParentConfig applies the common parent-to-child configuration that
// all delegation paths share: unique instance name, registry, event wiring,
// reflection directory, and task board.
func propagateParentConfig(parent, child *Agent, configName string, t delegateTask) {
	child.name = fmt.Sprintf("%s-%s-%d", configName, taskSlug(t.Task), parent.registry.NextID(configName))
	child.registry = parent.registry
	child.events.notifier = parent.events.notifier
	child.events.eventFunc = delegationProgressFunc(parent.events.eventFunc, parent.events.notifier, child.name, parent.name)
	child.events.cancelRegistrar = parent.events.cancelRegistrar
	child.events.cancelUnregistrar = parent.events.cancelUnregistrar
	child.delegation.reflectionDir = parent.delegation.reflectionDir
	child.delegation.taskBoard = parent.delegation.taskBoard
}

// buildInteractiveDelegateChild creates a child agent wired for interactive
// delegation with a shared question queue.
func buildInteractiveDelegateChild(a *Agent, t delegateTask, delegationID string, questionCh chan PendingQuestion) (*Agent, error) {
	return spawnChild(a, childConfig{
		agentName:   t.Agent,
		task:        t,
		contextMsg:  formatDelegationContext(t.Context),
		interaction: NewSharedInteractionChannel(delegationID, questionCh),
	})
}

// runDelegateTask builds a child agent for the given task, runs it, and returns
// a delegateResult. It handles task board claiming, agent lifecycle events,
// error cases (including iteration exhaustion), result aggregation, and
// peer handoffs (where a child transfers control to a sibling agent).
func runDelegateTask(ctx context.Context, a *Agent, t delegateTask) delegateResult {
	child, err := buildDelegateChild(a, t)
	if err != nil {
		return delegateResult{Agent: t.Agent, Error: err.Error()}
	}

	if dr := claimTaskOrFail(a.delegation.taskBoard, t.TaskID, child.name, t.Agent); dr != nil {
		return *dr
	}

	return runChildWithHandoff(ctx, a, child, t, 0)
}

// maxHandoffDefault is the fallback handoff chain limit when MaxHandoffs is set
// but no explicit cap is configured.
const maxHandoffDefault = 3

// runChildWithHandoff runs a child agent and handles handoff chains. If the
// child calls handoff, a peer agent is spawned and run with the transferred
// context. The handoffCount tracks how many handoffs have occurred to enforce
// the chain limit.
func runChildWithHandoff(ctx context.Context, a *Agent, child *Agent, t delegateTask, handoffCount int) delegateResult {
	taskBoard := a.delegation.taskBoard
	notifier := a.events.notifier

	// Wrap with a cancellable context so the TUI can cancel individual sub-agents.
	childCtx, childCancel := context.WithCancel(ctx)
	defer childCancel()

	// Watch for task cancellation on the board and propagate to child context.
	if t.TaskID != "" && taskBoard != nil {
		if watcher, ok := taskBoard.(TaskCancelWatcher); ok {
			cancelCh := watcher.WatchCanceled(childCtx, t.TaskID)
			go func() {
				select {
				case <-cancelCh:
					childCancel()
				case <-childCtx.Done():
				}
			}()
		}
	}

	if a.events.cancelRegistrar != nil {
		a.events.cancelRegistrar(child.name, childCancel)
		defer func() {
			if a.events.cancelUnregistrar != nil {
				a.events.cancelUnregistrar(child.name)
			}
		}()
	}

	// Start auto-answer goroutine for per-child InteractionChannels only.
	// Shared-queue channels (interactive mode) route questions to the parent.
	if child.interaction != nil && child.interaction.sharedQueue == nil {
		autoAnswer(childCtx, child.interaction, t.Context)
	}

	if notifier != nil {
		notifier(childCtx, "agent_start", child.name, AgentEventData{Prefix: child.Prefix(), Parent: a.name, ProviderLabel: child.ProviderLabel(), Task: t.Task})
	}

	reply, runErr := child.Run(childCtx)

	if notifier != nil {
		endData := AgentEventData{Prefix: child.Prefix(), Parent: a.name, ProviderLabel: child.ProviderLabel()}
		if cr := child.CompletionResult(); cr != nil && cr.Summary != "" {
			endData.Summary = cr.Summary
		} else {
			endData.Summary = reply.TextContent()
		}
		notifier(ctx, "agent_end", child.name, endData)
	}

	notifyResult := func(dr delegateResult) {
		emitDelegationResult(notifier, ctx, child.name, a.name, dr)
	}

	if runErr != nil {
		if errors.Is(runErr, ErrMaxIterations) || errors.Is(runErr, ErrTokenBudgetExhausted) || errors.Is(runErr, ErrTimeBudgetExhausted) {
			summary := fmt.Sprintf("Agent %q exhausted its iteration limit without completing the task.", t.Agent)
			caveats := "Iteration limit reached. Check progress notes for partial work."
			if errors.Is(runErr, ErrTokenBudgetExhausted) {
				summary = fmt.Sprintf("Agent %q exhausted its token budget without completing the task.", t.Agent)
				caveats = "Token budget exhausted. Check progress notes for partial work."
			}
			if errors.Is(runErr, ErrTimeBudgetExhausted) {
				summary = fmt.Sprintf("Agent %q exhausted its time budget without completing the task.", t.Agent)
				caveats = "Time budget exhausted. Check progress notes for partial work."
			}
			cr := &CompletionResult{
				Status:  "failed",
				Summary: summary,
				Caveats: caveats,
			}
			writeReflection(a.delegation.reflectionDir, t.Agent, t.Task, cr)
			dr := delegateResult{Agent: t.Agent, Completion: cr}
			dr.Warning = tryUpdateTask(taskBoard, t.TaskID, cr.Status)
			notifyResult(dr)
			return dr
		}
		// If the child context was canceled due to task cancellation,
		// report it as canceled rather than a generic error.
		if errors.Is(runErr, context.Canceled) && childCtx.Err() != nil && ctx.Err() == nil {
			dr := delegateResult{Agent: t.Agent, Error: "task canceled"}
			dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "canceled")
			notifyResult(dr)
			return dr
		}
		// Rollback task to "failed" so it doesn't stay stuck in_progress.
		dr := delegateResult{Agent: t.Agent, Error: runErr.Error()}
		dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "failed")
		notifyResult(dr)
		return dr
	}

	// Check for handoff — child wants to transfer control to a peer.
	if hr := child.HandoffResult(); hr != nil {
		return handleHandoff(ctx, a, child, t, hr, handoffCount)
	}

	// Auto-update task status based on completion result.
	cr := child.CompletionResult()
	if cr != nil {
		if cr.Status == "failed" {
			writeReflection(a.delegation.reflectionDir, t.Agent, t.Task, cr)
		}
		dr := buildDelegateResult(t.Agent, reply, cr)
		dr.Warning = tryUpdateTask(taskBoard, t.TaskID, cr.Status)
		notifyResult(dr)
		return dr
	}

	// Child finished without calling task_complete — mark as completed
	// since it ran to natural conclusion without error.
	dr := buildDelegateResult(t.Agent, reply, nil)
	dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "completed")
	notifyResult(dr)
	return dr
}

// handleHandoff processes a peer handoff from a child agent. It validates the
// handoff, spawns the peer agent, transfers context, and runs the peer.
func handleHandoff(ctx context.Context, a *Agent, child *Agent, t delegateTask, hr *HandoffResult, handoffCount int) delegateResult {
	notifier := a.events.notifier
	taskBoard := a.delegation.taskBoard

	// Enforce handoff chain limit.
	maxHandoffs := a.delegation.maxHandoffs
	if maxHandoffs <= 0 {
		maxHandoffs = maxHandoffDefault
	}
	if handoffCount >= maxHandoffs {
		dr := delegateResult{
			Agent: t.Agent,
			Error: fmt.Sprintf("handoff chain limit (%d) reached — cannot hand off to %q", maxHandoffs, hr.TargetAgent),
		}
		dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "failed")
		emitDelegationResult(notifier, ctx, child.name, a.name, dr)
		return dr
	}

	// Reject self-handoff (handing off to the same agent type).
	if strings.EqualFold(hr.TargetAgent, child.configName) {
		dr := delegateResult{
			Agent: t.Agent,
			Error: fmt.Sprintf("self-handoff is not allowed: %q cannot hand off to itself", child.configName),
		}
		dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "failed")
		emitDelegationResult(notifier, ctx, child.name, a.name, dr)
		return dr
	}

	// Spawn the peer agent with handoff context and reflections.
	handoffCtxMsg := fmt.Sprintf(
		"<handoff_context>\nThis task was handed off to you by agent %q.\nReason: %s\n\n%s\n</handoff_context>",
		child.configName, hr.Reason, hr.Context,
	)
	peer, err := spawnChild(a, childConfig{
		agentName:  hr.TargetAgent,
		task:       t,
		contextMsg: handoffCtxMsg,
	})
	if err != nil {
		dr := delegateResult{Agent: t.Agent, Error: fmt.Sprintf("handoff: %v", err)}
		dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "failed")
		emitDelegationResult(notifier, ctx, child.name, a.name, dr)
		return dr
	}

	// Re-claim task for the peer if task board is available.
	if dr := claimTaskOrFail(taskBoard, t.TaskID, peer.name, t.Agent); dr != nil {
		emitDelegationResult(notifier, ctx, child.name, a.name, *dr)
		return *dr
	}

	// Run the peer, continuing the handoff chain.
	// Use the handoff context for auto-answer so request_input gets the
	// peer's context instead of the original parent delegation context.
	peerTask := t
	peerTask.Context = hr.Context
	return runChildWithHandoff(ctx, a, peer, peerTask, handoffCount+1)
}

// buildDelegateChild spawns a child agent configured for blocking delegation.
func buildDelegateChild(a *Agent, t delegateTask) (*Agent, error) {
	return spawnChild(a, childConfig{
		agentName:  t.Agent,
		task:       t,
		contextMsg: formatDelegationContext(t.Context),
	})
}

// claimTaskOrFail attempts to claim a task on the board. Returns a non-nil
// delegateResult when the claim fails; returns nil on success or when no claim
// is needed (empty taskID or nil board).
func claimTaskOrFail(tb TaskBoard, taskID, childName, agentName string) *delegateResult {
	if taskID == "" || tb == nil {
		return nil
	}
	if err := tb.ClaimTask(taskID, childName); err != nil {
		return &delegateResult{
			Agent: agentName,
			Error: fmt.Sprintf("failed to claim task %q: %v", taskID, err),
		}
	}
	return nil
}

// tryUpdateTask updates a task's status on the board. Returns a non-empty
// warning string if the update fails; returns "" on success or when no update
// is needed (empty taskID or nil board).
func tryUpdateTask(taskBoard TaskBoard, taskID, status string) string {
	if taskID == "" || taskBoard == nil {
		return ""
	}
	if err := taskBoard.UpdateTaskStatus(taskID, status); err != nil {
		return fmt.Sprintf("task board update failed for %q: %v", taskID, err)
	}
	return ""
}

const maxDelegateResultLen = 2000

// maxDelegateContextRunes caps the delegation context field to prevent
// unbounded context dumps to children (~4000 tokens).
const maxDelegateContextRunes = 16000

// buildDelegateResult constructs a delegateResult from a child agent's reply.
// When a CompletionResult is available, its Summary is used as the primary
// result to keep the parent's context concise. Otherwise, the reply text is
// truncated to maxDelegateResultLen.
func buildDelegateResult(agentName string, reply message.Message, cr *CompletionResult) delegateResult {
	result := delegateResult{
		Agent:      agentName,
		Completion: cr,
	}

	switch {
	case cr != nil && cr.Summary != "":
		result.Result = cr.Summary
	case cr != nil && cr.Status != "":
		parts := []string{cr.Status}
		if cr.Caveats != "" {
			parts = append(parts, cr.Caveats)
		}
		result.Result = strings.Join(parts, ": ")
	default:
		text := reply.TextContent()
		if utf8.RuneCountInString(text) > maxDelegateResultLen {
			text = string([]rune(text)[:maxDelegateResultLen]) + "… [truncated]"
		}
		result.Result = text
	}

	return result
}

// formatDelegationContext wraps a delegation context string in XML tags and
// truncates it to maxDelegateContextRunes. Returns "" if ctx is empty.
func formatDelegationContext(ctx string) string {
	if ctx == "" {
		return ""
	}
	if utf8.RuneCountInString(ctx) > maxDelegateContextRunes {
		ctx = string([]rune(ctx)[:maxDelegateContextRunes]) + "… [context truncated]"
	}
	return "<delegation_context>\n" + ctx + "\n</delegation_context>"
}

// childConfig parameterizes child agent creation for delegation and handoff.
type childConfig struct {
	agentName   string              // Registry name of the agent to spawn.
	task        delegateTask        // Delegation task (used for instance naming, task message, reflections).
	contextMsg  string              // Pre-formatted context message (appended as-is; empty = skip).
	interaction *InteractionChannel // Interaction channel to wire; nil = NewInteractionChannel().
}

// spawnChild creates a configured child agent from the registry. It handles
// unique naming, parent config propagation, interaction wiring, context/
// reflection prepending, and task message appending.
func spawnChild(parent *Agent, cfg childConfig) (*Agent, error) {
	child, ok := parent.registry.Spawn(cfg.agentName, parent.depth+1)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", cfg.agentName)
	}

	propagateParentConfig(parent, child, cfg.agentName, cfg.task)

	if cfg.interaction != nil {
		child.interaction = cfg.interaction
	} else {
		child.interaction = NewInteractionChannel()
	}

	if cfg.contextMsg != "" {
		child.chat.Append(message.NewText("user", role.User, cfg.contextMsg))
	}

	if reflections := searchReflections(parent.delegation.reflectionDir, cfg.task.Task); reflections != "" {
		child.chat.Append(message.NewText("user", role.User, reflections))
	}

	child.chat.Append(message.NewText("user", role.User, cfg.task.Task))

	return child, nil
}
