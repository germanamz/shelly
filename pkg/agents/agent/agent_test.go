package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/content"
	"github.com/germanamz/shelly/pkg/chatty/message"
	"github.com/germanamz/shelly/pkg/chatty/role"
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

func TestNew(t *testing.T) {
	p := &mockProvider{}
	c := chat.New()
	tb := newTestToolBox()

	a := New("bot", p, c, tb)

	assert.Equal(t, "bot", a.Name)
	assert.Equal(t, p, a.ModelAdapter)
	assert.Equal(t, c, a.Chat)
	assert.Len(t, a.ToolBoxes, 1)
}

func TestNewNoToolBoxes(t *testing.T) {
	a := New("bot", &mockProvider{}, chat.New())

	assert.Empty(t, a.ToolBoxes)
}

func TestComplete(t *testing.T) {
	p := &mockProvider{
		reply: message.NewText("", role.Assistant, "Hello!"),
	}
	c := chat.New(
		message.NewText("user", role.User, "Hi"),
	)
	a := New("bot", p, c)

	reply, err := a.Complete(context.Background())

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
	a := New("myagent", p, chat.New())

	reply, err := a.Complete(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "myagent", reply.Sender)

	last, ok := a.Chat.Last()
	require.True(t, ok)
	assert.Equal(t, "myagent", last.Sender)
}

func TestCompleteError(t *testing.T) {
	p := &mockProvider{
		err: errors.New("provider failed"),
	}
	a := New("bot", p, chat.New())

	_, err := a.Complete(context.Background())

	require.EqualError(t, err, "provider failed")
	assert.Equal(t, 0, a.Chat.Len())
}

func TestCallTools(t *testing.T) {
	tb := newTestToolBox()
	a := New("bot", &mockProvider{}, chat.New(), tb)

	msg := message.New("bot", role.Assistant,
		content.Text{Text: "Let me call a tool."},
		content.ToolCall{ID: "call-1", Name: "echo", Arguments: `{"text":"hi"}`},
	)

	results := a.CallTools(context.Background(), msg)

	require.Len(t, results, 1)
	assert.Equal(t, "call-1", results[0].ToolCallID)
	assert.JSONEq(t, `{"text":"hi"}`, results[0].Content)
	assert.False(t, results[0].IsError)
	assert.Equal(t, 1, a.Chat.Len())
}

func TestCallToolsNoToolCalls(t *testing.T) {
	a := New("bot", &mockProvider{}, chat.New(), newTestToolBox())

	msg := message.NewText("bot", role.Assistant, "No tools needed.")
	results := a.CallTools(context.Background(), msg)

	assert.Nil(t, results)
	assert.Equal(t, 0, a.Chat.Len())
}

func TestCallToolsNotFound(t *testing.T) {
	a := New("bot", &mockProvider{}, chat.New(), newTestToolBox())

	msg := message.New("bot", role.Assistant,
		content.ToolCall{ID: "call-1", Name: "missing", Arguments: `{}`},
	)

	results := a.CallTools(context.Background(), msg)

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

	a := New("bot", &mockProvider{}, chat.New(), tb1, tb2)

	msg := message.New("bot", role.Assistant,
		content.ToolCall{ID: "c1", Name: "greet", Arguments: `{}`},
		content.ToolCall{ID: "c2", Name: "echo", Arguments: `{"x":1}`},
	)

	results := a.CallTools(context.Background(), msg)

	require.Len(t, results, 2)
	assert.Equal(t, "hello", results[0].Content)
	assert.False(t, results[0].IsError)
	assert.Equal(t, `{"x":1}`, results[1].Content)
	assert.False(t, results[1].IsError)
	assert.Equal(t, 2, a.Chat.Len())
}

func TestCallToolsAppendsSender(t *testing.T) {
	a := New("myagent", &mockProvider{}, chat.New(), newTestToolBox())

	msg := message.New("myagent", role.Assistant,
		content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
	)
	a.CallTools(context.Background(), msg)

	last, ok := a.Chat.Last()
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

	a := New("bot", &mockProvider{}, chat.New(), tb1, tb2)

	tools := a.Tools()
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
	a := New("bot", &mockProvider{}, chat.New())

	assert.Empty(t, a.Tools())
}
