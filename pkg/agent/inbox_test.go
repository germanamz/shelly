package agent

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitInbox(t *testing.T) {
	a := New("test", "test agent", "do stuff", &sequenceCompleter{
		replies: []message.Message{message.NewText("test", role.Assistant, "done")},
	}, Options{})

	assert.Nil(t, a.Inbox())
	a.InitInbox()
	assert.NotNil(t, a.Inbox())
	assert.Equal(t, 1, cap(a.Inbox()))

	// Idempotent — second call is a no-op.
	inbox := a.Inbox()
	a.InitInbox()
	assert.Equal(t, inbox, a.Inbox())
}

func TestInbox_MessageDeliveredAfterToolResults(t *testing.T) {
	// Completer that:
	// 1. Returns a tool call
	// 2. Returns final text (should see the injected user message)
	var snapshots [][]message.Message
	completer := &snapshotCompleter{
		snapshots: &snapshots,
		replies: []message.Message{
			// First reply: call "echo" tool
			{
				Sender: "test",
				Role:   role.Assistant,
				Parts: []content.Part{
					content.ToolCall{ID: "call-1", Name: "echo", Arguments: `{"text":"hi"}`},
				},
			},
			// Second reply: final answer
			message.NewText("test", role.Assistant, "got it"),
		},
	}

	a := New("test", "test agent", "do stuff", completer, Options{})
	a.AddToolBoxes(newEchoToolBox())
	a.InitInbox()

	// Queue a user message in the inbox before running.
	a.inbox <- message.NewText("user", role.User, "injected message")

	reply, err := a.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "got it", reply.TextContent())

	// The second snapshot (before second Complete) should contain the injected
	// user message after the tool result.
	require.Len(t, snapshots, 2)
	secondSnapshot := snapshots[1]

	// Find the injected message — it should appear after the tool result.
	var found bool
	for _, msg := range secondSnapshot {
		if msg.Role == role.User && msg.TextContent() == "injected message" {
			found = true
			break
		}
	}
	assert.True(t, found, "injected user message should appear in chat before second completion")
}

func TestInbox_EmptyInboxDoesNotBlock(t *testing.T) {
	completer := &sequenceCompleter{
		replies: []message.Message{
			{
				Sender: "test",
				Role:   role.Assistant,
				Parts: []content.Part{
					content.ToolCall{ID: "call-1", Name: "echo", Arguments: `{}`},
				},
			},
			message.NewText("test", role.Assistant, "done"),
		},
	}

	a := New("test", "test agent", "do stuff", completer, Options{})
	a.AddToolBoxes(newEchoToolBox())
	a.InitInbox()

	// Run without putting anything in inbox — should not block.
	reply, err := a.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "done", reply.TextContent())
}

func TestInbox_NilInboxDoesNotPanic(t *testing.T) {
	completer := &sequenceCompleter{
		replies: []message.Message{
			{
				Sender: "test",
				Role:   role.Assistant,
				Parts: []content.Part{
					content.ToolCall{ID: "call-1", Name: "echo", Arguments: `{}`},
				},
			},
			message.NewText("test", role.Assistant, "done"),
		},
	}

	a := New("test", "test agent", "do stuff", completer, Options{})
	a.AddToolBoxes(newEchoToolBox())
	// Do NOT call InitInbox — inbox is nil.

	reply, err := a.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "done", reply.TextContent())
}
