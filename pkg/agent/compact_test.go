package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers for compaction ---

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

// --- shouldCompact tests ---

func TestShouldCompact_Disabled(t *testing.T) {
	// ContextWindow = 0 means compaction is disabled.
	a := New("bot", "", "", &sequenceCompleter{}, Options{})
	assert.False(t, a.shouldCompact())
}

func TestShouldCompact_ThresholdDisabled(t *testing.T) {
	a := New("bot", "", "", &sequenceCompleter{}, Options{
		ContextWindow:    100000,
		ContextThreshold: 0,
	})
	assert.False(t, a.shouldCompact())
}

func TestShouldCompact_NotUsageReporter(t *testing.T) {
	// Plain sequenceCompleter doesn't implement UsageReporter.
	a := New("bot", "", "", &sequenceCompleter{}, Options{
		ContextWindow:    100000,
		ContextThreshold: 0.8,
	})
	assert.False(t, a.shouldCompact())
}

func TestShouldCompact_NoUsageData(t *testing.T) {
	uc := &usageCompleter{}
	a := New("bot", "", "", uc, Options{
		ContextWindow:    100000,
		ContextThreshold: 0.8,
	})
	assert.False(t, a.shouldCompact())
}

func TestShouldCompact_BelowThreshold(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 50000, OutputTokens: 1000})

	a := New("bot", "", "", uc, Options{
		ContextWindow:    100000,
		ContextThreshold: 0.8,
	})
	// 50000 < 100000*0.8 = 80000
	assert.False(t, a.shouldCompact())
}

func TestShouldCompact_AtThreshold(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 80000, OutputTokens: 1000})

	a := New("bot", "", "", uc, Options{
		ContextWindow:    100000,
		ContextThreshold: 0.8,
	})
	// 80000 >= 100000*0.8 = 80000
	assert.True(t, a.shouldCompact())
}

func TestShouldCompact_AboveThreshold(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 95000, OutputTokens: 2000})

	a := New("bot", "", "", uc, Options{
		ContextWindow:    100000,
		ContextThreshold: 0.8,
	})
	assert.True(t, a.shouldCompact())
}

// --- renderConversation tests ---

func TestRenderConversation_Basic(t *testing.T) {
	c := chat.New(
		message.NewText("", role.System, "You are helpful."),
		message.NewText("user", role.User, "Hello"),
		message.NewText("bot", role.Assistant, "Hi there!"),
	)

	result := renderConversation(c)

	// System messages are skipped.
	assert.NotContains(t, result, "You are helpful.")
	assert.Contains(t, result, "[user] Hello")
	assert.Contains(t, result, "[assistant] Hi there!")
}

