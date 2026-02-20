package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a test provider that returns a preconfigured reply.
type mockProvider struct {
	reply message.Message
	err   error
	calls int
}

func (m *mockProvider) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	m.calls++
	return m.reply, m.err
}

func newTestToolBox() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(toolbox.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return string(input), nil
		},
	})

	return tb
}

func TestNewAgentBase(t *testing.T) {
	p := &mockProvider{}
	c := chat.New()
	tb := newTestToolBox()

	b := NewAgentBase("bot", p, c, tb)

	assert.Equal(t, "bot", b.Name)
	assert.Equal(t, p, b.ModelAdapter)
	assert.Equal(t, c, b.Chat)
	assert.Len(t, b.ToolBoxes, 1)
}

func TestNewAgentBaseNoToolBoxes(t *testing.T) {
	b := NewAgentBase("bot", &mockProvider{}, chat.New())

	assert.Empty(t, b.ToolBoxes)
}

func TestComplete(t *testing.T) {
	p := &mockProvider{
		reply: message.NewText("", role.Assistant, "Hello!"),
	}
	c := chat.New(
		message.NewText("user", role.User, "Hi"),
	)
	b := NewAgentBase("bot", p, c)

	reply, err := b.Complete(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "bot", reply.Sender)
	assert.Equal(t, "Hello!", reply.TextContent())
	assert.Equal(t, 2, c.Len())
	assert.Equal(t, 1, p.calls)
}

func TestCompleteSetsSender(t *testing.T) {
	p := &mockProvider{
		reply: message.NewText("other", role.Assistant, "Reply"),
	}
	b := NewAgentBase("myagent", p, chat.New())

	reply, err := b.Complete(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "myagent", reply.Sender)

	last, ok := b.Chat.Last()
	require.True(t, ok)
	assert.Equal(t, "myagent", last.Sender)
}

func TestCompleteError(t *testing.T) {
	p := &mockProvider{
		err: errors.New("provider failed"),
	}
	b := NewAgentBase("bot", p, chat.New())

	_, err := b.Complete(context.Background())

	require.EqualError(t, err, "provider failed")
	assert.Equal(t, 0, b.Chat.Len())
}

func TestCallTools(t *testing.T) {
	tb := newTestToolBox()
	b := NewAgentBase("bot", &mockProvider{}, chat.New(), tb)

	msg := message.New("bot", role.Assistant,
		content.Text{Text: "Let me call a tool."},
		content.ToolCall{ID: "call-1", Name: "echo", Arguments: `{"text":"hi"}`},
	)

	results := b.CallTools(context.Background(), msg)

	require.Len(t, results, 1)
	assert.Equal(t, "call-1", results[0].ToolCallID)
	assert.JSONEq(t, `{"text":"hi"}`, results[0].Content)
	assert.False(t, results[0].IsError)
	assert.Equal(t, 1, b.Chat.Len())
}

func TestCallToolsNoToolCalls(t *testing.T) {
	b := NewAgentBase("bot", &mockProvider{}, chat.New(), newTestToolBox())

	msg := message.NewText("bot", role.Assistant, "No tools needed.")
	results := b.CallTools(context.Background(), msg)

	assert.Nil(t, results)
	assert.Equal(t, 0, b.Chat.Len())
}

func TestCallToolsNotFound(t *testing.T) {
	b := NewAgentBase("bot", &mockProvider{}, chat.New(), newTestToolBox())

	msg := message.New("bot", role.Assistant,
		content.ToolCall{ID: "call-1", Name: "missing", Arguments: `{}`},
	)

	results := b.CallTools(context.Background(), msg)

	require.Len(t, results, 1)
	assert.True(t, results[0].IsError)
	assert.Contains(t, results[0].Content, "tool not found: missing")
}

func TestCallToolsMultipleToolBoxes(t *testing.T) {
	tb1 := toolbox.New()
	tb1.Register(toolbox.Tool{
		Name:        "greet",
		Description: "Greets",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "hello", nil
		},
	})

	tb2 := newTestToolBox() // has "echo"

	b := NewAgentBase("bot", &mockProvider{}, chat.New(), tb1, tb2)

	msg := message.New("bot", role.Assistant,
		content.ToolCall{ID: "c1", Name: "greet", Arguments: `{}`},
		content.ToolCall{ID: "c2", Name: "echo", Arguments: `{"x":1}`},
	)

	results := b.CallTools(context.Background(), msg)

	require.Len(t, results, 2)
	assert.Equal(t, "hello", results[0].Content)
	assert.False(t, results[0].IsError)
	assert.Equal(t, `{"x":1}`, results[1].Content)
	assert.False(t, results[1].IsError)
	assert.Equal(t, 2, b.Chat.Len())
}

func TestCallToolsAppendsSender(t *testing.T) {
	b := NewAgentBase("myagent", &mockProvider{}, chat.New(), newTestToolBox())

	msg := message.New("myagent", role.Assistant,
		content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
	)
	b.CallTools(context.Background(), msg)

	last, ok := b.Chat.Last()
	require.True(t, ok)
	assert.Equal(t, "myagent", last.Sender)
	assert.Equal(t, role.Tool, last.Role)
}

func TestTools(t *testing.T) {
	tb1 := toolbox.New()
	tb1.Register(toolbox.Tool{Name: "a"})
	tb1.Register(toolbox.Tool{Name: "b"})

	tb2 := toolbox.New()
	tb2.Register(toolbox.Tool{Name: "c"})

	b := NewAgentBase("bot", &mockProvider{}, chat.New(), tb1, tb2)

	tools := b.Tools()
	assert.Len(t, tools, 3)

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	assert.True(t, names["a"])
	assert.True(t, names["b"])
	assert.True(t, names["c"])
}

func TestToolsEmpty(t *testing.T) {
	b := NewAgentBase("bot", &mockProvider{}, chat.New())

	assert.Empty(t, b.Tools())
}
