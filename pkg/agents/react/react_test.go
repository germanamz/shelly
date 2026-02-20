package react

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/agents"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sequenceProvider returns a sequence of preconfigured replies.
type sequenceProvider struct {
	replies []message.Message
	index   int
}

func (p *sequenceProvider) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	if p.index >= len(p.replies) {
		return message.Message{}, errors.New("no more replies")
	}
	reply := p.replies[p.index]
	p.index++
	return reply, nil
}

// errorProvider always returns an error.
type errorProvider struct {
	err error
}

func (p *errorProvider) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return message.Message{}, p.err
}

func newEchoToolBox() *toolbox.ToolBox {
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

func TestRunNoToolCalls(t *testing.T) {
	p := &sequenceProvider{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Done."),
		},
	}
	base := agents.NewAgentBase("bot", p, chat.New())
	a := New(base, Options{})

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Done.", result.TextContent())
	assert.Equal(t, "bot", result.Sender)
}

func TestRunSingleIteration(t *testing.T) {
	p := &sequenceProvider{
		replies: []message.Message{
			// First reply: tool call
			message.New("", role.Assistant,
				content.Text{Text: "Calling tool."},
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{"msg":"hi"}`},
			),
			// Second reply: final answer
			message.NewText("", role.Assistant, "Got the result."),
		},
	}
	base := agents.NewAgentBase("bot", p, chat.New(), newEchoToolBox())
	a := New(base, Options{})

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Got the result.", result.TextContent())
	assert.Equal(t, 2, p.index)
}

func TestRunMultipleIterations(t *testing.T) {
	p := &sequenceProvider{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{"step":1}`},
			),
			message.New("", role.Assistant,
				content.ToolCall{ID: "c2", Name: "echo", Arguments: `{"step":2}`},
			),
			message.NewText("", role.Assistant, "All done."),
		},
	}
	base := agents.NewAgentBase("bot", p, chat.New(), newEchoToolBox())
	a := New(base, Options{})

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "All done.", result.TextContent())
	assert.Equal(t, 3, p.index)
}

func TestRunMaxIterations(t *testing.T) {
	p := &sequenceProvider{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
			),
			message.New("", role.Assistant,
				content.ToolCall{ID: "c2", Name: "echo", Arguments: `{}`},
			),
			message.New("", role.Assistant,
				content.ToolCall{ID: "c3", Name: "echo", Arguments: `{}`},
			),
		},
	}
	base := agents.NewAgentBase("bot", p, chat.New(), newEchoToolBox())
	a := New(base, Options{MaxIterations: 2})

	_, err := a.Run(context.Background())

	require.ErrorIs(t, err, ErrMaxIterations)
	assert.Equal(t, 2, p.index)
}

func TestRunProviderError(t *testing.T) {
	p := &errorProvider{err: errors.New("api error")}
	base := agents.NewAgentBase("bot", p, chat.New())
	a := New(base, Options{})

	_, err := a.Run(context.Background())

	assert.EqualError(t, err, "api error")
}

func TestRunProviderErrorAfterToolCall(t *testing.T) {
	p := &sequenceProvider{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
			),
		},
	}
	base := agents.NewAgentBase("bot", p, chat.New(), newEchoToolBox())
	a := New(base, Options{})

	_, err := a.Run(context.Background())

	assert.EqualError(t, err, "no more replies")
}

func TestRunContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &errorProvider{err: ctx.Err()}
	base := agents.NewAgentBase("bot", p, chat.New())
	a := New(base, Options{})

	_, err := a.Run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}
