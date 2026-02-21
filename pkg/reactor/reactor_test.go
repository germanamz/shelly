package reactor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/agents"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface checks.
var (
	_ agents.Agent = (*Reactor)(nil)
	_ NamedAgent   = (*Reactor)(nil)
)

// mockAgent implements NamedAgent for testing. It returns preconfigured replies
// and appends each reply to its private chat (simulating AgentBase.Complete).
type mockAgent struct {
	name    string
	chat    *chat.Chat
	replies []message.Message
	index   int
}

func (m *mockAgent) Run(_ context.Context) (message.Message, error) {
	if m.index >= len(m.replies) {
		return message.Message{}, errors.New("no more replies")
	}

	reply := m.replies[m.index]
	reply.Sender = m.name
	m.index++
	m.chat.Append(reply)

	return reply, nil
}

func (m *mockAgent) AgentName() string     { return m.name }
func (m *mockAgent) AgentChat() *chat.Chat { return m.chat }

func newMockAgent(name string, replies ...message.Message) *mockAgent {
	return &mockAgent{
		name:    name,
		chat:    chat.New(),
		replies: replies,
	}
}

// errorAgent always returns an error from Run.
type errorAgent struct {
	name string
	chat *chat.Chat
	err  error
}

func (e *errorAgent) Run(_ context.Context) (message.Message, error) {
	return message.Message{}, e.err
}

func (e *errorAgent) AgentName() string     { return e.name }
func (e *errorAgent) AgentChat() *chat.Chat { return e.chat }

// member wraps a NamedAgent in a TeamMember with the given role.
func member(a NamedAgent, r TeamRole) TeamMember {
	return TeamMember{Agent: a, Role: r}
}

func TestNewNoMembers(t *testing.T) {
	_, err := New("r", chat.New(), nil, Options{Coordinator: NewSequence()})

	require.ErrorIs(t, err, ErrNoMembers)
}

func TestTwoMemberSequence(t *testing.T) {
	a := newMockAgent("A", message.NewText("", role.Assistant, "Hello from A"))
	b := newMockAgent("B", message.NewText("", role.Assistant, "Hello from B"))

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{
		member(a, "worker"),
		member(b, "worker"),
	}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	result, err := r.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "Hello from B", result.TextContent())
	assert.Equal(t, 2, shared.Len())
	assert.Equal(t, "Hello from A", shared.At(0).TextContent())
	assert.Equal(t, "Hello from B", shared.At(1).TextContent())
}

func TestSharedChatSync(t *testing.T) {
	a := newMockAgent("A", message.NewText("", role.Assistant, "Reply from A"))
	b := newMockAgent("B", message.NewText("", role.Assistant, "Reply from B"))

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{
		member(a, "worker"),
		member(b, "worker"),
	}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.NoError(t, err)

	// B's private chat should contain A's reply (synced as User) + B's own reply.
	require.Equal(t, 2, b.AgentChat().Len())

	synced := b.AgentChat().At(0)
	assert.Equal(t, "A", synced.Sender)
	assert.Equal(t, role.User, synced.Role)
	assert.Equal(t, "Reply from A", synced.TextContent())
}

