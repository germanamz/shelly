// Package agent provides a unified agent type that runs a ReAct loop
// (reason + act), supports dynamic delegation to other agents via a registry,
// and can learn procedures from skills.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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

// CompletionResult carries structured completion data from a sub-agent.
// Set by the task_complete tool, read by delegation tools after Run() returns.
type CompletionResult struct {
	Status        string   `json:"status"`                   // "completed" or "failed"
	Summary       string   `json:"summary"`                  // What was done or why it failed.
	FilesModified []string `json:"files_modified,omitempty"` // Files changed.
	TestsRun      []string `json:"tests_run,omitempty"`      // Tests executed.
	Caveats       string   `json:"caveats,omitempty"`        // Known limitations.
}

// EventNotifier is called by orchestration tools to publish sub-agent
// lifecycle events (e.g. "agent_start", "agent_end") to the engine's EventBus.
type EventNotifier func(ctx context.Context, kind string, agentName string, data any)

// EventFunc is called by the agent to publish fine-grained loop events
// (tool_call_start, tool_call_end, message_added).
type EventFunc func(ctx context.Context, kind string, data any)

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
	MaxIterations          int           // ReAct loop limit (0 = unlimited).
	MaxDelegationDepth     int           // Max tree depth for delegation (0 = cannot delegate).
	Skills                 []skill.Skill // Procedures the agent knows.
	Middleware             []Middleware  // Applied around Run().
	Effects                []Effect      // Per-iteration hooks run inside the ReAct loop.
	Context                string        // Project context injected into the system prompt.
	EventNotifier          EventNotifier // Publishes sub-agent lifecycle events.
	Prefix                 string        // Display prefix (emoji + label) for the TUI.
	TaskBoard              TaskBoard     // Optional task board for automatic task lifecycle during delegation.
	ReflectionDir          string        // Directory for failure reflection notes (empty = disabled).
	DisableBehavioralHints bool          // When true, omits the <behavioral_constraints> section from the system prompt.
	EventFunc              EventFunc     // Optional callback for fine-grained loop events (tool calls, message added).
}

// Agent is the unified agent type. It runs a ReAct loop, can delegate to other
// agents via a Registry, and learns procedures from Skills.
type Agent struct {
	name             string
	configName       string // Template/kind name for registry lookups (defaults to name).
	description      string
	instructions     string
	completer        modeladapter.Completer
	chat             *chat.Chat
	toolboxes        []*toolbox.ToolBox
	registry         *Registry
	options          Options
	depth            int
	completionResult *CompletionResult
	completionOnce   sync.Once
}

// New creates an Agent with the given configuration.
func New(name, description, instructions string, completer modeladapter.Completer, opts Options) *Agent {
	return &Agent{
		name:         name,
		configName:   name,
		description:  description,
		instructions: instructions,
		completer:    completer,
		chat:         chat.New(),
		options:      opts,
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
	if a.options.Prefix != "" {
		return a.options.Prefix
	}
	return "🤖"
}

// Chat returns the agent's chat.
func (a *Agent) Chat() *chat.Chat { return a.chat }

// Completer returns the agent's completer.
func (a *Agent) Completer() modeladapter.Completer { return a.completer }

// CompletionResult returns the structured completion data set by the
// task_complete tool, or nil if the agent stopped without calling it.
func (a *Agent) CompletionResult() *CompletionResult { return a.completionResult }

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
	for i := len(a.options.Middleware) - 1; i >= 0; i-- {
		runner = a.options.Middleware[i](runner)
	}

	return runner.Run(ctx)
}

