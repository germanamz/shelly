// Package agent provides a unified agent type that runs a ReAct loop
// (reason + act), supports dynamic delegation to other agents via a registry,
// and can learn procedures from skills.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/agentctx"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/skill"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// ErrMaxIterations is returned when the ReAct loop exceeds MaxIterations
// without the model producing a final answer.
var ErrMaxIterations = errors.New("agent: max iterations reached")

// EventNotifier is called by orchestration tools to publish sub-agent
// lifecycle events (e.g. "agent_start", "agent_end") to the engine's EventBus.
type EventNotifier func(ctx context.Context, kind string, agentName string, data any)

// EventFunc is called by the agent to publish fine-grained loop events
// (tool_call_start, tool_call_end, message_added).
type EventFunc func(ctx context.Context, kind string, data any)

// CancelRegistrar registers a context.CancelFunc for a named child agent so
// that the engine (or TUI) can cancel individual sub-agents.
type CancelRegistrar func(name string, cancel context.CancelFunc)

// CancelUnregistrar removes a previously registered cancel function.
type CancelUnregistrar func(name string)

// ToolCallEventData carries metadata for tool_call_start / tool_call_end events.
type ToolCallEventData struct {
	ToolName string `json:"tool_name"`
	CallID   string `json:"call_id"`
}

// MessageAddedEventData carries metadata for message_added events.
type MessageAddedEventData struct {
	Role    string          `json:"role"`
	Message message.Message `json:"message"`
}

// TaskBoard abstracts the shared task board so orchestration tools can
// manage task lifecycle during delegation without importing pkg/tasks.
type TaskBoard interface {
	ClaimTask(id, agent string) error
	UpdateTaskStatus(id, status string) error
}

// Options configures an Agent.
type Options struct {
	MaxIterations          int               // ReAct loop limit (0 = unlimited).
	MaxDelegationDepth     int               // Max tree depth for delegation (0 = cannot delegate).
	Skills                 []skill.Skill     // Procedures the agent knows.
	Middleware             []Middleware      // Applied around Run().
	Effects                []Effect          // Per-iteration hooks run inside the ReAct loop.
	Context                string            // Project context injected into the system prompt.
	EventNotifier          EventNotifier     // Publishes sub-agent lifecycle events.
	Prefix                 string            // Display prefix (emoji + label) for the TUI.
	TaskBoard              TaskBoard         // Optional task board for automatic task lifecycle during delegation.
	ReflectionDir          string            // Directory for failure reflection notes (empty = disabled).
	DisableBehavioralHints bool              // When true, omits the <behavioral_constraints> section from the system prompt.
	EventFunc              EventFunc         // Optional callback for fine-grained loop events (tool calls, message added).
	CancelRegistrar        CancelRegistrar   // Registers child-agent cancel funcs for external cancellation.
	CancelUnregistrar      CancelUnregistrar // Unregisters child-agent cancel funcs.
	ProviderLabel          string            // Display label for the provider (e.g. "anthropic/claude-sonnet-4").
}

// delegationConfig groups fields used by the delegation handler.
type delegationConfig struct {
	maxDepth      int
	reflectionDir string
	taskBoard     TaskBoard
}

// promptConfig groups fields used by the prompt builder.
type promptConfig struct {
	context                string
	skills                 []skill.Skill
	disableBehavioralHints bool
}

// eventConfig groups fields used for event emission.
type eventConfig struct {
	notifier          EventNotifier
	eventFunc         EventFunc
	cancelRegistrar   CancelRegistrar
	cancelUnregistrar CancelUnregistrar
}

// Agent is the unified agent type. It runs a ReAct loop, can delegate to other
// agents via a Registry, and learns procedures from Skills.
type Agent struct {
	name          string
	configName    string // Template/kind name for registry lookups (defaults to name).
	description   string
	instructions  string
	completer     modeladapter.Completer
	chat          *chat.Chat
	toolboxes     []*toolbox.ToolBox
	registry      *Registry
	prefix        string
	providerLabel string
	maxIterations int
	middleware    []Middleware
	effects       []Effect
	delegation    delegationConfig
	prompt        promptConfig
	events        eventConfig
	depth         int
	completion    completionHandler
}

