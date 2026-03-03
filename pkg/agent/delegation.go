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
	Prefix  string // Display prefix (e.g. "🤖", "📝").
	Parent  string // Name of the parent agent (empty for top-level).
	Summary string // Completion summary (populated on agent_end events).
}

// orchestrationToolBox builds a ToolBox containing the built-in orchestration
// tools (list_agents, delegate) for the given agent.
func orchestrationToolBox(a *Agent) *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(
		listAgentsTool(a),
		delegateTool(a),
	)

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
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"agent":{"type":"string","description":"Name of the agent"},"task":{"type":"string","description":"The task to delegate"},"context":{"type":"string","description":"Background context for the agent: relevant file contents, decisions, constraints, or any info the agent needs to complete the task without re-exploring."},"task_id":{"type":"string","description":"Optional task board ID. When provided, the task is auto-claimed for the child agent and its status is updated based on the completion result."}},"required":["agent","task","context"]},"description":"List of agent tasks to run concurrently"}},"required":["tasks"]}`),
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

			results := make([]delegateResult, len(di.Tasks))

			var wg sync.WaitGroup

			eventNotifier := a.events.notifier
			eventFunc := a.events.eventFunc
			reflectionDir := a.delegation.reflectionDir
			taskBoard := a.delegation.taskBoard

			for i, t := range di.Tasks {
				wg.Go(func() {
					child, ok := a.registry.Spawn(t.Agent, a.depth+1)
					if !ok {
						results[i] = delegateResult{
							Agent: t.Agent,
							Error: fmt.Sprintf("agent %q not found", t.Agent),
						}
						return
					}

					// Generate a unique instance name: "<configName>-<slug>-<counter>".
					child.name = fmt.Sprintf("%s-%s-%d", t.Agent, taskSlug(t.Task), a.registry.NextID(t.Agent))

					child.registry = a.registry
					child.events.notifier = eventNotifier
					child.events.eventFunc = eventFunc
					child.delegation.reflectionDir = reflectionDir
					child.delegation.taskBoard = taskBoard
					prependContext(child, t.Context)

					if reflections := searchReflections(reflectionDir, t.Task); reflections != "" {
						child.chat.Append(message.NewText("user", role.User, reflections))
					}

					child.chat.Append(message.NewText("user", role.User, t.Task))

					// Auto-claim task if task_id is provided and TaskBoard is available.
					if t.TaskID != "" && taskBoard != nil {
						if claimErr := taskBoard.ClaimTask(t.TaskID, child.name); claimErr != nil {
							results[i] = delegateResult{
								Agent: t.Agent,
								Error: fmt.Sprintf("failed to claim task %q: %v", t.TaskID, claimErr),
							}
							return
						}
					}

					if eventNotifier != nil {
						eventNotifier(ctx, "agent_start", child.name, AgentEventData{Prefix: child.Prefix(), Parent: a.name})
					}

					reply, err := child.Run(ctx)

					if eventNotifier != nil {
						endData := AgentEventData{Prefix: child.Prefix(), Parent: a.name}
						if cr := child.CompletionResult(); cr != nil && cr.Summary != "" {
							endData.Summary = cr.Summary
						} else {
							endData.Summary = reply.TextContent()
						}
						eventNotifier(ctx, "agent_end", child.name, endData)
					}

					if err != nil {
						if errors.Is(err, ErrMaxIterations) {
							cr := &CompletionResult{
								Status:  "failed",
								Summary: fmt.Sprintf("Agent %q exhausted its iteration limit without completing the task.", t.Agent),
								Caveats: "Iteration limit reached. Check progress notes for partial work.",
							}
							writeReflection(reflectionDir, t.Agent, t.Task, cr)
							dr := delegateResult{Agent: t.Agent, Completion: cr}
							dr.Warning = tryUpdateTask(taskBoard, t.TaskID, cr.Status)
							results[i] = dr
							return
						}
						// Rollback task to "failed" so it doesn't stay stuck in_progress.
						dr := delegateResult{Agent: t.Agent, Error: err.Error()}
						dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "failed")
						results[i] = dr
						return
					}

					// Auto-update task status based on completion result.
					cr := child.CompletionResult()
					if cr != nil {
						if cr.Status == "failed" {
							writeReflection(reflectionDir, t.Agent, t.Task, cr)
						}
						dr := buildDelegateResult(t.Agent, reply, cr)
						dr.Warning = tryUpdateTask(taskBoard, t.TaskID, cr.Status)
						results[i] = dr
					} else {
						// Child finished without calling task_complete — mark as completed
						// since it ran to natural conclusion without error.
						dr := buildDelegateResult(t.Agent, reply, nil)
						dr.Warning = tryUpdateTask(taskBoard, t.TaskID, "completed")
						results[i] = dr
					}
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

// prependContext adds a context message before the task message
// in a child agent's chat. The context is wrapped in <delegation_context> tags.
// If ctx is empty, no message is appended. Context exceeding
// maxDelegateContextRunes is truncated with a suffix.
func prependContext(child *Agent, ctx string) {
	if ctx == "" {
		return
	}
	if utf8.RuneCountInString(ctx) > maxDelegateContextRunes {
		ctx = string([]rune(ctx)[:maxDelegateContextRunes]) + "… [context truncated]"
	}
	child.chat.Append(message.NewText("user", role.User,
		"<delegation_context>\n"+ctx+"\n</delegation_context>"))
}
