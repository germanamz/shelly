// Package handoff enables agents to transfer control to each other
// mid-conversation. A HandoffAgent manages a set of named agents that share a
// single chat. Each agent receives transfer_to_<name> tools; calling one
// switches the active agent. The loop continues until an agent returns a final
// answer without triggering a handoff.
package handoff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/germanamz/shelly/pkg/agents"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/reactor"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Compile-time interface checks.
var (
	_ agents.Agent       = (*HandoffAgent)(nil)
	_ reactor.NamedAgent = (*HandoffAgent)(nil)
)

// ErrMaxHandoffs is returned when the handoff limit is exceeded.
var ErrMaxHandoffs = errors.New("handoff: max handoffs reached")

// ErrNoMembers is returned when a HandoffAgent is created with no members.
var ErrNoMembers = errors.New("handoff: at least one member is required")

// HandoffError signals that the active agent wants to transfer control to
// another agent. It is returned by the transfer tool handler and caught by
// HandoffAgent.Run to switch the active agent.
type HandoffError struct {
	Target string
}

func (e *HandoffError) Error() string {
	return fmt.Sprintf("handoff to %q", e.Target)
}

// AgentFactory builds a NamedAgent given the shared chat and a ToolBox
// containing the transfer tools. This lets HandoffAgent control what chat and
// tools each member agent uses.
type AgentFactory func(shared *chat.Chat, transferTools *toolbox.ToolBox) reactor.NamedAgent

// Options configures a HandoffAgent.
type Options struct {
	// MaxHandoffs limits the number of transfers. Zero means no limit.
	MaxHandoffs int
}

// Member pairs a name with a factory that builds the agent.
type Member struct {
	Name    string
	Factory AgentFactory
}

// HandoffAgent manages a set of agents sharing a single chat. One agent is
// active at a time. Transfer tools allow agents to hand off control to each
// other.
type HandoffAgent struct {
	name    string
	members map[string]reactor.NamedAgent
	active  string
	chat    *chat.Chat
	opts    Options
}

// New creates a HandoffAgent. The first member becomes the initially active
// agent. It returns ErrNoMembers if members is empty.
func New(name string, shared *chat.Chat, members []Member, opts Options) (*HandoffAgent, error) {
	if len(members) == 0 {
		return nil, ErrNoMembers
	}

	h := &HandoffAgent{
		name:    name,
		members: make(map[string]reactor.NamedAgent, len(members)),
		active:  members[0].Name,
		chat:    shared,
		opts:    opts,
	}

	// Build transfer tools containing all member names.
	transferTB := buildTransferToolBox(members)

	// Build each agent using its factory, passing the shared chat and transfer tools.
	for _, m := range members {
		agent := m.Factory(shared, transferTB)
		h.members[m.Name] = agent
	}

	return h, nil
}

// AgentName returns the handoff agent's name.
func (h *HandoffAgent) AgentName() string { return h.name }

// AgentChat returns the shared chat.
func (h *HandoffAgent) AgentChat() *chat.Chat { return h.chat }

// Run executes the handoff loop. On each iteration the active agent runs. If
// it returns a HandoffError, control transfers to the target agent. The loop
// ends when an agent returns normally (final answer) or the handoff limit is
// exceeded.
func (h *HandoffAgent) Run(ctx context.Context) (message.Message, error) {
	handoffs := 0

	for {
		agent, ok := h.members[h.active]
		if !ok {
			return message.Message{}, fmt.Errorf("handoff: unknown agent %q", h.active)
		}

		reply, err := agent.Run(ctx)
		if err == nil {
			return reply, nil
		}

		var he *HandoffError
		if !errors.As(err, &he) {
			return message.Message{}, err
		}

		if _, ok := h.members[he.Target]; !ok {
			return message.Message{}, fmt.Errorf("handoff: unknown target agent %q", he.Target)
		}

		handoffs++
		if h.opts.MaxHandoffs > 0 && handoffs > h.opts.MaxHandoffs {
			return message.Message{}, ErrMaxHandoffs
		}

		h.active = he.Target
	}
}

// buildTransferToolBox creates a ToolBox with a transfer_to_<name> tool for
// each member. Each tool's handler returns a HandoffError that the Run loop
// catches.
func buildTransferToolBox(members []Member) *toolbox.ToolBox {
	tb := toolbox.New()

	for _, m := range members {
		target := m.Name
		tb.Register(toolbox.Tool{
			Name:        "transfer_to_" + target,
			Description: fmt.Sprintf("Transfer control to the %q agent", target),
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
				return "", &HandoffError{Target: target}
			},
		})
	}

	return tb
}
