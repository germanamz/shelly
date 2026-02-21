package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/agents"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/reactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

// stubAgent returns a fixed message from Run.
type stubAgent struct {
	msg message.Message
	err error
}

func (s *stubAgent) Run(_ context.Context) (message.Message, error) {
	return s.msg, s.err
}

// namedStubAgent is a stubAgent that also implements reactor.NamedAgent.
type namedStubAgent struct {
	stubAgent
	name string
	c    *chat.Chat
}

func (n *namedStubAgent) AgentName() string     { return n.name }
func (n *namedStubAgent) AgentChat() *chat.Chat { return n.c }

// panicAgent panics during Run.
type panicAgent struct{}

func (p *panicAgent) Run(_ context.Context) (message.Message, error) {
	panic("something went wrong")
}

// orderTracker records middleware execution order.
type orderTracker struct {
	order []string
}

func (o *orderTracker) middleware(name string) Middleware {
	return func(next agents.Agent) agents.Agent {
		return &orderAgent{
			namedAgentBase: namedAgentBase{next: next},
			tracker:        o,
			name:           name,
		}
	}
}

type orderAgent struct {
	namedAgentBase
	tracker *orderTracker
	name    string
}

func (o *orderAgent) Run(ctx context.Context) (message.Message, error) {
	o.tracker.order = append(o.tracker.order, o.name+":before")
	msg, err := o.next.Run(ctx)
	o.tracker.order = append(o.tracker.order, o.name+":after")
	return msg, err
}

// Compile-time checks.
var (
	_ agents.Agent       = (*stubAgent)(nil)
	_ reactor.NamedAgent = (*namedStubAgent)(nil)
)

// --- Timeout tests ---

func TestTimeout(t *testing.T) {
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "done"),
	}

	wrapped := Timeout(time.Second)(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "done", msg.TextContent())
}

func TestTimeoutExpires(t *testing.T) {
	inner := &stubAgent{}
	inner.err = nil

	// Create a slow agent that respects context cancellation
	slow := &slowAgent{delay: 200 * time.Millisecond}
	wrapped := Timeout(50 * time.Millisecond)(slow)
	_, err := wrapped.Run(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

type slowAgent struct {
	delay time.Duration
}

func (s *slowAgent) Run(ctx context.Context) (message.Message, error) {
	select {
	case <-time.After(s.delay):
		return message.NewText("bot", role.Assistant, "done"), nil
	case <-ctx.Done():
		return message.Message{}, ctx.Err()
	}
}

// --- Recovery tests ---

func TestRecovery(t *testing.T) {
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "ok"),
	}

	wrapped := Recovery()(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "ok", msg.TextContent())
}

func TestRecoveryCatchesPanic(t *testing.T) {
	wrapped := Recovery()(&panicAgent{})
	msg, err := wrapped.Run(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent panicked")
	assert.Contains(t, err.Error(), "something went wrong")
	assert.Equal(t, message.Message{}, msg)
}

// --- Logger tests ---

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	inner := &namedStubAgent{
		stubAgent: stubAgent{
			msg: message.NewText("bot", role.Assistant, "reply"),
		},
		name: "test-agent",
		c:    chat.New(),
	}

	wrapped := Logger(log)(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "reply", msg.TextContent())

	output := buf.String()
	assert.Contains(t, output, "agent started")
	assert.Contains(t, output, "agent finished")
	assert.Contains(t, output, "test-agent")
}

func TestLoggerWithError(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	inner := &stubAgent{err: errors.New("boom")}

	wrapped := Logger(log)(inner)
	_, err := wrapped.Run(context.Background())

	require.Error(t, err)
	output := buf.String()
	assert.Contains(t, output, "agent finished with error")
	assert.Contains(t, output, "boom")
}

// --- OutputGuardrail tests ---

func TestOutputGuardrailPasses(t *testing.T) {
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "safe content"),
	}
	check := func(_ message.Message) error {
		return nil
	}

	wrapped := OutputGuardrail(check)(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "safe content", msg.TextContent())
}

