package ask

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponder_AskWithOptions(t *testing.T) {
	var received Question

	var r *Responder
	r = NewResponder(func(_ context.Context, q Question) {
		received = q
		go func() {
			_ = r.Respond(q.ID, "blue")
		}()
	})

	tb := r.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "ask_user",
		Arguments: `{"question":"Pick a color","options":["red","blue","green"]}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "blue", tr.Content)
	assert.Equal(t, "tc1", tr.ToolCallID)

	assert.Equal(t, "Pick a color", received.Text)
	assert.Equal(t, []string{"red", "blue", "green"}, received.Options)
	assert.NotEmpty(t, received.ID)
}

func TestResponder_AskFreeForm(t *testing.T) {
	var r *Responder
	r = NewResponder(func(_ context.Context, q Question) {
		go func() {
			_ = r.Respond(q.ID, "42")
		}()
	})

	tb := r.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "ask_user",
		Arguments: `{"question":"What is your age?"}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "42", tr.Content)
}

func TestResponder_ContextCancelled(t *testing.T) {
	r := NewResponder(nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan content.ToolResult, 1)
	go func() {
		tb := r.Tools()
		done <- tb.Call(ctx, content.ToolCall{
			ID:        "tc1",
			Name:      "ask_user",
			Arguments: `{"question":"hello?"}`,
		})
	}()

	cancel()

	tr := <-done
	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "context canceled")
}

func TestResponder_RespondUnknownID(t *testing.T) {
	r := NewResponder(nil)

	err := r.Respond("bogus", "whatever")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResponder_EmptyQuestion(t *testing.T) {
	r := NewResponder(nil)

	tb := r.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "ask_user",
		Arguments: `{"question":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "question is required")
}

func TestResponder_InvalidInput(t *testing.T) {
	r := NewResponder(nil)

	tb := r.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "ask_user",
		Arguments: `not json`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "invalid input")
}

func TestResponder_Ask(t *testing.T) {
	var r *Responder
	r = NewResponder(func(_ context.Context, q Question) {
		go func() {
			_ = r.Respond(q.ID, "yes")
		}()
	})

	resp, err := r.Ask(context.Background(), "Allow access?", []string{"yes", "no"})
	require.NoError(t, err)
	assert.Equal(t, "yes", resp)
}

func TestResponder_Ask_EmptyQuestion(t *testing.T) {
	r := NewResponder(nil)

	_, err := r.Ask(context.Background(), "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "question is required")
}

func TestResponder_Ask_ContextCancelled(t *testing.T) {
	r := NewResponder(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Ask(ctx, "hello?", nil)
	assert.Error(t, err)
}

func TestResponder_UniqueIDs(t *testing.T) {
	var ids []string

	var r *Responder
	r = NewResponder(func(_ context.Context, q Question) {
		ids = append(ids, q.ID)
		go func() {
			_ = r.Respond(q.ID, "ok")
		}()
	})

	tb := r.Tools()
	for range 3 {
		tr := tb.Call(context.Background(), content.ToolCall{
			ID:        "tc",
			Name:      "ask_user",
			Arguments: `{"question":"q?"}`,
		})
		assert.False(t, tr.IsError, tr.Content)
	}

	assert.Len(t, ids, 3)
	assert.NotEqual(t, ids[0], ids[1])
	assert.NotEqual(t, ids[1], ids[2])
}
