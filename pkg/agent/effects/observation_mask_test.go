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

func TestObservationMaskEffect_SkipsAfterComplete(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
	})

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestObservationMaskEffect_SkipsIteration0(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
	})

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 0,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestObservationMaskEffect_BelowThreshold(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 400, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
		RecentWindow:  3,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "file contents here"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// Content unchanged — below threshold.
	assert.Equal(t, "file contents here", c.At(2).Parts[0].(content.ToolResult).Content)
}

func TestObservationMaskEffect_MasksOldResults(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
		RecentWindow:  2, // Only keep last 2 messages at full fidelity.
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		// Old: assistant + tool result (will be masked).
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "old file contents that should be masked because this text is long enough to exceed the preview limit of eighty characters"},
		),
		// Recent: assistant + tool result (preserved).
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "write_file", Arguments: `{"path":"b.go"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "recent result preserved"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Old tool result should be masked.
	oldResult := c.At(2).Parts[0].(content.ToolResult)
	assert.Contains(t, oldResult.Content, "[tool result for read_file:")
	assert.NotContains(t, oldResult.Content, "exceed the preview limit")

	// Recent tool result should be preserved.
	recentResult := c.At(4).Parts[0].(content.ToolResult)
	assert.Equal(t, "recent result preserved", recentResult.Content)
}

func TestObservationMaskEffect_PreservesErrors(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
		RecentWindow:  1,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"bad.go"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "file not found: bad.go", IsError: true},
		),
		message.NewText("bot", role.Assistant, "done"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Error results should NOT be masked.
	errResult := c.At(2).Parts[0].(content.ToolResult)
	assert.Equal(t, "file not found: bad.go", errResult.Content)
}

func TestObservationMaskEffect_SkipsAlreadyMasked(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
		RecentWindow:  1,
	})

	// Pre-mask a tool result.
	toolMsg := message.New("", role.Tool,
		content.ToolResult{ToolCallID: "c1", Content: "[tool result for read_file: already masked]"},
	)
	toolMsg.SetMeta(obsMaskMetaKey, true)

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		),
		toolMsg,
		message.NewText("bot", role.Assistant, "done"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Content should remain unchanged (not double-masked).
	result := c.At(2).Parts[0].(content.ToolResult)
	assert.Equal(t, "[tool result for read_file: already masked]", result.Content)
}

func TestObservationMaskEffect_TruncatesLongPreview(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
		RecentWindow:  1,
	})

	longContent := "x]" + string(make([]byte, 200)) // > 80 chars
	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longContent},
		),
		message.NewText("bot", role.Assistant, "done"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	result := c.At(2).Parts[0].(content.ToolResult)
	assert.Contains(t, result.Content, "[tool result for read_file:")
	assert.Contains(t, result.Content, "\u2026")
}

func TestObservationMaskEffect_DisabledByZeroWindow(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 0,
		Threshold:     0.6,
	})

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestObservationMaskEffect_Defaults(t *testing.T) {
	e := NewObservationMaskEffect(ObservationMaskConfig{})
	assert.Equal(t, defaultObsMaskRecentWindow, e.cfg.RecentWindow)
	assert.InDelta(t, defaultObsMaskThreshold, e.cfg.Threshold, 0.001)
}

func TestObservationMaskEffect_EmptyChat(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
		RecentWindow:  5,
	})

	c := chat.New()

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 0, c.Len())
}

func TestObservationMaskEffect_AllMessagesWithinRecentWindow(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 700, OutputTokens: 100})

	e := NewObservationMaskEffect(ObservationMaskConfig{
		ContextWindow: 1000,
		Threshold:     0.6,
		RecentWindow:  10, // Large enough to cover all messages.
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "full content preserved"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// All within recent window — nothing masked.
	result := c.At(2).Parts[0].(content.ToolResult)
	assert.Equal(t, "full content preserved", result.Content)
}
