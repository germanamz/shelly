package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandoffHandlerToolSuccess(t *testing.T) {
	var hh handoffHandler
	tool := hh.tool()

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"target_agent":"coder","reason":"needs coding expertise","context":"file X needs refactoring"}`,
	))

	require.NoError(t, err)
	assert.Equal(t, "Handing off to coder.", result)

	hr := hh.Result()
	require.NotNil(t, hr)
	assert.Equal(t, "coder", hr.TargetAgent)
	assert.Equal(t, "needs coding expertise", hr.Reason)
	assert.Equal(t, "file X needs refactoring", hr.Context)
	assert.True(t, hh.IsHandoff())
}

func TestHandoffHandlerToolDuplicateIgnored(t *testing.T) {
	var hh handoffHandler
	tool := hh.tool()

	result1, err := tool.Handler(context.Background(), json.RawMessage(
		`{"target_agent":"coder","reason":"first","context":"ctx1"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Handing off to coder.", result1)

	result2, err := tool.Handler(context.Background(), json.RawMessage(
		`{"target_agent":"tester","reason":"second","context":"ctx2"}`,
	))
	require.NoError(t, err)
	assert.Contains(t, result2, "already initiated")

	// Original handoff preserved.
	hr := hh.Result()
	require.NotNil(t, hr)
	assert.Equal(t, "coder", hr.TargetAgent)
}

func TestHandoffHandlerToolInvalidInput(t *testing.T) {
	var hh handoffHandler
	tool := hh.tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestHandoffHandlerToolEmptyTargetAgent(t *testing.T) {
	var hh handoffHandler
	tool := hh.tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"target_agent":"","reason":"reason","context":"ctx"}`,
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_agent is required")
	assert.False(t, hh.IsHandoff())
}

func TestHandoffHandlerToolEmptyReason(t *testing.T) {
	var hh handoffHandler
	tool := hh.tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"target_agent":"coder","reason":"","context":"ctx"}`,
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reason is required")
}

func TestHandoffHandlerToolEmptyContext(t *testing.T) {
	var hh handoffHandler
	tool := hh.tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"target_agent":"coder","reason":"reason","context":""}`,
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context is required")
}

func TestHandoffHandlerInitialState(t *testing.T) {
	var hh handoffHandler
	assert.Nil(t, hh.Result())
	assert.False(t, hh.IsHandoff())
}
