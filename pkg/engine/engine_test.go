package engine

import (
	"context"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCompleter is a simple Completer that returns a canned reply.
type mockCompleter struct {
	reply string
}

func (m *mockCompleter) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return message.NewText("bot", role.Assistant, m.reply), nil
}

func TestEngine_NewSession_DefaultAgent(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "hello"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock", Model: "test"}},
		Agents:    []AgentConfig{{Name: "bot", Description: "test bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)
	assert.NotEmpty(t, sess.ID())

	found, ok := eng.Session(sess.ID())
	assert.True(t, ok)
	assert.Equal(t, sess, found)
}

func TestEngine_NewSession_NamedAgent(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "hi"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents: []AgentConfig{
			{Name: "alpha", Provider: "p1"},
			{Name: "beta", Provider: "p1"},
		},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	_, err = eng.NewSession("beta")
	assert.NoError(t, err)
}

func TestEngine_NewSession_NotFound(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "x"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	_, err = eng.NewSession("nope")
	assert.Error(t, err)
}

func TestEngine_SendAndEvents(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "world"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sub := eng.Events().Subscribe(32)
	defer eng.Events().Unsubscribe(sub)

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	reply, err := sess.Send(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "world", reply.TextContent())

	// Drain events — expect at least AgentStart and AgentEnd.
	var kinds []EventKind
	timeout := time.After(time.Second)
	for {
		select {
		case e := <-sub.C:
			kinds = append(kinds, e.Kind)
		case <-timeout:
			goto done
		}
	}
done:
	assert.Contains(t, kinds, EventAgentStart)
	assert.Contains(t, kinds, EventAgentEnd)
}

func TestEngine_StateEnabled(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1", Toolboxes: []string{"state"}}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	assert.NotNil(t, eng.State())
}

func TestEngine_TasksEnabled(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1", Toolboxes: []string{"tasks"}}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	assert.NotNil(t, eng.Tasks())
}

func TestEngine_TasksDisabled(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	assert.Nil(t, eng.Tasks())
}

func TestEngine_InvalidConfig(t *testing.T) {
	_, err := New(context.Background(), Config{})
	assert.Error(t, err)
}

func TestSession_SendParts(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	reply, err := sess.SendParts(context.Background(), content.Text{Text: "hi"})
	require.NoError(t, err)
	assert.Equal(t, "ok", reply.TextContent())
}

func TestSession_ConcurrentSendBlocked(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	// Manually lock the session.
	sess.mu.Lock()
	sess.active = true
	sess.mu.Unlock()

	_, err = sess.Send(context.Background(), "test")
	require.ErrorContains(t, err, "already active")

	// Unlock for cleanup.
	sess.mu.Lock()
	sess.active = false
	sess.mu.Unlock()
}
