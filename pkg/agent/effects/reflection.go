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
	cfg               ReflectionConfig
	lastInjectedCount int // guards against re-injecting at same count
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

	if count < e.cfg.FailureThreshold {
		e.lastInjectedCount = 0

		return nil
	}

	// Only inject when count increases since last injection.
	if count <= e.lastInjectedCount {
		return nil
	}

	e.lastInjectedCount = count

	ic.Chat.Append(message.NewText("", role.User,
		fmt.Sprintf(`You have encountered %d consecutive tool failures. Before your next action:
1. Analyze what went wrong in each failed attempt
2. Identify the root cause (wrong path? wrong approach? missing prerequisite?)
3. Describe a different strategy you will try next`, count),
	))

	return nil
}

// Reset clears per-run state so the effect behaves correctly across multiple
// Run() calls on a long-lived agent. Implements agent.Resetter.
func (e *ReflectionEffect) Reset() { e.lastInjectedCount = 0 }

// countConsecutiveFailures scans from the end of the chat backwards, grouping
// consecutive tool messages between assistant messages into a single "step".
// A step counts as a failure only if ALL tool results in it failed (none
// succeeded). Stops at the first successful step or user message.
func (e *ReflectionEffect) countConsecutiveFailures(ic agent.IterationContext) int {
	msgs := ic.Chat.Messages()
	count := 0
	stepHasError := false
	stepHasSuccess := false
	inStep := false

	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == role.Assistant {
			if inStep {
				if stepHasError && !stepHasSuccess {
					count++
				} else if inStep && (stepHasSuccess || !stepHasError) {
					break
				}
				stepHasError = false
				stepHasSuccess = false
				inStep = false
			}

			continue
		}

		if msgs[i].Role != role.Tool {
			if inStep {
				if stepHasError && !stepHasSuccess {
					count++
				} else {
					break
				}
			}

			break
		}

		inStep = true

		for _, p := range msgs[i].Parts {
			tr, ok := p.(content.ToolResult)
			if !ok {
				continue
			}

			if tr.IsError {
				stepHasError = true
			} else {
				stepHasSuccess = true
			}
		}
	}

	if inStep {
		if stepHasError && !stepHasSuccess {
			count++
		}
	}

	return count
}