func TestRenderConversation_ToolCalls(t *testing.T) {
	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"/foo/bar.go"}`},
		),
		message.New("bot", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "file contents here"},
		),
	)

	result := renderConversation(c)

	assert.Contains(t, result, `[assistant] Called tool read_file({"path":"/foo/bar.go"})`)
	assert.Contains(t, result, "[tool result] file contents here")
}

func TestRenderConversation_ToolError(t *testing.T) {
	c := chat.New(
		message.New("bot", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "not found", IsError: true},
		),
	)

	result := renderConversation(c)

	assert.Contains(t, result, "[tool error] not found")
}

func TestRenderConversation_TruncatesLongArgs(t *testing.T) {
	longArgs := strings.Repeat("x", 300)
	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "test", Arguments: longArgs},
		),
	)

	result := renderConversation(c)

	// Should be truncated to 200 chars + "…"
	assert.Contains(t, result, strings.Repeat("x", 200)+"…")
	assert.NotContains(t, result, strings.Repeat("x", 201)+")")
}

func TestRenderConversation_TruncatesLongResults(t *testing.T) {
	longResult := strings.Repeat("y", 600)
	c := chat.New(
		message.New("bot", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longResult},
		),
	)

	result := renderConversation(c)

	assert.Contains(t, result, strings.Repeat("y", 500)+"…")
}

// --- compact tests ---

func TestCompact_Success(t *testing.T) {
	summaryCompleter := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				// Summarization result — compact() calls Complete once.
				message.NewText("", role.Assistant, "Summary: user asked for help."),
			},
		},
	}

	a := New("bot", "", "Be helpful.", summaryCompleter, Options{
		ContextWindow:    1000,
		ContextThreshold: 0.8,
	})
	a.Init()
	a.chat.Append(
		message.NewText("user", role.User, "Help me"),
		message.NewText("bot", role.Assistant, "Sure!"),
	)

	var notified bool
	a.options.NotifyFunc = func(_ context.Context, msg string) {
		notified = true
		assert.Equal(t, "Context window compacted", msg)
	}

	err := a.compact(context.Background())

	require.NoError(t, err)
	assert.True(t, notified)

	// Chat should now have exactly 2 messages: system + compacted user msg.
	assert.Equal(t, 2, a.chat.Len())
	assert.Equal(t, role.System, a.chat.At(0).Role)
	assert.Contains(t, a.chat.At(1).TextContent(), "Summary: user asked for help.")
	assert.Contains(t, a.chat.At(1).TextContent(), "[Conversation compacted")
}

func TestCompact_SummarizationError_NoAskFunc(t *testing.T) {
	ec := &usageCompleter{
		sequenceCompleter: sequenceCompleter{},
	}
	// Force the completer to fail (no more replies).
	ec.index = 0

	a := New("bot", "", "", ec, Options{
		ContextWindow:    1000,
		ContextThreshold: 0.8,
	})
	a.Init()
	a.chat.Append(message.NewText("user", role.User, "test"))

	// Without AskFunc, compact should continue silently.
	err := a.compact(context.Background())
	require.NoError(t, err)
}

func TestCompact_SummarizationError_AskUser_Continue(t *testing.T) {
	ec := &usageCompleter{
		sequenceCompleter: sequenceCompleter{},
	}
	ec.index = 0

	var asked bool
	a := New("bot", "", "", ec, Options{
		ContextWindow:    1000,
		ContextThreshold: 0.8,
		AskFunc: func(_ context.Context, text string, options []string) (string, error) {
			asked = true
			assert.Contains(t, text, "Context compaction failed")
			return "Continue without compaction", nil
		},
	})
	a.Init()
	a.chat.Append(message.NewText("user", role.User, "test"))

	err := a.compact(context.Background())
	require.NoError(t, err)
	assert.True(t, asked)
}

func TestCompact_SummarizationError_AskUser_Retry(t *testing.T) {
	callCount := 0
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				// The retry summarization call succeeds.
				message.NewText("", role.Assistant, "Retry summary."),
			},
		},
	}
	// Make the first call fail by exhausting replies, then reset for retry.
	// Actually, let's use a custom approach: first call fails (index=1, no reply at 1),
	// then for retry we need a fresh reply.
	// Simpler: just have the first summarization fail and the retry succeed.
	failThenSucceed := &failOnceCompleter{
		failErr: errors.New("temporary error"),
		inner:   uc,
	}

	var notified bool
	a := New("bot", "", "", failThenSucceed, Options{
		ContextWindow:    1000,
		ContextThreshold: 0.8,
		AskFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			callCount++
			return "Retry compaction", nil
		},
		NotifyFunc: func(_ context.Context, _ string) {
			notified = true
		},
	})
	a.Init()
	a.chat.Append(message.NewText("user", role.User, "test"))

	err := a.compact(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
	assert.True(t, notified)
}

// --- truncate tests ---

func TestTruncate_Short(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 10))
}

func TestTruncate_Exact(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 3))
}

func TestTruncate_Long(t *testing.T) {
	assert.Equal(t, "ab…", truncate("abcdef", 2))
}

// --- integration test: run loop with compaction ---

func TestRunWithCompaction(t *testing.T) {
	uc := &usageCompleter{
		sequenceCompleter: sequenceCompleter{
			replies: []message.Message{
				// Iteration 0: tool call.
				message.New("", role.Assistant,
					content.ToolCall{ID: "c1", Name: "echo", Arguments: `{"msg":"hi"}`},
				),
				// Iteration 1 (compaction happens before this): summarization reply.
				message.NewText("", role.Assistant, "Conversation summary."),
				// Iteration 1 (after compaction): final answer.
				message.NewText("", role.Assistant, "All done after compaction."),
			},
		},
	}

	// Set usage so shouldCompact triggers after iteration 0.
	uc.tracker.Add(usage.TokenCount{InputTokens: 900, OutputTokens: 100})

	var compacted bool
	a := New("bot", "", "", uc, Options{
		ContextWindow:    1000,
		ContextThreshold: 0.8,
		NotifyFunc: func(_ context.Context, _ string) {
			compacted = true
		},
	})
	a.AddToolBoxes(newEchoToolBox())

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "All done after compaction.", result.TextContent())
	assert.True(t, compacted)
}

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