func TestOutputGuardrailRejects(t *testing.T) {
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "bad content"),
	}
	check := func(m message.Message) error {
		if m.TextContent() == "bad content" {
			return errors.New("guardrail: content rejected")
		}
		return nil
	}

	wrapped := OutputGuardrail(check)(inner)
	msg, err := wrapped.Run(context.Background())

	require.Error(t, err)
	assert.Equal(t, "guardrail: content rejected", err.Error())
	assert.Equal(t, message.Message{}, msg)
}

func TestOutputGuardrailSkipsOnError(t *testing.T) {
	inner := &stubAgent{err: errors.New("agent failed")}
	called := false
	check := func(_ message.Message) error {
		called = true
		return nil
	}

	wrapped := OutputGuardrail(check)(inner)
	_, err := wrapped.Run(context.Background())

	require.Error(t, err)
	assert.Equal(t, "agent failed", err.Error())
	assert.False(t, called)
}

// --- Chain / Apply tests ---

func TestChainOrder(t *testing.T) {
	tracker := &orderTracker{}
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "done"),
	}

	chained := Chain(
		tracker.middleware("A"),
		tracker.middleware("B"),
		tracker.middleware("C"),
	)

	wrapped := chained(inner)
	_, err := wrapped.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{
		"A:before", "B:before", "C:before",
		"C:after", "B:after", "A:after",
	}, tracker.order)
}

func TestApply(t *testing.T) {
	tracker := &orderTracker{}
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "done"),
	}

	wrapped := Apply(inner,
		tracker.middleware("first"),
		tracker.middleware("second"),
	)

	_, err := wrapped.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{
		"first:before", "second:before",
		"second:after", "first:after",
	}, tracker.order)
}

func TestChainEmpty(t *testing.T) {
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "unchanged"),
	}

	wrapped := Chain()(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "unchanged", msg.TextContent())
}

// --- NamedAgent preservation tests ---

func TestNamedAgentPreservedThroughMiddleware(t *testing.T) {
	c := chat.New()
	inner := &namedStubAgent{
		stubAgent: stubAgent{
			msg: message.NewText("bot", role.Assistant, "reply"),
		},
		name: "my-agent",
		c:    c,
	}

	wrapped := Apply(inner,
		Timeout(time.Second),
		Recovery(),
		OutputGuardrail(func(_ message.Message) error { return nil }),
	)

	na, ok := wrapped.(reactor.NamedAgent)
	require.True(t, ok, "wrapped agent should implement reactor.NamedAgent")
	assert.Equal(t, "my-agent", na.AgentName())
	assert.Same(t, c, na.AgentChat())
}

func TestNamedAgentNotImplementedOnPlainAgent(t *testing.T) {
	inner := &stubAgent{
		msg: message.NewText("bot", role.Assistant, "reply"),
	}

	wrapped := Timeout(time.Second)(inner)

	na, ok := wrapped.(reactor.NamedAgent)
	require.True(t, ok, "wrapper always implements NamedAgent interface")
	assert.Empty(t, na.AgentName())
	assert.Nil(t, na.AgentChat())
}

// --- Composition integration test ---

func TestComposedMiddleware(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	c := chat.New()
	inner := &namedStubAgent{
		stubAgent: stubAgent{
			msg: message.NewText("bot", role.Assistant, "safe"),
		},
		name: "composed-agent",
		c:    c,
	}

	wrapped := Apply(inner,
		Recovery(),
		Logger(log),
		Timeout(time.Second),
		OutputGuardrail(func(_ message.Message) error { return nil }),
	)

	msg, err := wrapped.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "safe", msg.TextContent())

	na, ok := wrapped.(reactor.NamedAgent)
	require.True(t, ok)
	assert.Equal(t, "composed-agent", na.AgentName())
	assert.Same(t, c, na.AgentChat())

	output := buf.String()
	assert.Contains(t, output, "composed-agent")
}
