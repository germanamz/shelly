package effects

import (
	"context"
	"fmt"
	"time"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

const defaultTimeBudgetWarnThreshold = 0.8

// TimeBudgetConfig holds parameters for the TimeBudgetEffect.
type TimeBudgetConfig struct {
	MaxDuration   time.Duration // Hard cap on cumulative LLM inference time.
	WarnThreshold float64       // Fraction at which to inject a wrap-up message (default: 0.8).
}

// TimeBudgetEffect tracks cumulative LLM inference time and enforces a budget.
// It runs at both phases: PhaseBeforeComplete records the call start time,
// PhaseAfterComplete accumulates elapsed time and checks thresholds.
// At the warn threshold it injects a wrap-up message; at 100% it returns
// ErrTimeBudgetExhausted.
type TimeBudgetEffect struct {
	cfg       TimeBudgetConfig
	warned    bool
	elapsed   time.Duration
	callStart time.Time
	nowFunc   func() time.Time // for testing; defaults to time.Now
}

// NewTimeBudgetEffect creates a TimeBudgetEffect with the given configuration.
func NewTimeBudgetEffect(cfg TimeBudgetConfig) *TimeBudgetEffect {
	if cfg.WarnThreshold <= 0 || cfg.WarnThreshold >= 1 {
		cfg.WarnThreshold = defaultTimeBudgetWarnThreshold
	}
	return &TimeBudgetEffect{cfg: cfg, nowFunc: time.Now}
}

// Reset implements agent.Resetter.
func (e *TimeBudgetEffect) Reset() {
	e.warned = false
	e.elapsed = 0
	e.callStart = time.Time{}
}

// Eval implements agent.Effect.
func (e *TimeBudgetEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if e.cfg.MaxDuration <= 0 {
		return nil
	}

	now := e.nowFunc

	switch ic.Phase {
	case agent.PhaseBeforeComplete:
		e.callStart = now()
		return nil

	case agent.PhaseAfterComplete:
		if !e.callStart.IsZero() {
			e.elapsed += now().Sub(e.callStart)
			e.callStart = time.Time{}
		}

		ratio := float64(e.elapsed) / float64(e.cfg.MaxDuration)

		if ratio >= 1.0 {
			return agent.ErrTimeBudgetExhausted
		}

		if !e.warned && ratio >= e.cfg.WarnThreshold {
			e.warned = true
			ic.Chat.Append(message.NewText("", role.User,
				fmt.Sprintf(
					"You've used %.0f%% of your time budget (%s/%s of LLM inference time). "+
						"Prioritize completing the most critical remaining work.",
					ratio*100, e.elapsed.Truncate(time.Second), e.cfg.MaxDuration.Truncate(time.Second),
				),
			))
		}

		return nil
	}

	return nil
}
