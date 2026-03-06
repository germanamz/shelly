package effects

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

const defaultTokenBudgetWarnThreshold = 0.8

// TokenBudgetConfig holds parameters for the TokenBudgetEffect.
type TokenBudgetConfig struct {
	MaxTokens     int     // Hard cap on total tokens (input + output).
	WarnThreshold float64 // Fraction at which to inject a wrap-up message (default: 0.8).
}

// TokenBudgetEffect tracks cumulative actual token usage and enforces a budget.
// It runs at PhaseAfterComplete to read actual usage from the completer's
// UsageTracker. At the warn threshold it injects a wrap-up message; at 100%
// it returns ErrTokenBudgetExhausted.
type TokenBudgetEffect struct {
	cfg    TokenBudgetConfig
	warned bool
}

// NewTokenBudgetEffect creates a TokenBudgetEffect with the given configuration.
func NewTokenBudgetEffect(cfg TokenBudgetConfig) *TokenBudgetEffect {
	if cfg.WarnThreshold <= 0 || cfg.WarnThreshold >= 1 {
		cfg.WarnThreshold = defaultTokenBudgetWarnThreshold
	}
	return &TokenBudgetEffect{cfg: cfg}
}

// Reset implements agent.Resetter.
func (e *TokenBudgetEffect) Reset() {
	e.warned = false
}

// Eval implements agent.Effect.
func (e *TokenBudgetEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseAfterComplete {
		return nil
	}

	if e.cfg.MaxTokens <= 0 {
		return nil
	}

	reporter, ok := ic.Completer.(modeladapter.UsageReporter)
	if !ok {
		return nil
	}

	total := reporter.UsageTracker().Total()
	used := total.InputTokens + total.OutputTokens
	ratio := float64(used) / float64(e.cfg.MaxTokens)

	if ratio >= 1.0 {
		return agent.ErrTokenBudgetExhausted
	}

	if !e.warned && ratio >= e.cfg.WarnThreshold {
		e.warned = true
		ic.Chat.Append(message.NewText("", role.User,
			fmt.Sprintf(
				"You've used %.0f%% of your token budget (%d/%d tokens). "+
					"Prioritize completing the most critical remaining work.",
				ratio*100, used, e.cfg.MaxTokens,
			),
		))
	}

	return nil
}
