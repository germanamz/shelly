// Package react implements the ReAct (Reason + Act) agent pattern. It
// drives an Agent through iterative cycles of LLM completion and tool
// execution until the provider returns a final answer with no tool calls.
package react

import (
	"context"
	"errors"

	"github.com/germanamz/shelly/pkg/agents/agent"
	"github.com/germanamz/shelly/pkg/chats/message"
)

// ErrMaxIterations is returned when the ReAct loop exceeds MaxIterations
// without the provider producing a final answer.
var ErrMaxIterations = errors.New("react: max iterations reached")

// Options configures the ReAct loop.
type Options struct {
	// MaxIterations limits the number of reason-act cycles. Zero means no limit.
	MaxIterations int
}

// Run executes the ReAct loop on the given agent. Each iteration calls
// Complete to get the provider's reply, then CallTools if the reply contains
// tool calls. The loop ends when the provider returns a message with no tool
// calls (the final answer) or MaxIterations is reached.
func Run(ctx context.Context, a *agent.Agent, opts Options) (message.Message, error) {
	for i := 0; opts.MaxIterations == 0 || i < opts.MaxIterations; i++ {
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