// run is the internal ReAct loop.
func (a *Agent) run(ctx context.Context) (message.Message, error) {
	ctx = agentctx.WithAgentName(ctx, a.name)

	// Ensure system prompt exists (fallback for direct usage without Init).
	a.Init()

	// Collect all toolboxes (user + orchestration).
	toolboxes := a.allToolBoxes()

	// Collect tool declarations from all toolboxes for the completer,
	// deduplicating by name so providers that reject duplicate definitions
	// (e.g. Grok) don't fail when parent toolboxes are injected into children.
	seen := make(map[string]struct{})
	var tools []toolbox.Tool
	for _, tb := range toolboxes {
		for _, t := range tb.Tools() {
			if _, dup := seen[t.Name]; dup {
				continue
			}
			seen[t.Name] = struct{}{}
			tools = append(tools, t)
		}
	}

	// Reset effects that track per-run state so they behave correctly across
	// multiple Run() calls on a long-lived session agent.
	for _, eff := range a.options.Effects {
		if r, ok := eff.(Resetter); ok {
			r.Reset()
		}
	}

	for i := 0; a.options.MaxIterations == 0 || i < a.options.MaxIterations; i++ {
		ic := IterationContext{
			Phase:     PhaseBeforeComplete,
			Iteration: i,
			Chat:      a.chat,
			Completer: a.completer,
			AgentName: a.name,
		}

		if err := a.evalEffects(ctx, ic); err != nil {
			return message.Message{}, err
		}

		reply, err := a.completer.Complete(ctx, a.chat, tools)
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
				results[idx] = callTool(ctx, toolboxes, tc)
				a.emitEvent(ctx, "tool_call_end", ToolCallEventData{ToolName: tc.Name, CallID: tc.ID})
			})
		}

		wg.Wait()

		// Honour context cancellation after all tools have run.
		if err := ctx.Err(); err != nil {
			return message.Message{}, err
		}

		for _, result := range results {
			msg := message.New(a.name, role.Tool, result)
			a.chat.Append(msg)
			a.emitEvent(ctx, "message_added", MessageAddedEventData{Role: string(role.Tool), Message: msg})
		}

		if a.completionResult != nil {
			return reply, nil
		}
	}

	return message.Message{}, ErrMaxIterations
}

// evalEffects runs registered effects for the given phase.
func (a *Agent) evalEffects(ctx context.Context, ic IterationContext) error {
	for _, eff := range a.options.Effects {
		if err := eff.Eval(ctx, ic); err != nil {
			return err
		}
	}

	return nil
}

// allToolBoxes returns the combined set of user toolboxes and orchestration
// toolbox (if a registry is set).
func (a *Agent) allToolBoxes() []*toolbox.ToolBox {
	tbs := make([]*toolbox.ToolBox, len(a.toolboxes))
	copy(tbs, a.toolboxes)

	if a.registry != nil && a.options.MaxDelegationDepth > 0 {
		tbs = append(tbs, orchestrationToolBox(a))
	}

	if a.depth > 0 {
		completionTB := toolbox.New()
		completionTB.Register(taskCompleteTool(a))
		tbs = append(tbs, completionTB)
	}

	return tbs
}

