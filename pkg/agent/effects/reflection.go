package effects

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

const defaultFailureThreshold = 2

// ReflectionConfig holds parameters for the ReflectionEffect.
type ReflectionConfig struct {
	FailureThreshold int // Consecutive failures before reflection (default: 2).
}

// ReflectionEffect detects consecutive tool failures and injects a reflection
// prompt, forcing the agent to analyze what went wrong before retrying. It runs
// at PhaseBeforeComplete when Iteration > 0.
type ReflectionEffect struct {
	cfg ReflectionConfig
}

// NewReflectionEffect creates a ReflectionEffect with the given configuration,
// applying defaults for zero or negative values.
func NewReflectionEffect(cfg ReflectionConfig) *ReflectionEffect {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = defaultFailureThreshold
	}

	return &ReflectionEffect{cfg: cfg}
}

// Eval implements agent.Effect.
func (e *ReflectionEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseBeforeComplete || ic.Iteration == 0 {
		return nil
	}

	count := e.countConsecutiveFailures(ic)
	if count >= e.cfg.FailureThreshold {
		ic.Chat.Append(message.NewText("", role.User,
			fmt.Sprintf(`You have encountered %d consecutive tool failures. Before your next action:
1. Analyze what went wrong in each failed attempt
2. Identify the root cause (wrong path? wrong approach? missing prerequisite?)
3. Describe a different strategy you will try next`, count),
		))
	}

	return nil
}

// countConsecutiveFailures scans from the end of the chat for consecutive
// tool-role messages with IsError == true. It skips assistant messages between
// tool results (since tool calls always produce an assistant message followed
// by tool results). Stops at the first non-error tool result or user message.
func (e *ReflectionEffect) countConsecutiveFailures(ic agent.IterationContext) int {
	msgs := ic.Chat.Messages()
	count := 0

	for i := len(msgs) - 1; i >= 0; i-- {
		// Skip assistant messages between tool results.
		if msgs[i].Role == role.Assistant {
			continue
		}

		if msgs[i].Role != role.Tool {
			break
		}

		allErrors := true

		for _, p := range msgs[i].Parts {
			tr, ok := p.(content.ToolResult)
			if !ok {
				continue
			}

			if !tr.IsError {
				allErrors = false
				break
			}
		}

		if !allErrors {
			break
		}

		count++
	}

	return count
}
