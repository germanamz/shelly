package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

func stubRunner(msg message.Message, err error) Runner {
	return RunnerFunc(func(_ context.Context) (message.Message, error) {
		return msg, err
	})
}

func panicRunner() Runner {
	return RunnerFunc(func(_ context.Context) (message.Message, error) {
		panic("something went wrong")
	})
}

func slowRunner(delay time.Duration) Runner {
	return RunnerFunc(func(ctx context.Context) (message.Message, error) {
		select {
		case <-time.After(delay):
			return message.NewText("bot", role.Assistant, "done"), nil
		case <-ctx.Done():
			return message.Message{}, ctx.Err()
		}
	})
}

// --- Timeout tests ---

func TestTimeout(t *testing.T) {
	inner := stubRunner(message.NewText("bot", role.Assistant, "done"), nil)

	wrapped := Timeout(time.Second)(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "done", msg.TextContent())
}

func TestTimeoutExpires(t *testing.T) {
	wrapped := Timeout(50 * time.Millisecond)(slowRunner(200 * time.Millisecond))
	_, err := wrapped.Run(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// --- Recovery tests ---

func TestRecovery(t *testing.T) {
	inner := stubRunner(message.NewText("bot", role.Assistant, "ok"), nil)

	wrapped := Recovery()(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "ok", msg.TextContent())
}

func TestRecoveryCatchesPanic(t *testing.T) {
	wrapped := Recovery()(panicRunner())
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

	inner := stubRunner(message.NewText("bot", role.Assistant, "reply"), nil)

	wrapped := Logger(log, "test-agent")(inner)
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

	inner := stubRunner(message.Message{}, errors.New("boom"))

	wrapped := Logger(log, "err-agent")(inner)
	_, err := wrapped.Run(context.Background())

	require.Error(t, err)
	output := buf.String()
	assert.Contains(t, output, "agent finished with error")
	assert.Contains(t, output, "boom")
}

// --- OutputGuardrail tests ---

func TestOutputGuardrailPasses(t *testing.T) {
	inner := stubRunner(message.NewText("bot", role.Assistant, "safe content"), nil)
	check := func(_ message.Message) error { return nil }

	wrapped := OutputGuardrail(check)(inner)
	msg, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "safe content", msg.TextContent())
}

func TestOutputGuardrailRejects(t *testing.T) {
	inner := stubRunner(message.NewText("bot", role.Assistant, "bad content"), nil)
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
	inner := stubRunner(message.Message{}, errors.New("agent failed"))
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

// --- Middleware composition test ---

func TestMiddlewareComposition(t *testing.T) {
	var order []string

	mw := func(name string) Middleware {
		return func(next Runner) Runner {
			return RunnerFunc(func(ctx context.Context) (message.Message, error) {
				order = append(order, name+":before")
				msg, err := next.Run(ctx)
				order = append(order, name+":after")
				return msg, err
			})
		}
	}

	inner := stubRunner(message.NewText("bot", role.Assistant, "done"), nil)

	// Apply A(B(C(inner)))
	wrapped := mw("A")(mw("B")(mw("C")(inner)))
	_, err := wrapped.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{
		"A:before", "B:before", "C:before",
		"C:after", "B:after", "A:after",
	}, order)
}
