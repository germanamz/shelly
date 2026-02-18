package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/message"
	"github.com/germanamz/shelly/pkg/chatty/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ Provider = (*mockProvider)(nil)

type mockProvider struct {
	msg message.Message
	err error
}

func (m *mockProvider) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return m.msg, m.err
}

func TestProvider_Complete_Success(t *testing.T) {
	reply := message.NewText("bot", role.Assistant, "hello back")
	p := &mockProvider{msg: reply}

	c := chat.New(message.NewText("alice", role.User, "hello"))
	got, err := p.Complete(context.Background(), c)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, got.Role)
	assert.Equal(t, "hello back", got.TextContent())
}

func TestProvider_Complete_Error(t *testing.T) {
	p := &mockProvider{err: errors.New("api error")}

	c := chat.New(message.NewText("alice", role.User, "hello"))
	_, err := p.Complete(context.Background(), c)

	assert.EqualError(t, err, "api error")
}
