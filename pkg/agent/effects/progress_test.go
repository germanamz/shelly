package effects

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProgressEffect_SkipsAfterComplete(t *testing.T) {
	e := NewProgressEffect(ProgressConfig{Interval: 5})

	c := chat.New(message.NewText("", role.System, "sys"))

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 5,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len())
}

func TestProgressEffect_SkipsIteration0(t *testing.T) {
	e := NewProgressEffect(ProgressConfig{Interval: 5})

	c := chat.New(message.NewText("", role.System, "sys"))

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 0,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len())
}

func TestProgressEffect_InjectsAtInterval(t *testing.T) {
	e := NewProgressEffect(ProgressConfig{Interval: 5})

	c := chat.New(message.NewText("", role.System, "sys"))

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 5,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	assert.Equal(t, 2, c.Len())
	last := c.At(1)
	assert.Equal(t, role.User, last.Role)
	assert.Contains(t, last.TextContent(), "5 iterations")
	assert.Contains(t, last.TextContent(), "write_note")
}

func TestProgressEffect_InjectsAtMultipleIntervals(t *testing.T) {
	e := NewProgressEffect(ProgressConfig{Interval: 3})

	c := chat.New(message.NewText("", role.System, "sys"))

	// Iteration 3 — should inject.
	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 3,
		Chat:      c,
	}
	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 2, c.Len())

	// Iteration 6 — should inject again.
	ic.Iteration = 6
	err = e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 3, c.Len())
	assert.Contains(t, c.At(2).TextContent(), "6 iterations")
}

func TestProgressEffect_SkipsNonInterval(t *testing.T) {
	e := NewProgressEffect(ProgressConfig{Interval: 5})

	c := chat.New(message.NewText("", role.System, "sys"))

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 3,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// No injection at iteration 3 (not a multiple of 5).
	assert.Equal(t, 1, c.Len())
}

func TestProgressEffect_Defaults(t *testing.T) {
	e := NewProgressEffect(ProgressConfig{})
	assert.Equal(t, defaultProgressInterval, e.cfg.Interval)
}