// New creates an Agent with the given configuration.
func New(name, description, instructions string, completer modeladapter.Completer, opts Options) *Agent {
	return &Agent{
		name:          name,
		configName:    name,
		description:   description,
		instructions:  instructions,
		completer:     completer,
		chat:          chat.New(),
		prefix:        opts.Prefix,
		providerLabel: opts.ProviderLabel,
		maxIterations: opts.MaxIterations,
		middleware:    opts.Middleware,
		effects:       opts.Effects,
		delegation: delegationConfig{
			maxDepth:      opts.MaxDelegationDepth,
			reflectionDir: opts.ReflectionDir,
			taskBoard:     opts.TaskBoard,
		},
		prompt: promptConfig{
			context:                opts.Context,
			skills:                 opts.Skills,
			disableBehavioralHints: opts.DisableBehavioralHints,
		},
		events: eventConfig{
			notifier:          opts.EventNotifier,
			eventFunc:         opts.EventFunc,
			cancelRegistrar:   opts.CancelRegistrar,
			cancelUnregistrar: opts.CancelUnregistrar,
		},
	}
}

// Init builds the system prompt and sets it in the chat. Call this after
// SetRegistry and AddToolBoxes so the prompt includes all available agents.
// Safe to call multiple times — the system prompt message is replaced each time.
func (a *Agent) Init() {
	newPrompt := message.NewText(a.name, role.System, a.buildSystemPrompt())

	if a.chat.Len() == 0 {
		a.chat.Append(newPrompt)
		return
	}

	// Replace the existing system prompt message (always at index 0).
	msgs := a.chat.Messages()
	if msgs[0].Role == role.System {
		msgs[0] = newPrompt
	} else {
		msgs = append([]message.Message{newPrompt}, msgs...)
	}
	a.chat.Replace(msgs...)
}

// Name returns the agent's name.
func (a *Agent) Name() string { return a.name }

// ConfigName returns the agent's config/template name used for registry lookups.
// For session agents this equals Name(); for spawned sub-agents it is the
// registry key that was used to create the agent.
func (a *Agent) ConfigName() string { return a.configName }

// Description returns the agent's description.
func (a *Agent) Description() string { return a.description }

// Prefix returns the agent's display prefix, defaulting to "🤖" if unset.
func (a *Agent) Prefix() string {
	if a.prefix != "" {
		return a.prefix
	}
	return "🤖"
}

// ProviderLabel returns the display label for the agent's provider.
func (a *Agent) ProviderLabel() string { return a.providerLabel }

// Chat returns the agent's chat.
func (a *Agent) Chat() *chat.Chat { return a.chat }

// Completer returns the agent's completer.
func (a *Agent) Completer() modeladapter.Completer { return a.completer }

// CompletionResult returns the structured completion data set by the
// task_complete tool, or nil if the agent stopped without calling it.
func (a *Agent) CompletionResult() *CompletionResult { return a.completion.Result() }

// SetRegistry enables dynamic delegation by setting the agent's registry.
func (a *Agent) SetRegistry(r *Registry) {
	a.registry = r
}

// AddToolBoxes adds user-provided toolboxes to the agent, skipping any that
// are already present (pointer equality) to avoid duplicate tools.
func (a *Agent) AddToolBoxes(tbs ...*toolbox.ToolBox) {
	existing := make(map[*toolbox.ToolBox]struct{}, len(a.toolboxes))
	for _, tb := range a.toolboxes {
		existing[tb] = struct{}{}
	}

	for _, tb := range tbs {
		if _, dup := existing[tb]; dup {
			continue
		}
		existing[tb] = struct{}{}
		a.toolboxes = append(a.toolboxes, tb)
	}
}

