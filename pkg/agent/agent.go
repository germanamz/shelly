// Package agent provides a unified agent type that runs a ReAct loop
// (reason + act), supports dynamic delegation to other agents via a registry,
// and can learn procedures from skills.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// Options configures an Agent.
type Options struct {
	MaxIterations      int           // ReAct loop limit (0 = unlimited).
	MaxDelegationDepth int           // Prevents infinite delegation loops (0 = unlimited).
	Skills             []skill.Skill // Procedures the agent knows.
	Middleware         []Middleware  // Applied around Run().
	Effects            []Effect      // Per-iteration hooks run inside the ReAct loop.
	Context            string        // Project context injected into the system prompt.
	EventNotifier      EventNotifier // Publishes sub-agent lifecycle events.
	Prefix             string        // Display prefix (emoji + label) for the TUI.
}

// Agent is the unified agent type. It runs a ReAct loop, can delegate to other
// agents via a Registry, and learns procedures from Skills.
type Agent struct {
	name             string
	description      string
	instructions     string
	completer        modeladapter.Completer
	chat             *chat.Chat
	toolboxes        []*toolbox.ToolBox
	registry         *Registry
	options          Options
	depth            int
	completionResult *CompletionResult
}

// New creates an Agent with the given configuration.
func New(name, description, instructions string, completer modeladapter.Completer, opts Options) *Agent {
	return &Agent{
		name:         name,
		description:  description,
		instructions: instructions,
		completer:    completer,
		chat:         chat.New(),
		options:      opts,
	}
}

// Init builds the system prompt and appends it to the chat. Call this after
// SetRegistry and AddToolBoxes so the prompt includes all available agents.
func (a *Agent) Init() {
	if a.chat.SystemPrompt() == "" {
		a.chat.Append(message.NewText(a.name, role.System, a.buildSystemPrompt()))
	}
}

// Name returns the agent's name.
func (a *Agent) Name() string { return a.name }

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

	// Collect tool declarations from all toolboxes for the completer.
	var tools []toolbox.Tool
	for _, tb := range toolboxes {
		tools = append(tools, tb.Tools()...)
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

		ic.Phase = PhaseAfterComplete
		if err := a.evalEffects(ctx, ic); err != nil {
			return message.Message{}, err
		}

		calls := reply.ToolCalls()
		if len(calls) == 0 {
			return reply, nil
		}

		for _, tc := range calls {
			result := callTool(ctx, toolboxes, tc)
			a.chat.Append(message.New(a.name, role.Tool, result))
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

	if a.registry != nil {
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
		b.WriteString("</completion_protocol>\n")
	}

	// Instructions.
	if a.instructions != "" {
		b.WriteString("\n<instructions>\n")
		b.WriteString(a.instructions)
		b.WriteString("\n</instructions>\n")
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
			if e.Name != a.name {
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
