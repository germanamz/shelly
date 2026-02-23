package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- EffectFunc adapter ---

func TestEffectFunc(t *testing.T) {
	var called bool
	ef := EffectFunc(func(_ context.Context, _ IterationContext) error {
		called = true
		return nil
	})

	err := ef.Eval(context.Background(), IterationContext{})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestEffectFunc_Error(t *testing.T) {
	ef := EffectFunc(func(_ context.Context, _ IterationContext) error {
		return errors.New("boom")
	})

	err := ef.Eval(context.Background(), IterationContext{})
	assert.EqualError(t, err, "boom")
}

// --- evalEffects tests ---

func TestEvalEffects_RunsInOrder(t *testing.T) {
	var order []string
	mkEffect := func(tag string) Effect {
		return EffectFunc(func(_ context.Context, _ IterationContext) error {
			order = append(order, tag)
			return nil
		})
	}

	a := New("bot", "", "", &sequenceCompleter{}, Options{
		Effects: []Effect{mkEffect("A"), mkEffect("B"), mkEffect("C")},
	})

	err := a.evalEffects(context.Background(), IterationContext{Phase: PhaseBeforeComplete})
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B", "C"}, order)
}

func TestEvalEffects_StopsOnError(t *testing.T) {
	var ran []string

	a := New("bot", "", "", &sequenceCompleter{}, Options{
		Effects: []Effect{
			EffectFunc(func(_ context.Context, _ IterationContext) error {
				ran = append(ran, "first")
				return nil
			}),
			EffectFunc(func(_ context.Context, _ IterationContext) error {
				return errors.New("stop")
			}),
			EffectFunc(func(_ context.Context, _ IterationContext) error {
				ran = append(ran, "third")
				return nil
			}),
		},
	})

	err := a.evalEffects(context.Background(), IterationContext{Phase: PhaseBeforeComplete})
	require.EqualError(t, err, "stop")
	assert.Equal(t, []string{"first"}, ran)
}

func TestEvalEffects_PassesIterationContext(t *testing.T) {
	var captured IterationContext

	a := New("bot", "", "", &sequenceCompleter{}, Options{
		Effects: []Effect{
			EffectFunc(func(_ context.Context, ic IterationContext) error {
				captured = ic
				return nil
			}),
		},
	})

	ic := IterationContext{
		Phase:     PhaseAfterComplete,
		Iteration: 42,
		Chat:      a.chat,
		Completer: a.completer,
		AgentName: "bot",
	}

	err := a.evalEffects(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, PhaseAfterComplete, captured.Phase)
	assert.Equal(t, 42, captured.Iteration)
	assert.Equal(t, "bot", captured.AgentName)
}

// --- Integration: ReAct loop with effect error aborts ---

func TestRunWithEffectError(t *testing.T) {
	a := New("bot", "", "", &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Should not reach."),
		},
	}, Options{
		Effects: []Effect{
			EffectFunc(func(_ context.Context, ic IterationContext) error {
				if ic.Phase == PhaseBeforeComplete {
					return errors.New("effect abort")
				}
				return nil
			}),
		},
	})

	_, err := a.Run(context.Background())
	assert.EqualError(t, err, "effect abort")
}

func TestRunWithAfterCompleteEffectError(t *testing.T) {
	a := New("bot", "", "", &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
			),
		},
	}, Options{
		Effects: []Effect{
			EffectFunc(func(_ context.Context, ic IterationContext) error {
				if ic.Phase == PhaseAfterComplete {
					return errors.New("after-complete abort")
				}
				return nil
			}),
		},
	})
	a.AddToolBoxes(newEchoToolBox())

	_, err := a.Run(context.Background())
	assert.EqualError(t, err, "after-complete abort")
}
