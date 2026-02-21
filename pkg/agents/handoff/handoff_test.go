package handoff

import (
	"context"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/reactor"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAgent implements reactor.NamedAgent for testing. On each Run call it
// returns the next preconfigured reply or error.
type mockAgent struct {
	name    string
	chat    *chat.Chat
	replies []mockReply
	index   int
}

type mockReply struct {
	msg message.Message
	err error
}

func (m *mockAgent) Run(_ context.Context) (message.Message, error) {
	if m.index >= len(m.replies) {
		return message.Message{}, errors.New("no more replies")
	}

	r := m.replies[m.index]
	m.index++

	if r.err != nil {
		return message.Message{}, r.err
	}

	reply := r.msg
	reply.Sender = m.name
	m.chat.Append(reply)

	return reply, nil
}

func (m *mockAgent) AgentName() string     { return m.name }
func (m *mockAgent) AgentChat() *chat.Chat { return m.chat }

// factory returns an AgentFactory that ignores the shared chat and transfer
// tools, returning a pre-built mock agent. This is useful when we want full
// control of mock behavior (including triggering HandoffErrors directly).
func factory(agent reactor.NamedAgent) AgentFactory {
	return func(_ *chat.Chat, _ *toolbox.ToolBox) reactor.NamedAgent {
		return agent
	}
}

func TestNewNoMembers(t *testing.T) {
	_, err := New("h", chat.New(), nil, Options{})
	require.ErrorIs(t, err, ErrNoMembers)
}

func TestNewEmptyMembers(t *testing.T) {
	_, err := New("h", chat.New(), []Member{}, Options{})
	require.ErrorIs(t, err, ErrNoMembers)
}

func TestSingleAgentNoHandoff(t *testing.T) {
	agent := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{msg: message.NewText("", role.Assistant, "Hello!")},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agent)},
	}, Options{})
	require.NoError(t, err)

	result, err := h.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Hello!", result.TextContent())
}

func TestSingleHandoff(t *testing.T) {
	agentA := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "B"}},
		},
	}
	agentB := &mockAgent{
		name: "B",
		chat: chat.New(),
		replies: []mockReply{
			{msg: message.NewText("", role.Assistant, "B here!")},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agentA)},
		{Name: "B", Factory: factory(agentB)},
	}, Options{})
	require.NoError(t, err)

	result, err := h.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "B here!", result.TextContent())
}

func TestChainOfHandoffs(t *testing.T) {
	agentA := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "B"}},
		},
	}
	agentB := &mockAgent{
		name: "B",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "C"}},
		},
	}
	agentC := &mockAgent{
		name: "C",
		chat: chat.New(),
		replies: []mockReply{
			{msg: message.NewText("", role.Assistant, "C done!")},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agentA)},
		{Name: "B", Factory: factory(agentB)},
		{Name: "C", Factory: factory(agentC)},
	}, Options{})
	require.NoError(t, err)

	result, err := h.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "C done!", result.TextContent())
}

func TestMaxHandoffsExceeded(t *testing.T) {
	// A and B keep handing off to each other forever.
	agentA := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "B"}},
			{err: &HandoffError{Target: "B"}},
			{err: &HandoffError{Target: "B"}},
		},
	}
	agentB := &mockAgent{
		name: "B",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "A"}},
			{err: &HandoffError{Target: "A"}},
			{err: &HandoffError{Target: "A"}},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agentA)},
		{Name: "B", Factory: factory(agentB)},
	}, Options{MaxHandoffs: 3})
	require.NoError(t, err)

	_, err = h.Run(context.Background())

	require.ErrorIs(t, err, ErrMaxHandoffs)
}

func TestHandoffToUnknownTarget(t *testing.T) {
	agent := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "nonexistent"}},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agent)},
	}, Options{})
	require.NoError(t, err)

	_, err = h.Run(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown target agent")
}

