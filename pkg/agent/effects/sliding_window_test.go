package effects

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlidingWindowEffect_SkipsAfterComplete(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewSlidingWindowEffect(SlidingWindowConfig{
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

func TestSlidingWindowEffect_SkipsIteration0(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewSlidingWindowEffect(SlidingWindowConfig{
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

func TestSlidingWindowEffect_BelowThreshold(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 500, OutputTokens: 100})

	e := NewSlidingWindowEffect(SlidingWindowConfig{
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
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 2, c.Len())
}

func TestSlidingWindowEffect_TrimsMediumZone(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "Updated summary."),
			},
		},
	}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewSlidingWindowEffect(SlidingWindowConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		RecentZone:    2,
		MediumZone:    2,
		TrimLength:    10,
	})

	longContent := "this is a very long tool result that should be trimmed in the medium zone"
	c := chat.New(
		message.NewText("", role.System, "sys"),
		// Old zone (will be summarized away).
		message.NewText("user", role.User, "old request"),
		message.NewText("bot", role.Assistant, "old response"),
		// Medium zone (tool results trimmed).
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{"path":"/a"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longContent},
		),
		// Recent zone (full fidelity).
		message.NewText("user", role.User, "recent question"),
		message.NewText("bot", role.Assistant, "recent answer"),
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

	// Old messages should be gone, replaced by summary.
	msgs := c.Messages()
	assert.Equal(t, role.System, msgs[0].Role)

	// Should have a summary message.
	assert.Contains(t, msgs[1].TextContent(), "[Context summary")

	// Recent messages should be preserved intact.
	last := msgs[len(msgs)-1]
	assert.Equal(t, "recent answer", last.TextContent())
}

func TestSlidingWindowEffect_NotEnoughMessages(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewSlidingWindowEffect(SlidingWindowConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		RecentZone:    10, // More than available messages.
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.NewText("user", role.User, "hello"),
		message.NewText("bot", role.Assistant, "hi"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// Nothing should change.
	assert.Equal(t, 3, c.Len())
}

func TestSlidingWindowEffect_DefaultConfig(t *testing.T) {
	e := NewSlidingWindowEffect(SlidingWindowConfig{})
	assert.Equal(t, defaultRecentZone, e.cfg.RecentZone)
	assert.Equal(t, defaultMediumZone, e.cfg.MediumZone)
	assert.Equal(t, defaultSlidingTrimLen, e.cfg.TrimLength)
}

func TestSlidingWindowEffect_PreservesErrorToolResults(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "Summary."),
			},
		},
	}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	longError := "this is a long error message that should NOT be trimmed because errors are important"

	e := NewSlidingWindowEffect(SlidingWindowConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		RecentZone:    1,
		MediumZone:    2,
		TrimLength:    10,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		// Old zone.
		message.NewText("user", role.User, "old"),
		// Medium zone.
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longError, IsError: true},
		),
		// Recent zone.
		message.NewText("bot", role.Assistant, "final"),
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

	// Find the error tool result — it should be preserved.
	var found bool
	msgs := c.Messages()
	for _, m := range msgs {
		if m.Role != role.Tool {
			continue
		}
		for _, p := range m.Parts {
			tr, ok := p.(content.ToolResult)
			if ok && tr.IsError {
				assert.Equal(t, longError, tr.Content)
				found = true
			}
		}
	}
	assert.True(t, found)
}

func TestSlidingWindowEffect_AllSystemMessagesNoPanic(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	e := NewSlidingWindowEffect(SlidingWindowConfig{
		ContextWindow: 1000,
		Threshold:     0.8,
		RecentZone:    2,
	})

	// Chat with only system messages — edge case that must not panic or corrupt.
	c := chat.New(
		message.NewText("", role.System, "sys prompt"),
		message.NewText("", role.System, "another system message"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Chat should be unchanged — nonSystem is empty, nothing to manage.
	assert.Equal(t, 2, c.Len())
}