func TestSkipSelfMessages(t *testing.T) {
	a := newMockAgent("A",
		message.NewText("", role.Assistant, "First"),
		message.NewText("", role.Assistant, "Second"),
	)

	step := 0
	coord := &funcCoordinator{fn: func(_ context.Context, _ *chat.Chat, _ []TeamMember) (Selection, error) {
		if step >= 2 {
			return Selection{Done: true}, nil
		}
		step++

		return Selection{Members: []int{0}}, nil
	}}

	shared := chat.New(message.NewText("user", role.User, "Hello"))
	r, err := New("reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: coord})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.NoError(t, err)

	// Agent's chat should have:
	// 1. Synced "Hello" from user (before first run)
	// 2. "First" (agent's own reply from first run)
	// 3. "Second" (agent's own reply from second run)
	// A's own "First" in shared chat should NOT be synced back.
	assert.Equal(t, 3, a.AgentChat().Len())
}

func TestRoleRemapping(t *testing.T) {
	a := newMockAgent("A", message.NewText("", role.Assistant, "assistant reply"))
	b := newMockAgent("B", message.NewText("", role.Assistant, "done"))

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{
		member(a, "worker"),
		member(b, "worker"),
	}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.NoError(t, err)

	// A's reply was originally Assistant role, synced to B as User.
	synced := b.AgentChat().At(0)
	assert.Equal(t, role.User, synced.Role)
}

func TestMetadataPreservation(t *testing.T) {
	reply := message.NewText("", role.Assistant, "with meta")
	reply.SetMeta("key", "value")

	a := newMockAgent("A", reply)
	b := newMockAgent("B", message.NewText("", role.Assistant, "done"))

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{
		member(a, "worker"),
		member(b, "worker"),
	}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.NoError(t, err)

	synced := b.AgentChat().At(0)
	val, ok := synced.GetMeta("key")
	assert.True(t, ok)
	assert.Equal(t, "value", val)
}

func TestAgentErrorPropagation(t *testing.T) {
	agentErr := errors.New("agent failed")
	a := &errorAgent{name: "A", chat: chat.New(), err: agentErr}

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.ErrorIs(t, err, agentErr)
	assert.Contains(t, err.Error(), `agent "A"`)
}

func TestCoordinatorErrorPropagation(t *testing.T) {
	coordErr := errors.New("coordination failed")
	coord := &funcCoordinator{fn: func(_ context.Context, _ *chat.Chat, _ []TeamMember) (Selection, error) {
		return Selection{}, coordErr
	}}

	a := newMockAgent("A", message.NewText("", role.Assistant, "x"))
	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: coord})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.ErrorIs(t, err, coordErr)
	assert.Contains(t, err.Error(), "coordinator")
}

func TestInvalidMemberIndex(t *testing.T) {
	coord := &funcCoordinator{fn: func(_ context.Context, _ *chat.Chat, _ []TeamMember) (Selection, error) {
		return Selection{Members: []int{5}}, nil
	}}

	a := newMockAgent("A", message.NewText("", role.Assistant, "x"))
	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: coord})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid member index 5")
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := &errorAgent{name: "A", chat: chat.New(), err: context.Canceled}
	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	_, err = r.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestInitialSharedMessages(t *testing.T) {
	a := newMockAgent("A", message.NewText("", role.Assistant, "got it"))

	shared := chat.New(message.NewText("user", role.User, "Hello!"))
	r, err := New("reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.NoError(t, err)

	// A's private chat should have the initial message (synced) + its own reply.
	require.Equal(t, 2, a.AgentChat().Len())

	synced := a.AgentChat().At(0)
	assert.Equal(t, "Hello!", synced.TextContent())
	assert.Equal(t, role.User, synced.Role)
	assert.Equal(t, "user", synced.Sender)
}

func TestReactorAgentName(t *testing.T) {
	a := newMockAgent("A", message.NewText("", role.Assistant, "x"))
	shared := chat.New()

	r, err := New("my-reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	assert.Equal(t, "my-reactor", r.AgentName())
}

func TestReactorAgentChat(t *testing.T) {
	a := newMockAgent("A", message.NewText("", role.Assistant, "x"))
	shared := chat.New()

	r, err := New("reactor", shared, []TeamMember{member(a, "worker")}, Options{Coordinator: NewSequence()})
	require.NoError(t, err)

	assert.Same(t, shared, r.AgentChat())
}

// concurrentMockAgent is like mockAgent but tracks concurrent execution.
type concurrentMockAgent struct {
	name    string
	chat    *chat.Chat
	reply   message.Message
	running *atomic.Int32
	maxSeen *atomic.Int32
}

func (c *concurrentMockAgent) Run(_ context.Context) (message.Message, error) {
	cur := c.running.Add(1)

	// Track maximum concurrency observed.
	for {
		old := c.maxSeen.Load()
		if cur <= old || c.maxSeen.CompareAndSwap(old, cur) {
			break
		}
	}

	// Brief sleep to give goroutines a chance to overlap.
	time.Sleep(10 * time.Millisecond)
	c.running.Add(-1)

	reply := c.reply
	reply.Sender = c.name
	c.chat.Append(reply)

	return reply, nil
}

func (c *concurrentMockAgent) AgentName() string     { return c.name }
func (c *concurrentMockAgent) AgentChat() *chat.Chat { return c.chat }

func TestConcurrentExecution(t *testing.T) {
	running := &atomic.Int32{}
	maxSeen := &atomic.Int32{}

	a := &concurrentMockAgent{
		name: "A", chat: chat.New(),
		reply: message.NewText("", role.Assistant, "A done"), running: running, maxSeen: maxSeen,
	}
	b := &concurrentMockAgent{
		name: "B", chat: chat.New(),
		reply: message.NewText("", role.Assistant, "B done"), running: running, maxSeen: maxSeen,
	}

	// Coordinator that selects both members at once, then signals done.
	step := 0
	coord := &funcCoordinator{fn: func(_ context.Context, _ *chat.Chat, _ []TeamMember) (Selection, error) {
		if step > 0 {
			return Selection{Done: true}, nil
		}
		step++

		return Selection{Members: []int{0, 1}}, nil
	}}

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{
		member(a, "worker"),
		member(b, "worker"),
	}, Options{Coordinator: coord})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int32(2), maxSeen.Load(), "both agents should run concurrently")
	assert.Equal(t, 2, shared.Len())
}

