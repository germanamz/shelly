package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionHandlerToolSuccess(t *testing.T) {
	var ch completionHandler
	tool := ch.tool()

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"completed","summary":"Implemented feature X","files_modified":["foo.go","bar.go"],"tests_run":["TestFoo"],"caveats":"needs docs"}`,
	))

	require.NoError(t, err)
	assert.Equal(t, "Task marked as completed.", result)

	cr := ch.Result()
	require.NotNil(t, cr)
	assert.Equal(t, "completed", cr.Status)
	assert.Equal(t, "Implemented feature X", cr.Summary)
	assert.Equal(t, []string{"foo.go", "bar.go"}, cr.FilesModified)
	assert.Equal(t, []string{"TestFoo"}, cr.TestsRun)
	assert.Equal(t, "needs docs", cr.Caveats)
	assert.True(t, ch.IsComplete())
}

func TestCompletionHandlerToolFailed(t *testing.T) {
	var ch completionHandler
	tool := ch.tool()

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"failed","summary":"Could not find the module"}`,
	))

	require.NoError(t, err)
	assert.Equal(t, "Task marked as failed.", result)

	cr := ch.Result()
	require.NotNil(t, cr)
	assert.Equal(t, "failed", cr.Status)
	assert.Equal(t, "Could not find the module", cr.Summary)
}

func TestCompletionHandlerToolInvalidStatus(t *testing.T) {
	var ch completionHandler
	tool := ch.tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"unknown","summary":"whatever"}`,
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status must be")
	assert.False(t, ch.IsComplete())
}

func TestCompletionHandlerToolInvalidInput(t *testing.T) {
	var ch completionHandler
	tool := ch.tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestCompletionHandlerToolDuplicateCallIgnored(t *testing.T) {
	var ch completionHandler
	tool := ch.tool()

	result1, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"completed","summary":"first call"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Task marked as completed.", result1)

	// Second call should be ignored and return "already marked".
	result2, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"failed","summary":"second call"}`,
	))
	require.NoError(t, err)
	assert.Contains(t, result2, "already marked")

	// Original completion result should be preserved.
	cr := ch.Result()
	require.NotNil(t, cr)
	assert.Equal(t, "completed", cr.Status)
	assert.Equal(t, "first call", cr.Summary)
}

func TestCompletionHandlerInitialState(t *testing.T) {
	var ch completionHandler
	assert.Nil(t, ch.Result())
	assert.False(t, ch.IsComplete())
}
