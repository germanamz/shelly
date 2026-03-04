package effects

import (
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
)

// --- renderMessages ---

func TestRenderMessages_Basic(t *testing.T) {
	c := chat.New(
		message.NewText("", role.System, "You are helpful."),
		message.NewText("user", role.User, "Hello"),
		message.NewText("bot", role.Assistant, "Hi there!"),
	)

	result := renderMessages(c.Messages())
	assert.NotContains(t, result, "You are helpful.")
	assert.Contains(t, result, "[user] Hello")
	assert.Contains(t, result, "[assistant] Hi there!")
}

func TestRenderMessages_ToolCalls(t *testing.T) {
	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"/foo/bar.go"}`},
		),
		message.New("bot", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "file contents here"},
		),
	)

	result := renderMessages(c.Messages())
	assert.Contains(t, result, `[assistant] Called tool read_file({"path":"/foo/bar.go"})`)
	assert.Contains(t, result, "[tool result] file contents here")
}

func TestRenderMessages_ToolError(t *testing.T) {
	c := chat.New(
		message.New("bot", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "not found", IsError: true},
		),
	)

	result := renderMessages(c.Messages())
	assert.Contains(t, result, "[tool error] not found")
}

func TestRenderMessages_TruncatesLongArgs(t *testing.T) {
	longArgs := strings.Repeat("x", 300)
	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "test", Arguments: longArgs},
		),
	)

	result := renderMessages(c.Messages())
	assert.Contains(t, result, strings.Repeat("x", 200)+"\u2026")
}

func TestRenderMessages_TruncatesLongResults(t *testing.T) {
	longResult := strings.Repeat("y", 600)
	c := chat.New(
		message.New("bot", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longResult},
		),
	)

	result := renderMessages(c.Messages())
	assert.Contains(t, result, strings.Repeat("y", 500)+"\u2026")
}

// --- truncate ---

func TestTruncate_Short(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 10))
}

func TestTruncate_Exact(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 3))
}

func TestTruncate_Long(t *testing.T) {
	assert.Equal(t, "ab\u2026", truncate("abcdef", 2))
}

func TestTruncate_MultiByte(t *testing.T) {
	// 5 runes: こ ん に ち は (each 3 bytes in UTF-8)
	s := "こんにちは"
	result := truncate(s, 3)
	assert.Equal(t, "こんに\u2026", result)

	// At exact rune length — no truncation.
	assert.Equal(t, s, truncate(s, 5))

	// Shorter than limit — returned as-is.
	assert.Equal(t, s, truncate(s, 10))
}
