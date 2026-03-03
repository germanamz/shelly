package effects

import (
	"context"
	"strings"
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

func TestOffloadEffect_OffloadsLargeResult(t *testing.T) {
	dir := t.TempDir()

	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 600, OutputTokens: 100})

	e := NewOffloadEffect(OffloadConfig{
		ContextWindow: 1000,
		Threshold:     0.5,
		MinResultLen:  50,
		RecentWindow:  1,
		StorageDir:    dir,
	})

	longContent := strings.Repeat("x", 100)
	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longContent},
		),
		// Recent tool message (preserved).
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "fs_read", Arguments: `{}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "recent short result"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Old tool result should be offloaded.
	oldResult := c.At(2).Parts[0].(content.ToolResult)
	assert.Contains(t, oldResult.Content, "[offloaded:")
	assert.Contains(t, oldResult.Content, "recall(")

	// Recent tool result should be preserved.
	recentResult := c.At(4).Parts[0].(content.ToolResult)
	assert.Equal(t, "recent short result", recentResult.Content)

	// Recall should work.
	recalled, err := e.Recall("c1")
	require.NoError(t, err)
	assert.Equal(t, longContent, recalled)
}

func TestOffloadEffect_SkipsSmallResults(t *testing.T) {
	dir := t.TempDir()

	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 600, OutputTokens: 100})

	e := NewOffloadEffect(OffloadConfig{
		ContextWindow: 1000,
		Threshold:     0.5,
		MinResultLen:  500,
		RecentWindow:  0,
		StorageDir:    dir,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "short result"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Result should be unchanged.
	result := c.At(2).Parts[0].(content.ToolResult)
	assert.Equal(t, "short result", result.Content)
}

func TestOffloadEffect_ResetCleansUp(t *testing.T) {
	dir := t.TempDir()

	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 600, OutputTokens: 100})

	e := NewOffloadEffect(OffloadConfig{
		ContextWindow: 1000,
		Threshold:     0.5,
		MinResultLen:  10,
		RecentWindow:  0,
		StorageDir:    dir,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: strings.Repeat("x", 50)},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Recall works before reset.
	_, err = e.Recall("c1")
	require.NoError(t, err)

	// Reset cleans up.
	e.Reset()

	_, err = e.Recall("c1")
	require.Error(t, err)
}

func TestOffloadEffect_PreservesErrors(t *testing.T) {
	dir := t.TempDir()

	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 600, OutputTokens: 100})

	longError := strings.Repeat("e", 500)
	e := NewOffloadEffect(OffloadConfig{
		ContextWindow: 1000,
		Threshold:     0.5,
		MinResultLen:  10,
		RecentWindow:  0,
		StorageDir:    dir,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longError, IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Error results are never offloaded.
	result := c.At(2).Parts[0].(content.ToolResult)
	assert.Equal(t, longError, result.Content)
}

func TestOffloadEffect_ProvidedToolsReturnsRecall(t *testing.T) {
	e := NewOffloadEffect(OffloadConfig{
		StorageDir: t.TempDir(),
	})

	tb := e.ProvidedTools()
	require.NotNil(t, tb)

	_, ok := tb.Get("recall")
	assert.True(t, ok)
}

func TestOffloadEffect_ProvidedToolsNilWithoutStorageDir(t *testing.T) {
	e := NewOffloadEffect(OffloadConfig{})
	assert.Nil(t, e.ProvidedTools())
}

func TestOffloadEffect_SkipsWhenBelowThreshold(t *testing.T) {
	dir := t.TempDir()

	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 100, OutputTokens: 10})

	e := NewOffloadEffect(OffloadConfig{
		ContextWindow: 1000,
		Threshold:     0.5,
		MinResultLen:  10,
		RecentWindow:  0,
		StorageDir:    dir,
	})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: strings.Repeat("x", 50)},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
		Completer: uc,
		AgentName: "bot",
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Result unchanged — below threshold.
	result := c.At(2).Parts[0].(content.ToolResult)
	assert.NotContains(t, result.Content, "[offloaded:")
}
