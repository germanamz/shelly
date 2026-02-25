package effects

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

const defaultProgressInterval = 5

// ProgressConfig holds parameters for the ProgressEffect.
type ProgressConfig struct {
	Interval int // Inject progress prompt every N iterations (default: 5).
}

// ProgressEffect periodically prompts the agent to write a progress note,
// ensuring continuity if context is compacted. It runs at PhaseBeforeComplete
// and only activates when the notes tools (write_note) are available.
type ProgressEffect struct {
	cfg ProgressConfig
}

// NewProgressEffect creates a ProgressEffect with the given configuration,
// applying defaults for zero or negative values.
func NewProgressEffect(cfg ProgressConfig) *ProgressEffect {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultProgressInterval
	}

	return &ProgressEffect{cfg: cfg}
}

// Eval implements agent.Effect.
func (e *ProgressEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseBeforeComplete || ic.Iteration == 0 {
		return nil
	}

	if ic.Iteration%e.cfg.Interval != 0 {
		return nil
	}

	ic.Chat.Append(message.NewText("", role.User,
		fmt.Sprintf(`You have completed %d iterations. Write a brief progress note using write_note documenting: (1) what you've accomplished, (2) what remains, (3) any blockers. This ensures continuity if context is compacted.`, ic.Iteration),
	))

	return nil
}
