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

// Options configures an Agent.
type Options struct {
	MaxIterations      int           // ReAct loop limit (0 = unlimited).
	MaxDelegationDepth int           // Prevents infinite delegation loops (0 = unlimited).
	Skills             []skill.Skill // Procedures the agent knows.
	Middleware         []Middleware  // Applied around Run().
	Context            string        // Project context injected into the system prompt.
}

// Agent is the unified agent type. It runs a ReAct loop, can delegate to other
// agents via a Registry, and learns procedures from Skills.
type Agent struct {
	name         string
	description  string
	instructions string
	completer    modeladapter.Completer
	chat         *chat.Chat
	toolboxes    []*toolbox.ToolBox
	registry     *Registry
	options      Options
	depth        int
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

// Chat returns the agent's chat.
func (a *Agent) Chat() *chat.Chat { return a.chat }

// Completer returns the agent's completer.
func (a *Agent) Completer() modeladapter.Completer { return a.completer }

// SetRegistry enables dynamic delegation by setting the agent's registry.
func (a *Agent) SetRegistry(r *Registry) {
	a.registry = r
}

// AddToolBoxes adds user-provided toolboxes to the agent.
func (a *Agent) AddToolBoxes(tbs ...*toolbox.ToolBox) {
	a.toolboxes = append(a.toolboxes, tbs...)
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
		reply, err := a.completer.Complete(ctx, a.chat, tools)
		if err != nil {
			return message.Message{}, err
		}

		reply.Sender = a.name
		a.chat.Append(reply)

		calls := reply.ToolCalls()
		if len(calls) == 0 {
			return reply, nil
		}

		for _, tc := range calls {
			result := callTool(ctx, toolboxes, tc)
			a.chat.Append(message.New(a.name, role.Tool, result))
		}
	}

	return message.Message{}, ErrMaxIterations
}

// allToolBoxes returns the combined set of user toolboxes and orchestration
// toolbox (if a registry is set).
func (a *Agent) allToolBoxes() []*toolbox.ToolBox {
	tbs := make([]*toolbox.ToolBox, len(a.toolboxes))
	copy(tbs, a.toolboxes)

	if a.registry != nil {
		tbs = append(tbs, orchestrationToolBox(a))
	}

	return tbs
}

// buildSystemPrompt constructs the system prompt from identity, instructions,
// skills, and registry.
func (a *Agent) buildSystemPrompt() string {
	var b strings.Builder

	// Identity.
	fmt.Fprintf(&b, "You are %s.", a.name)
	if a.description != "" {
		fmt.Fprintf(&b, " %s", a.description)
	}
	b.WriteString("\n")

	// Instructions.
	if a.instructions != "" {
		b.WriteString("\n## Instructions\n\n")
		b.WriteString(a.instructions)
		b.WriteString("\n")
	}

	// Project context.
	if a.options.Context != "" {
		b.WriteString("\n## Project Context\n\n")
		b.WriteString("The following is context about the project you are working in. ")
		b.WriteString("Treat this as your own knowledge — do not say you lack context about the project. ")
		b.WriteString("Use this information to guide your responses and actions.\n\n")
		b.WriteString(a.options.Context)
		b.WriteString("\n")
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
		b.WriteString("\n## Skills\n")
		for _, s := range inline {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n", s.Name, s.Content)
		}
	}

	if len(onDemand) > 0 {
		b.WriteString("\n## Available Skills\n\nUse the load_skill tool to retrieve the full content of a skill when needed.\n")
		for _, s := range onDemand {
			fmt.Fprintf(&b, "- **%s**: %s\n", s.Name, s.Description)
		}
	}

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
			b.WriteString("\n## Available Agents\n\n")
			for _, e := range others {
				fmt.Fprintf(&b, "- **%s**: %s\n", e.Name, e.Description)
			}
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
