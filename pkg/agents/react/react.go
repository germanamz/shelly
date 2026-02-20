// Package react implements the ReAct (Reason + Act) agent pattern. It
// drives an Agent through iterative cycles of LLM completion and tool
// execution until the provider returns a final answer with no tool calls.
package react

import (
	"context"
	"errors"

	"github.com/germanamz/shelly/pkg/agents"
	"github.com/germanamz/shelly/pkg/chats/message"
)

// Compile-time check that *ReActAgent implements agents.Agent.
var _ agents.Agent = (*ReActAgent)(nil)

// ErrMaxIterations is returned when the ReAct loop exceeds MaxIterations
// without the provider producing a final answer.
var ErrMaxIterations = errors.New("react: max iterations reached")

// Options configures the ReAct loop.
type Options struct {
	// MaxIterations limits the number of reason-act cycles. Zero means no limit.
	MaxIterations int
}

// ReActAgent implements the ReAct pattern by embedding agents.AgentBase. Each call to
// Run drives iterative cycles of LLM completion and tool execution until the
// provider returns a final answer with no tool calls.
type ReActAgent struct {
	agents.AgentBase
	Options Options
}

// New creates a ReActAgent from an AgentBase and options.
func New(base agents.AgentBase, opts Options) *ReActAgent {
	return &ReActAgent{
		AgentBase: base,
		Options:   opts,
	}
}

// Run executes the ReAct loop. Each iteration calls Complete to get the
// provider's reply, then CallTools if the reply contains tool calls. The loop
// ends when the provider returns a message with no tool calls (the final
// answer) or MaxIterations is reached.
func (a *ReActAgent) Run(ctx context.Context) (message.Message, error) {
	for i := 0; a.Options.MaxIterations == 0 || i < a.Options.MaxIterations; i++ {
		reply, err := a.Complete(ctx)
		if err != nil {
			return message.Message{}, err
		}

		if len(reply.ToolCalls()) == 0 {
			return reply, nil
		}

		a.CallTools(ctx, reply)
	}

	return message.Message{}, ErrMaxIterations
}
