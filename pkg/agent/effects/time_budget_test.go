package effects

import (
	"context"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTimeBudgetIC(phase agent.IterationPhase, c *chat.Chat) agent.IterationContext {
	return agent.IterationContext{
		Phase:     phase,
		Iteration: 1,
		Chat:      c,
		Completer: &sequenceCompleter{},
	}
}

func TestTimeBudgetEffect_SkipsWhenMaxDurationZero(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 0})

	c := chat.New(message.NewText("", role.System, "sys"))
	err := e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	require.NoError(t, err)
}

func TestTimeBudgetEffect_RecordsCallStartBeforeComplete(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute})

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))
	err := e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	require.NoError(t, err)
	assert.Equal(t, now, e.callStart)
}

func TestTimeBudgetEffect_AccumulatesElapsedTime(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute})

	callTime := 30 * time.Second
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))

	// Before complete — records start.
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))

	// Simulate time passing.
	now = now.Add(callTime)

	// After complete — accumulates.
	err := e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	require.NoError(t, err)
	assert.Equal(t, callTime, e.elapsed)
	assert.Equal(t, 1, c.Len()) // no warning yet (30s / 10m = 5%)
}

func TestTimeBudgetEffect_InjectsWarnMessage(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute, WarnThreshold: 0.8})

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))

	// Simulate 8.5 minutes of inference (85% of 10m).
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(8*time.Minute + 30*time.Second)
	err := e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	require.NoError(t, err)

	require.Equal(t, 2, c.Len())
	warn := c.At(1)
	assert.Equal(t, role.User, warn.Role)
	assert.Contains(t, warn.TextContent(), "85%")
	assert.Contains(t, warn.TextContent(), "time budget")
}

func TestTimeBudgetEffect_WarnOnlyOnce(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute, WarnThreshold: 0.8})

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))

	// First call: 8.5 minutes — triggers warning.
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(8*time.Minute + 30*time.Second)
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	assert.Equal(t, 2, c.Len())

	// Second call: still above threshold — no second warning.
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(30 * time.Second)
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	assert.Equal(t, 2, c.Len())
}

func TestTimeBudgetEffect_ReturnsErrorAtBudget(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 5 * time.Minute})

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))

	// Simulate 6 minutes of inference (120% of 5m).
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(6 * time.Minute)
	err := e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	assert.ErrorIs(t, err, agent.ErrTimeBudgetExhausted)
}

func TestTimeBudgetEffect_NoWarnBelowThreshold(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute, WarnThreshold: 0.8})

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))

	// 2 minutes — 20%, well below threshold.
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(2 * time.Minute)
	err := e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len())
}

func TestTimeBudgetEffect_Reset(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute, WarnThreshold: 0.8})

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))

	// Accumulate some time and trigger warning.
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(9 * time.Minute)
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))
	assert.True(t, e.warned)
	assert.NotZero(t, e.elapsed)

	// Reset clears everything.
	e.Reset()
	assert.False(t, e.warned)
	assert.Zero(t, e.elapsed)
	assert.True(t, e.callStart.IsZero())
}

func TestTimeBudgetEffect_DefaultWarnThreshold(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute})
	assert.InEpsilon(t, defaultTimeBudgetWarnThreshold, e.cfg.WarnThreshold, 1e-9)
}

func TestTimeBudgetEffect_InvalidWarnThresholdDefaulted(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute, WarnThreshold: 1.5})
	assert.InEpsilon(t, defaultTimeBudgetWarnThreshold, e.cfg.WarnThreshold, 1e-9)

	e2 := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute, WarnThreshold: -0.5})
	assert.InEpsilon(t, defaultTimeBudgetWarnThreshold, e2.cfg.WarnThreshold, 1e-9)
}

func TestTimeBudgetEffect_MultipleCallsAccumulate(t *testing.T) {
	e := NewTimeBudgetEffect(TimeBudgetConfig{MaxDuration: 10 * time.Minute})

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.nowFunc = func() time.Time { return now }

	c := chat.New(message.NewText("", role.System, "sys"))

	// First LLM call: 2 minutes.
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(2 * time.Minute)
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))

	// Gap for tool execution (not measured).
	now = now.Add(5 * time.Minute)

	// Second LLM call: 3 minutes.
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseBeforeComplete, c))
	now = now.Add(3 * time.Minute)
	_ = e.Eval(context.Background(), newTimeBudgetIC(agent.PhaseAfterComplete, c))

	// Total: 2 + 3 = 5 minutes (the 5-minute gap is excluded).
	assert.Equal(t, 5*time.Minute, e.elapsed)
}