// Run executes the agent's ReAct loop with middleware applied.
func (a *Agent) Run(ctx context.Context) (message.Message, error) {
	var runner Runner = RunnerFunc(a.run)

	// Apply middleware in reverse order so the first middleware is outermost.
	for i := len(a.middleware) - 1; i >= 0; i-- {
		runner = a.middleware[i](runner)
	}

	return runner.Run(ctx)
}

// maxToolResultRunes caps individual tool results before they enter the chat
// to prevent a single large result from consuming the entire context (~8000 tokens).
const maxToolResultRunes = 32000

// capToolResult truncates oversized tool results. Error results are never
// capped to preserve diagnostic information.
func capToolResult(result content.ToolResult) content.ToolResult {
	if result.IsError {
		return result
	}
	if utf8.RuneCountInString(result.Content) <= maxToolResultRunes {
		return result
	}
	result.Content = string([]rune(result.Content)[:maxToolResultRunes]) +
		"\n… [output capped — use targeted reads for remaining content]"
	return result
}

// run is the internal ReAct loop.
func (a *Agent) run(ctx context.Context) (message.Message, error) {
	ctx = agentctx.WithAgentName(ctx, a.name)

	// Ensure system prompt exists (fallback for direct usage without Init).
	a.Init()

	// Collect all toolboxes (user + orchestration).
	toolboxes := a.allToolBoxes()

	// Collect tool declarations and a handler map from all toolboxes.
	tools, handlers := deduplicateTools(toolboxes)

	// Reset effects that track per-run state so they behave correctly across
	// multiple Run() calls on a long-lived session agent.
	for _, eff := range a.effects {
		if r, ok := eff.(Resetter); ok {
			r.Reset()
		}
	}

	// Cache the static tool token cost so effects can inspect it without
	// recomputing on every iteration.
	var estimator modeladapter.TokenEstimator
	toolTokens := estimator.EstimateTools(tools)

	for i := 0; a.maxIterations == 0 || i < a.maxIterations; i++ {
		estimatedTokens := estimator.EstimateTotal(a.chat, tools)

		ic := IterationContext{
			Phase:           PhaseBeforeComplete,
			Iteration:       i,
			Chat:            a.chat,
			Completer:       a.completer,
			AgentName:       a.name,
			EstimatedTokens: estimatedTokens,
			ToolTokens:      toolTokens,
		}

		if err := a.evalEffects(ctx, ic); err != nil {
			return message.Message{}, err
		}

		iterTools := a.filterTools(ctx, ic, tools)
		reply, err := a.completer.Complete(ctx, a.chat, iterTools)
		if err != nil {
			return message.Message{}, err
		}

		reply.Sender = a.name
		a.chat.Append(reply)
		a.emitEvent(ctx, "message_added", MessageAddedEventData{Role: string(reply.Role), Message: reply})

		ic.Phase = PhaseAfterComplete
		if err := a.evalEffects(ctx, ic); err != nil {
			return message.Message{}, err
		}

		calls := reply.ToolCalls()
		if len(calls) == 0 {
			return reply, nil
		}

		// Execute tool calls concurrently, collecting results in order.
		results := make([]content.ToolResult, len(calls))

		var wg sync.WaitGroup

		for idx, tc := range calls {
			wg.Go(func() {
				a.emitEvent(ctx, "tool_call_start", ToolCallEventData{ToolName: tc.Name, CallID: tc.ID})
				results[idx] = callTool(ctx, handlers, tc)
				a.emitEvent(ctx, "tool_call_end", ToolCallEventData{ToolName: tc.Name, CallID: tc.ID})
			})
		}

		wg.Wait()

		// Honour context cancellation after all tools have run.
		if err := ctx.Err(); err != nil {
			return message.Message{}, err
		}

		for _, result := range results {
			result = capToolResult(result)
			msg := message.New(a.name, role.Tool, result)
			a.chat.Append(msg)
			a.emitEvent(ctx, "message_added", MessageAddedEventData{Role: string(role.Tool), Message: msg})
		}

		if a.completion.IsComplete() {
			return reply, nil
		}
	}

	return message.Message{}, ErrMaxIterations
}

