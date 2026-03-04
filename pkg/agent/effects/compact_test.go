package effects

import (
	"context"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

// sequenceCompleter returns a sequence of preconfigured replies.
type sequenceCompleter struct {
	replies []message.Message
	index   int
}

func (p *sequenceCompleter) Complete(_ context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	if p.index >= len(p.replies) {
		return message.Message{}, errors.New("no more replies")
	}
	reply := p.replies[p.index]
	p.index++
	return reply, nil
}

// usageCompleter wraps a sequenceCompleter and implements UsageReporter.
type usageCompleter struct {
	sequenceCompleter
	tracker usage.Tracker
}

func (u *usageCompleter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	return u.sequenceCompleter.Complete(ctx, c, tools)
}

func (u *usageCompleter) UsageTracker() *usage.Tracker { return &u.tracker }
func (u *usageCompleter) ModelMaxTokens() int          { return 4096 }

// failOnceCompleter fails on the first Complete call, then delegates to inner.
type failOnceCompleter struct {
	failErr error
	inner   *usageCompleter
	called  bool
}

func (f *failOnceCompleter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	if !f.called {
		f.called = true
		return message.Message{}, f.failErr
	}
	return f.inner.Complete(ctx, c, tools)
}

func (f *failOnceCompleter) UsageTracker() *usage.Tracker { return f.inner.UsageTracker() }
func (f *failOnceCompleter) ModelMaxTokens() int          { return f.inner.ModelMaxTokens() }

// --- CompactEffect phase filtering ---

func TestCompactEffect_SkipsAfterComplete(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
	})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestCompactEffect_SkipsIteration0WithoutEstimate(t *testing.T) {
	// Without EstimatedTokens and no prior usage data, iteration 0
	// should be skipped (UsageReporter has no data).
	uc := &usageCompleter{}

	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
	})

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 0,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestCompactEffect_FiresOnIteration0WithEstimate(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "Summary of initial context."),
			},
		},
	}

	var notified bool
	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		NotifyFunc: func(_ context.Context, _ string) {
			notified = true
		},
	})

	c := chat.New(
		message.NewText("bot", role.System, "Be helpful."),
		message.NewText("user", role.User, "Large delegation context here"),
	)

	ic := agent.IterationContext{
		Phase:           agent.PhaseBeforeComplete,
		Iteration:       0,
		Chat:            c,
		Completer:       uc,
		AgentName:       "bot",
		EstimatedTokens: 900, // >= 1000 * 0.8 = 800
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.True(t, notified)
	// Chat should be compacted.
	assert.Equal(t, 2, c.Len())
}

func TestCompactEffect_ShouldCompactWithEstimateOnly(t *testing.T) {
	// shouldCompact returns true from estimate even with no usage data.
	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
	})

	// No usage data, but estimate is over threshold.
	uc := &usageCompleter{}
	result := e.shouldCompact(uc, 900) // >= 800
	assert.True(t, result)

	// Estimate below threshold.
	result = e.shouldCompact(uc, 500)
	assert.False(t, result)
}

// --- shouldCompact logic ---

func TestCompactEffect_BelowThreshold(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 50000, OutputTokens: 1000})

	e := NewCompactEffect(CompactConfig{
		ContextWindow: 100000,
		Threshold:     0.8,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.NewText("user", role.User, "test"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// Chat unchanged — no compaction.
	assert.Equal(t, 2, c.Len())
}

func TestCompactEffect_DisabledByZeroWindow(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewCompactEffect(CompactConfig{
		ContextWindow: 0,
		Threshold:     0.8,
	})

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestCompactEffect_DisabledByZeroThreshold(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0,
	})

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

// --- compaction success ---

func TestCompactEffect_Success(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "Summary: user asked for help."),
			},
		},
	}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	var notified bool
	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		NotifyFunc: func(_ context.Context, msg string) {
			notified = true
			assert.Equal(t, "Context window compacted", msg)
		},
	})

	c := chat.New(
		message.NewText("bot", role.System, "Be helpful."),
		message.NewText("user", role.User, "Help me"),
		message.NewText("bot", role.Assistant, "Sure!"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.True(t, notified)

	// Chat should now have exactly 2 messages: system + compacted user msg.
	assert.Equal(t, 2, c.Len())
	assert.Equal(t, role.System, c.At(0).Role)
	assert.Contains(t, c.At(1).TextContent(), "Summary: user asked for help.")
	assert.Contains(t, c.At(1).TextContent(), "[Conversation compacted")
	assert.Contains(t, c.At(1).TextContent(), "Continue executing the next steps")
}

// --- compaction failure ---

func TestCompactEffect_Error_NoAskFunc(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{},
	}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.NewText("user", role.User, "test"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	// Without AskFunc, compact should continue silently.
	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestCompactEffect_Error_AskUser_Continue(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{},
	}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	var asked bool
	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		AskFunc: func(_ context.Context, text string, _ []string) (string, error) {
			asked = true
			assert.Contains(t, text, "Context compaction failed")
			return "Continue without compaction", nil
		},
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.NewText("user", role.User, "test"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.True(t, asked)
}

func TestCompactEffect_Error_AskUser_Retry(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "Retry summary."),
			},
		},
	}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	failThenSucceed := &failOnceCompleter{
		failErr: errors.New("temporary error"),
		inner:   uc,
	}

	callCount := 0
	var notified bool
	e := NewCompactEffect(CompactConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		AskFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			callCount++
			return "Retry compaction", nil
		},
		NotifyFunc: func(_ context.Context, _ string) {
			notified = true
		},
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.NewText("user", role.User, "test"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: failThenSucceed,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
	assert.True(t, notified)
}
