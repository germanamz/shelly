package chat

import (
	"testing"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	m1 := message.NewText("alice", role.User, "hello")
	m2 := message.NewText("bot", role.Assistant, "hi")
	c := New(m1, m2)

	assert.Equal(t, 2, c.Len())
}

func TestChat_ZeroValue(t *testing.T) {
	var c Chat

	assert.Equal(t, 0, c.Len())

	_, ok := c.Last()
	assert.False(t, ok)
	assert.Empty(t, c.Messages())
}

func TestChat_Append(t *testing.T) {
	c := New()
	c.Append(message.NewText("alice", role.User, "one"))
	c.Append(
		message.NewText("bot", role.Assistant, "two"),
		message.NewText("alice", role.User, "three"),
	)

	assert.Equal(t, 3, c.Len())
}

func TestChat_At(t *testing.T) {
	m := message.NewText("alice", role.User, "hello")
	c := New(m)

	got := c.At(0)
	assert.Equal(t, role.User, got.Role)
	assert.Equal(t, "hello", got.TextContent())
}

func TestChat_At_Panics(t *testing.T) {
	c := New()
	assert.Panics(t, func() { c.At(0) })
}

func TestChat_Last(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "first"),
		message.NewText("bot", role.Assistant, "second"),
	)

	msg, ok := c.Last()
	assert.True(t, ok)
	assert.Equal(t, "second", msg.TextContent())
}

func TestChat_Last_Empty(t *testing.T) {
	c := New()

	_, ok := c.Last()
	assert.False(t, ok)
}

func TestChat_Messages_ReturnsCopy(t *testing.T) {
	c := New(message.NewText("alice", role.User, "hello"))

	msgs := c.Messages()
	msgs[0] = message.NewText("bot", role.Assistant, "modified")

	assert.Equal(t, "hello", c.At(0).TextContent())
}

func TestChat_Each(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "a"),
		message.NewText("bot", role.Assistant, "b"),
		message.NewText("alice", role.User, "c"),
	)

	var visited []string
	c.Each(func(_ int, m message.Message) bool {
		visited = append(visited, m.TextContent())
		return true
	})

	assert.Equal(t, []string{"a", "b", "c"}, visited)
}

func TestChat_Each_EarlyStop(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "a"),
		message.NewText("bot", role.Assistant, "b"),
		message.NewText("alice", role.User, "c"),
	)

	var visited []string
	c.Each(func(_ int, m message.Message) bool {
		visited = append(visited, m.TextContent())
		return len(visited) < 2
	})

	assert.Equal(t, []string{"a", "b"}, visited)
}

func TestChat_BySender(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "hello"),
		message.NewText("bot", role.Assistant, "hi"),
		message.NewText("alice", role.User, "how are you?"),
		message.NewText("bot", role.Assistant, "great!"),
	)

	msgs := c.BySender("alice")

	assert.Len(t, msgs, 2)
	assert.Equal(t, "hello", msgs[0].TextContent())
	assert.Equal(t, "how are you?", msgs[1].TextContent())
}

func TestChat_BySender_NoMatch(t *testing.T) {
	c := New(message.NewText("alice", role.User, "hello"))

	assert.Empty(t, c.BySender("bob"))
}

func TestChat_BySender_Empty(t *testing.T) {
	c := New()

	assert.Empty(t, c.BySender("alice"))
}

func TestChat_SystemPrompt(t *testing.T) {
	c := New(
		message.NewText("", role.System, "you are helpful"),
		message.NewText("alice", role.User, "hello"),
	)

	assert.Equal(t, "you are helpful", c.SystemPrompt())
}

func TestChat_SystemPrompt_None(t *testing.T) {
	c := New(message.NewText("alice", role.User, "hello"))

	assert.Empty(t, c.SystemPrompt())
}

func TestChat_SystemPrompt_NotFirst(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "hello"),
		message.NewText("", role.System, "system msg"),
	)

	assert.Equal(t, "system msg", c.SystemPrompt())
}
