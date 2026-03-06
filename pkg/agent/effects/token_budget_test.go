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

	// Reset clears the warned flag.
	e.Reset()
	assert.False(t, e.warned)
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
