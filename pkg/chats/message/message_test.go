package message

import (
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/role"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	msg := New("alice", role.User, content.Text{Text: "hello"}, content.Image{URL: "img.png"})

	assert.Equal(t, "alice", msg.Sender)
	assert.Equal(t, role.User, msg.Role)
	assert.Len(t, msg.Parts, 2)
	assert.Nil(t, msg.Metadata)
}

func TestNewText(t *testing.T) {
	msg := NewText("bob", role.Assistant, "hi there")

	assert.Equal(t, "bob", msg.Sender)
	assert.Equal(t, role.Assistant, msg.Role)
	assert.Len(t, msg.Parts, 1)
	assert.Equal(t, "hi there", msg.Parts[0].(content.Text).Text)
}

func TestMessage_Sender_ZeroValue(t *testing.T) {
	var msg Message

	assert.Empty(t, msg.Sender)
}

func TestMessage_TextContent(t *testing.T) {
	msg := New("alice", role.User,
		content.Text{Text: "hello "},
		content.Image{URL: "img.png"},
		content.Text{Text: "world"},
	)

	assert.Equal(t, "hello world", msg.TextContent())
}

func TestMessage_TextContent_NoParts(t *testing.T) {
	msg := New("alice", role.User)
	assert.Empty(t, msg.TextContent())
}

func TestMessage_ToolCalls(t *testing.T) {
	tc1 := content.ToolCall{ID: "1", Name: "search", Arguments: `{"q":"go"}`}
	tc2 := content.ToolCall{ID: "2", Name: "read", Arguments: `{"file":"main.go"}`}
	msg := New("bot", role.Assistant,
		content.Text{Text: "let me help"},
		tc1,
		tc2,
	)

	calls := msg.ToolCalls()
	assert.Len(t, calls, 2)
	assert.Equal(t, tc1, calls[0])
	assert.Equal(t, tc2, calls[1])
}

func TestMessage_ToolCalls_None(t *testing.T) {
	msg := NewText("alice", role.User, "hello")
	assert.Empty(t, msg.ToolCalls())
}

func TestMessage_SetMeta_GetMeta(t *testing.T) {
	msg := NewText("alice", role.User, "hello")

	msg.SetMeta("model", "gpt-4")
	msg.SetMeta("tokens", 42)

	v, ok := msg.GetMeta("model")
	assert.True(t, ok)
	assert.Equal(t, "gpt-4", v)

	v, ok = msg.GetMeta("tokens")
	assert.True(t, ok)
	assert.Equal(t, 42, v)
}

func TestMessage_GetMeta_Missing(t *testing.T) {
	msg := NewText("alice", role.User, "hello")

	v, ok := msg.GetMeta("nope")
	assert.False(t, ok)
	assert.Nil(t, v)
}

func TestMessage_SetMeta_Overwrite(t *testing.T) {
	msg := NewText("alice", role.User, "hello")

	msg.SetMeta("key", "old")
	msg.SetMeta("key", "new")

	v, ok := msg.GetMeta("key")
	assert.True(t, ok)
	assert.Equal(t, "new", v)
}