// evalEffects runs registered effects for the given phase.
func (a *Agent) evalEffects(ctx context.Context, ic IterationContext) error {
	for _, eff := range a.effects {
		if err := eff.Eval(ctx, ic); err != nil {
			return err
		}
	}

	return nil
}

// canDelegate returns true when delegation is both configured and the current
// depth still allows it.
func (a *Agent) canDelegate() bool {
	return a.registry != nil && a.delegation.maxDepth > 0 && a.depth < a.delegation.maxDepth
}

// filterTools applies all ToolFilter effects to the tool list, returning the
// filtered set. Filters are applied sequentially (intersection semantics).
func (a *Agent) filterTools(ctx context.Context, ic IterationContext, tools []toolbox.Tool) []toolbox.Tool {
	filtered := tools
	for _, eff := range a.effects {
		if tf, ok := eff.(ToolFilter); ok {
			filtered = tf.FilterTools(ctx, ic, filtered)
		}
	}
	return filtered
}

// allToolBoxes returns the combined set of user toolboxes, orchestration
// toolbox (if delegation is possible), and effect-provided toolboxes.
func (a *Agent) allToolBoxes() []*toolbox.ToolBox {
	tbs := make([]*toolbox.ToolBox, len(a.toolboxes))
	copy(tbs, a.toolboxes)

	if a.canDelegate() {
		tbs = append(tbs, orchestrationToolBox(a))
	}

	if a.depth > 0 {
		completionTB := toolbox.New()
		completionTB.Register(a.completion.tool())
		tbs = append(tbs, completionTB)
	}

	// Collect tools provided by effects (e.g. offload's recall tool).
	for _, eff := range a.effects {
		if tp, ok := eff.(ToolProvider); ok {
			if tb := tp.ProvidedTools(); tb != nil {
				tbs = append(tbs, tb)
			}
		}
	}

	return tbs
}

// buildSystemPrompt constructs the system prompt by delegating to promptBuilder.
func (a *Agent) buildSystemPrompt() string {
	pb := promptBuilder{
		Name:                   a.name,
		Description:            a.description,
		Instructions:           a.instructions,
		Context:                a.prompt.context,
		ConfigName:             a.configName,
		Depth:                  a.depth,
		Skills:                 a.prompt.skills,
		DisableBehavioralHints: a.prompt.disableBehavioralHints,
		HasNotesTools:          a.hasNotesTools(),
		CanDelegate:            a.canDelegate(),
	}

	if pb.CanDelegate {
		pb.RegistryEntries = a.registry.List()
	}

	return pb.build()
}

// emitEvent publishes a fine-grained loop event if EventFunc is set.
func (a *Agent) emitEvent(ctx context.Context, kind string, data any) {
	if a.events.eventFunc != nil {
		a.events.eventFunc(ctx, kind, data)
	}
}

// hasNotesTools returns true if any toolbox contains the "list_notes" tool.
func (a *Agent) hasNotesTools() bool {
	for _, tb := range a.toolboxes {
		if _, ok := tb.Get("list_notes"); ok {
			return true
		}
	}
	return false
}

// callTool looks up the named tool in the pre-built handler map and executes it.
func callTool(ctx context.Context, handlers map[string]toolbox.Handler, tc content.ToolCall) content.ToolResult {
	handler, ok := handlers[tc.Name]
	if !ok {
		return content.ToolResult{
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("tool not found: %s", tc.Name),
			IsError:    true,
		}
	}

	result, err := handler(ctx, json.RawMessage(tc.Arguments))
	if err != nil {
		return content.ToolResult{
			ToolCallID: tc.ID,
			Content:    err.Error(),
			IsError:    true,
		}
	}

	return content.ToolResult{
		ToolCallID: tc.ID,
		Content:    result,
	}
}