func TestAgentErrorPropagation(t *testing.T) {
	agentErr := errors.New("agent crashed")
	agent := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{err: agentErr},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agent)},
	}, Options{})
	require.NoError(t, err)

	_, err = h.Run(context.Background())

	require.ErrorIs(t, err, agentErr)
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	agent := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{err: context.Canceled},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agent)},
	}, Options{})
	require.NoError(t, err)

	_, err = h.Run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestHandoffAgentName(t *testing.T) {
	agent := &mockAgent{name: "A", chat: chat.New(), replies: []mockReply{
		{msg: message.NewText("", role.Assistant, "ok")},
	}}
	shared := chat.New()

	h, err := New("my-handoff", shared, []Member{
		{Name: "A", Factory: factory(agent)},
	}, Options{})
	require.NoError(t, err)

	assert.Equal(t, "my-handoff", h.AgentName())
}

func TestHandoffAgentChat(t *testing.T) {
	agent := &mockAgent{name: "A", chat: chat.New(), replies: []mockReply{
		{msg: message.NewText("", role.Assistant, "ok")},
	}}
	shared := chat.New()

	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agent)},
	}, Options{})
	require.NoError(t, err)

	assert.Same(t, shared, h.AgentChat())
}

func TestTransferToolBoxContainsAllMembers(t *testing.T) {
	members := []Member{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}

	tb := buildTransferToolBox(members)
	tools := tb.Tools()

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	assert.True(t, names["transfer_to_alpha"])
	assert.True(t, names["transfer_to_beta"])
	assert.True(t, names["transfer_to_gamma"])
	assert.Len(t, tools, 3)
}

func TestTransferToolHandlerReturnsHandoffError(t *testing.T) {
	members := []Member{
		{Name: "target"},
	}

	tb := buildTransferToolBox(members)
	tool, ok := tb.Get("transfer_to_target")
	require.True(t, ok)

	_, err := tool.Handler(context.Background(), nil)

	var he *HandoffError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, "target", he.Target)
}

func TestHandoffErrorMessage(t *testing.T) {
	err := &HandoffError{Target: "agent-b"}
	assert.Equal(t, `handoff to "agent-b"`, err.Error())
}

func TestFactoryReceivesSharedChatAndTools(t *testing.T) {
	var receivedChat *chat.Chat
	var receivedTB *toolbox.ToolBox

	shared := chat.New()
	_, err := New("h", shared, []Member{
		{
			Name: "A",
			Factory: func(c *chat.Chat, tb *toolbox.ToolBox) reactor.NamedAgent {
				receivedChat = c
				receivedTB = tb
				return &mockAgent{name: "A", chat: c, replies: []mockReply{
					{msg: message.NewText("", role.Assistant, "ok")},
				}}
			},
		},
	}, Options{})
	require.NoError(t, err)

	assert.Same(t, shared, receivedChat)
	assert.NotNil(t, receivedTB)

	// The transfer toolbox should have a tool for "A".
	_, ok := receivedTB.Get("transfer_to_A")
	assert.True(t, ok)
}

func TestHandoffBackAndForth(t *testing.T) {
	// A hands off to B, B does work and hands back to A, A finishes.
	agentA := &mockAgent{
		name: "A",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "B"}},
			{msg: message.NewText("", role.Assistant, "A finished after B")},
		},
	}
	agentB := &mockAgent{
		name: "B",
		chat: chat.New(),
		replies: []mockReply{
			{err: &HandoffError{Target: "A"}},
		},
	}

	shared := chat.New()
	h, err := New("handoff", shared, []Member{
		{Name: "A", Factory: factory(agentA)},
		{Name: "B", Factory: factory(agentB)},
	}, Options{})
	require.NoError(t, err)

	result, err := h.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "A finished after B", result.TextContent())
}

func TestFirstMemberIsInitiallyActive(t *testing.T) {
	shared := chat.New()
	h, err := New("h", shared, []Member{
		{Name: "first", Factory: func(_ *chat.Chat, _ *toolbox.ToolBox) reactor.NamedAgent {
			return &mockAgent{name: "first", chat: chat.New(), replies: []mockReply{
				{msg: message.NewText("", role.Assistant, "first active")},
			}}
		}},
		{Name: "second", Factory: func(_ *chat.Chat, _ *toolbox.ToolBox) reactor.NamedAgent {
			return &mockAgent{name: "second", chat: chat.New(), replies: []mockReply{
				{msg: message.NewText("", role.Assistant, "second active")},
			}}
		}},
	}, Options{})
	require.NoError(t, err)

	result, err := h.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "first active", result.TextContent())
}
