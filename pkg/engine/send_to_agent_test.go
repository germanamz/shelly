package engine

import (
	"testing"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendToAgent_NotFound(t *testing.T) {
	e := &Engine{
		agentInboxes: make(map[string]chan message.Message),
	}
	err := e.SendToAgent("nonexistent", message.NewText("user", role.User, "hello"))
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestSendToAgent_Success(t *testing.T) {
	inbox := make(chan message.Message, 1)
	e := &Engine{
		agentInboxes: make(map[string]chan message.Message),
	}
	e.RegisterAgentInbox("agent-1", inbox)

	msg := message.NewText("user", role.User, "hello")
	err := e.SendToAgent("agent-1", msg)
	require.NoError(t, err)

	received := <-inbox
	assert.Equal(t, "hello", received.TextContent())
}

func TestSendToAgent_InboxFull(t *testing.T) {
	inbox := make(chan message.Message, 1)
	e := &Engine{
		agentInboxes: make(map[string]chan message.Message),
	}
	e.RegisterAgentInbox("agent-1", inbox)

	// Fill the inbox.
	msg1 := message.NewText("user", role.User, "first")
	require.NoError(t, e.SendToAgent("agent-1", msg1))

	// Second send should fail.
	msg2 := message.NewText("user", role.User, "second")
	err := e.SendToAgent("agent-1", msg2)
	assert.ErrorIs(t, err, ErrAgentInboxFull)
}

func TestRegisterUnregisterAgentInbox(t *testing.T) {
	inbox := make(chan message.Message, 1)
	e := &Engine{
		agentInboxes: make(map[string]chan message.Message),
	}

	e.RegisterAgentInbox("agent-1", inbox)
	require.NoError(t, e.SendToAgent("agent-1", message.NewText("user", role.User, "hello")))

	e.UnregisterAgentInbox("agent-1")
	err := e.SendToAgent("agent-1", message.NewText("user", role.User, "hello"))
	assert.ErrorIs(t, err, ErrAgentNotFound)
}