func TestConcurrentErrorCancelsOthers(t *testing.T) {
	agentErr := errors.New("agent B failed")
	a := newMockAgent("A", message.NewText("", role.Assistant, "A done"))
	b := &errorAgent{name: "B", chat: chat.New(), err: agentErr}

	step := 0
	coord := &funcCoordinator{fn: func(_ context.Context, _ *chat.Chat, _ []TeamMember) (Selection, error) {
		if step > 0 {
			return Selection{Done: true}, nil
		}
		step++

		return Selection{Members: []int{0, 1}}, nil
	}}

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{
		member(a, "worker"),
		member(b, "worker"),
	}, Options{Coordinator: coord})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.ErrorIs(t, err, agentErr)
}

func TestConcurrentDeterministicOrder(t *testing.T) {
	// Agent A sleeps longer so B finishes first, but replies should be in selection order.
	a := &delayMockAgent{
		name: "A", chat: chat.New(),
		reply: message.NewText("", role.Assistant, "A done"),
		delay: 20 * time.Millisecond,
	}
	b := &delayMockAgent{
		name: "B", chat: chat.New(),
		reply: message.NewText("", role.Assistant, "B done"),
		delay: 0,
	}

	step := 0
	coord := &funcCoordinator{fn: func(_ context.Context, _ *chat.Chat, _ []TeamMember) (Selection, error) {
		if step > 0 {
			return Selection{Done: true}, nil
		}
		step++

		return Selection{Members: []int{0, 1}}, nil
	}}

	shared := chat.New()
	r, err := New("reactor", shared, []TeamMember{
		member(a, "worker"),
		member(b, "worker"),
	}, Options{Coordinator: coord})
	require.NoError(t, err)

	_, err = r.Run(context.Background())
	require.NoError(t, err)

	// Despite B finishing first, A's reply should appear first (selection order).
	require.Equal(t, 2, shared.Len())
	assert.Equal(t, "A done", shared.At(0).TextContent())
	assert.Equal(t, "B done", shared.At(1).TextContent())
}

// --- test helpers ---

// funcCoordinator wraps a function as a Coordinator for ad-hoc test routing.
type funcCoordinator struct {
	fn func(context.Context, *chat.Chat, []TeamMember) (Selection, error)
}

func (f *funcCoordinator) Next(ctx context.Context, shared *chat.Chat, members []TeamMember) (Selection, error) {
	return f.fn(ctx, shared, members)
}

// delayMockAgent is a mock agent that introduces a delay before returning.
type delayMockAgent struct {
	name  string
	chat  *chat.Chat
	reply message.Message
	delay time.Duration
}

func (d *delayMockAgent) Run(_ context.Context) (message.Message, error) {
	time.Sleep(d.delay)

	reply := d.reply
	reply.Sender = d.name
	d.chat.Append(reply)

	return reply, nil
}

func (d *delayMockAgent) AgentName() string     { return d.name }
func (d *delayMockAgent) AgentChat() *chat.Chat { return d.chat }