// buildSystemPrompt constructs the system prompt from identity, instructions,
// skills, and registry.
//
// Sections are ordered for prompt-cache friendliness: static content first
// (identity, instructions), semi-static content next (project context, skills),
// and dynamic content last (agent directory). Each section uses XML tags so
// LLMs can attend to boundaries without relying on prose structure.
func (a *Agent) buildSystemPrompt() string {
	var b strings.Builder

	// --- Static content (rarely changes, cacheable prefix) ---

	// Identity.
	b.WriteString("<identity>\n")
	fmt.Fprintf(&b, "You are %s.", a.name)
	if a.description != "" {
		fmt.Fprintf(&b, " %s", a.description)
	}
	b.WriteString("\n</identity>\n")

	// Completion protocol (sub-agents only).
	if a.depth > 0 {
		b.WriteString("\n<completion_protocol>\n")
		b.WriteString("You are a sub-agent executing a delegated task. ")
		b.WriteString("When you finish, you MUST call the task_complete tool with:\n")
		b.WriteString("- status: \"completed\" or \"failed\"\n")
		b.WriteString("- summary: concise description of what was done\n")
		b.WriteString("- files_modified, tests_run, caveats: as applicable\n")
		b.WriteString("Do NOT simply stop responding — always call task_complete.\n")
		b.WriteString("If you sense you are running low on iterations and cannot finish, ")
		b.WriteString("call task_complete with status \"failed\", summarize what was done, ")
		b.WriteString("and describe remaining work in caveats. Write a progress note first.\n")
		b.WriteString("</completion_protocol>\n")
	}

	// Notes protocol (only when notes tools are available).
	if a.hasNotesTools() {
		b.WriteString("\n<notes_protocol>\n")
		b.WriteString("A shared notes system is available for durable cross-agent communication.\n")
		b.WriteString("Notes persist across agent boundaries and context compaction.\n")
		b.WriteString("When you expect context from another agent (plans, task specs, prior results), ")
		b.WriteString("use list_notes and read_note to retrieve it.\n")
		b.WriteString("When you complete significant work, use write_note to document results ")
		b.WriteString("so other agents can pick up where you left off.\n")
		b.WriteString("</notes_protocol>\n")
	}

	// Instructions.
	if a.instructions != "" {
		b.WriteString("\n<instructions>\n")
		b.WriteString(a.instructions)
		b.WriteString("\n</instructions>\n")
	}

	// Behavioral constraints (default on, can be disabled).
	if !a.options.DisableBehavioralHints {
		b.WriteString("\n<behavioral_constraints>\n")
		b.WriteString("- When a file operation fails, verify the path exists before retrying.\n")
		b.WriteString("- After a tool failure, analyze the error and change your approach before retrying. Do not repeat the same action more than twice expecting different results.\n")
		b.WriteString("- If you have made 5+ tool calls without visible progress, stop and reassess your approach.\n")
		b.WriteString("- Read files before editing them. Search before assuming file locations.\n")
		b.WriteString("- When a command errors, read the error message carefully and address the root cause.\n")
		b.WriteString("- Prefer targeted edits over full file rewrites to minimize unintended changes.\n")
		b.WriteString("- Before starting a multi-step task, briefly outline your plan and the order of steps.\n")
		b.WriteString("- When you have multiple tools that could work, prefer the most specific one for the task.\n")
		b.WriteString("- When approaching your iteration limit, prioritize completing the most critical remaining work and write a progress note.\n")
		b.WriteString("</behavioral_constraints>\n")
	}

	// --- Semi-static content (loaded once at startup) ---

	// Project context.
	if a.options.Context != "" {
		b.WriteString("\n<project_context>\n")
		b.WriteString("The following is context about the project you are working in. ")
		b.WriteString("Treat this as your own knowledge — do not say you lack context about the project. ")
		b.WriteString("Use this information to guide your responses and actions.\n\n")
		b.WriteString(a.options.Context)
		b.WriteString("\n</project_context>\n")
	}

	// Skills — split into inline (no description) and on-demand (has description).
	var inline, onDemand []skill.Skill
	for _, s := range a.options.Skills {
		if s.HasDescription() {
			onDemand = append(onDemand, s)
		} else {
			inline = append(inline, s)
		}
	}

	if len(inline) > 0 {
		b.WriteString("\n<skills>\n")
		for _, s := range inline {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n", s.Name, s.Content)
		}
		b.WriteString("</skills>\n")
	}

	if len(onDemand) > 0 {
		b.WriteString("\n<available_skills>\n")
		b.WriteString("Use the load_skill tool to retrieve the full content of a skill when needed.\n")
		for _, s := range onDemand {
			fmt.Fprintf(&b, "- **%s**: %s\n", s.Name, s.Description)
		}
		b.WriteString("</available_skills>\n")
	}

	// --- Dynamic content (changes per session, not cacheable) ---

	// Agent directory from registry.
	if a.registry != nil {
		entries := a.registry.List()
		var others []Entry
		for _, e := range entries {
			if e.Name != a.configName {
				others = append(others, e)
			}
		}

		if len(others) > 0 {
			b.WriteString("\n<available_agents>\n")
			for _, e := range others {
				fmt.Fprintf(&b, "- **%s**: %s\n", e.Name, e.Description)
			}
			b.WriteString("</available_agents>\n")
		}
	}

	return b.String()
}

// emitEvent publishes a fine-grained loop event if EventFunc is set.
func (a *Agent) emitEvent(ctx context.Context, kind string, data any) {
	if a.options.EventFunc != nil {
		a.options.EventFunc(ctx, kind, data)
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

// callTool searches all toolboxes for the named tool and executes it.
func callTool(ctx context.Context, toolboxes []*toolbox.ToolBox, tc content.ToolCall) content.ToolResult {
	for _, tb := range toolboxes {
		if _, ok := tb.Get(tc.Name); ok {
			return tb.Call(ctx, tc)
		}
	}

	return content.ToolResult{
		ToolCallID: tc.ID,
		Content:    fmt.Sprintf("tool not found: %s", tc.Name),
		IsError:    true,
	}
}
