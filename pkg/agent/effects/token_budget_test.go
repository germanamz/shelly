package effects

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenBudgetEffect_SkipsBeforeComplete(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000})

	c := chat.New(message.NewText("", role.System, "sys"))
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 200})

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len()) // no message injected
}

func TestTokenBudgetEffect_SkipsWhenMaxTokensZero(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 0})

	c := chat.New(message.NewText("", role.System, "sys"))
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 9999})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestTokenBudgetEffect_SkipsWithoutUsageReporter(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000})

	c := chat.New(message.NewText("", role.System, "sys"))
	// sequenceCompleter does not implement UsageReporter
	sc := &sequenceCompleter{}

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: sc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestTokenBudgetEffect_InjectsWarnMessage(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000, WarnThreshold: 0.8})

	c := chat.New(message.NewText("", role.System, "sys"))
	uc := &usageCompleter{}
	// 500 input + 350 output = 850 total → 85% of 1000
	uc.tracker.Add(usage.TokenCount{InputTokens: 500, OutputTokens: 350})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	require.Equal(t, 2, c.Len())

	warn := c.At(1)
	assert.Equal(t, role.User, warn.Role)
	assert.Contains(t, warn.TextContent(), "85%")
	assert.Contains(t, warn.TextContent(), "token budget")
}

func TestTokenBudgetEffect_WarnOnlyOnce(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000, WarnThreshold: 0.8})

	c := chat.New(message.NewText("", role.System, "sys"))
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 450, OutputTokens: 400})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	// First eval — should warn.
	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 2, c.Len())

	// Second eval — should NOT warn again.
	ic.Iteration = 2
	err = e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 2, c.Len())
}

func TestTokenBudgetEffect_ReturnsErrorAtBudget(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000})

	c := chat.New(message.NewText("", role.System, "sys"))
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 600, OutputTokens: 500})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	assert.ErrorIs(t, err, agent.ErrTokenBudgetExhausted)
}

func TestTokenBudgetEffect_NoWarnBelowThreshold(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000, WarnThreshold: 0.8})

	c := chat.New(message.NewText("", role.System, "sys"))
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 300, OutputTokens: 100})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len()) // no message injected
}

func TestTokenBudgetEffect_Reset(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000, WarnThreshold: 0.8})

	c := chat.New(message.NewText("", role.System, "sys"))
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 450, OutputTokens: 400})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	// Trigger warning.
	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.True(t, e.warned)

	// Reset clears the warned flag and snapshots baseline.
	e.Reset()
	assert.False(t, e.warned)
	assert.Equal(t, 850, e.baseline)
}

func TestTokenBudgetEffect_ResetBaselineAcrossRuns(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000, WarnThreshold: 0.8})

	uc := &usageCompleter{}

	// Simulate first run consuming 500 tokens.
	uc.tracker.Add(usage.TokenCount{InputTokens: 300, OutputTokens: 200})

	c := chat.New(message.NewText("", role.System, "sys"))
	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}
	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len()) // no warning at 50%

	// Reset between runs — snapshots baseline at 500.
	e.Reset()
	assert.Equal(t, 500, e.baseline)

	// Simulate second run: add 400 more tokens (cumulative 900, but per-run 400).
	uc.tracker.Add(usage.TokenCount{InputTokens: 200, OutputTokens: 200})

	c2 := chat.New(message.NewText("", role.System, "sys"))
	ic2 := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c2,
		Completer: uc,
	}
	// Per-run usage is 400/1000 = 40%, no warning expected.
	err = e.Eval(context.Background(), ic2)
	require.NoError(t, err)
	assert.Equal(t, 1, c2.Len()) // no warning injected

	// Add more tokens to push per-run usage to 850 (cumulative 1350).
	uc.tracker.Add(usage.TokenCount{InputTokens: 250, OutputTokens: 200})
	err = e.Eval(context.Background(), ic2)
	require.NoError(t, err)
	assert.Equal(t, 2, c2.Len()) // warning injected at 85% of per-run budget
}

func TestTokenBudgetEffect_DefaultWarnThreshold(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000})
	assert.InEpsilon(t, defaultTokenBudgetWarnThreshold, e.cfg.WarnThreshold, 1e-9)
}

func TestTokenBudgetEffect_InvalidWarnThresholdDefaulted(t *testing.T) {
	e := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000, WarnThreshold: 1.5})
	assert.InEpsilon(t, defaultTokenBudgetWarnThreshold, e.cfg.WarnThreshold, 1e-9)

	e2 := NewTokenBudgetEffect(TokenBudgetConfig{MaxTokens: 1000, WarnThreshold: -0.5})
	assert.InEpsilon(t, defaultTokenBudgetWarnThreshold, e2.cfg.WarnThreshold, 1e-9)
}
