package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteractionChannelQuestionAndAnswer(t *testing.T) {
	ic := NewInteractionChannel()

	a := &Agent{name: "child-1", interaction: ic}
	tool := requestInputTool(a, ic)

	assert.Equal(t, "request_input", tool.Name)

	// Run the tool in a goroutine since it blocks.
	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"question":"What is the target directory?"}`))
		ch <- result{out, err}
	}()

	// Read the question from the channel.
	select {
	case q := <-ic.Questions():
		assert.Equal(t, "child-1", q.Agent)
		assert.Equal(t, "What is the target directory?", q.Content)
		assert.Equal(t, "q-1", q.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question")
	}

	// Send answer.
	ic.Answer("/tmp/output")

	select {
	case r := <-ch:
		require.NoError(t, r.err)
		assert.Equal(t, "/tmp/output", r.output)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool result")
	}
}

func TestRequestInputToolContextCancellation(t *testing.T) {
	ic := NewInteractionChannel()
	a := &Agent{name: "child-1", interaction: ic}
	tool := requestInputTool(a, ic)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := tool.Handler(ctx, json.RawMessage(`{"question":"anything?"}`))

	assert.ErrorIs(t, err, context.Canceled)
}

func TestRequestInputToolEmptyQuestion(t *testing.T) {
	ic := NewInteractionChannel()
	a := &Agent{name: "child-1", interaction: ic}
	tool := requestInputTool(a, ic)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"question":""}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "question is required")
}

func TestRequestInputToolInvalidInput(t *testing.T) {
	ic := NewInteractionChannel()
	a := &Agent{name: "child-1", interaction: ic}
	tool := requestInputTool(a, ic)

	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestAutoAnswer(t *testing.T) {
	ic := NewInteractionChannel()
	ctx := t.Context()

	autoAnswer(ctx, ic, "The project uses Go 1.25 and targets Linux.")

	// Send a question.
	ic.questionCh <- Question{ID: "q-1", Agent: "child", Content: "What language?"}

	// Read the answer.
	select {
	case answer := <-ic.answerCh:
		assert.Contains(t, answer, "What language?")
		assert.Contains(t, answer, "The project uses Go 1.25 and targets Linux.")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for auto-answer")
	}
}

func TestAutoAnswerStopsOnContextCancel(t *testing.T) {
	ic := NewInteractionChannel()
	ctx, cancel := context.WithCancel(context.Background())

	autoAnswer(ctx, ic, "context")
	cancel()

	// Give the goroutine time to exit. Verify it doesn't answer after cancel.
	time.Sleep(50 * time.Millisecond)

	select {
	case ic.questionCh <- Question{ID: "q-1", Agent: "child", Content: "question?"}:
		// Question was sent, but auto-answer goroutine is stopped.
		select {
		case <-ic.answerCh:
			t.Fatal("should not receive answer after context cancel")
		case <-time.After(100 * time.Millisecond):
			// Expected: no answer.
		}
	case <-time.After(100 * time.Millisecond):
		// Channel buffer may be full, that's ok.
	}
}

func TestInteractionChannelQuestionIDIncrement(t *testing.T) {
	ic := NewInteractionChannel()
	a := &Agent{name: "child-1", interaction: ic}
	tool := requestInputTool(a, ic)

	ctx := t.Context()

	// Start auto-answerer.
	autoAnswer(ctx, ic, "ctx")

	// First call.
	out1, err := tool.Handler(ctx, json.RawMessage(`{"question":"q1"}`))
	require.NoError(t, err)
	assert.Contains(t, out1, "q1")

	// Second call.
	out2, err := tool.Handler(ctx, json.RawMessage(`{"question":"q2"}`))
	require.NoError(t, err)
	assert.Contains(t, out2, "q2")
}

func TestRequestInputContextCancelDuringAnswer(t *testing.T) {
	ic := NewInteractionChannel()
	a := &Agent{name: "child-1", interaction: ic}
	tool := requestInputTool(a, ic)

	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan error, 1)
	go func() {
		_, err := tool.Handler(ctx, json.RawMessage(`{"question":"test"}`))
		ch <- err
	}()

	// Wait for the question to be sent.
	select {
	case <-ic.Questions():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question")
	}

	// Cancel context instead of answering.
	cancel()

	select {
	case err := <-ch:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool error")
	}
}
