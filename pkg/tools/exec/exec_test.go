package exec

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/tools/permissions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autoApprove(_ context.Context, _ string, _ []string) (string, error) {
	return "yes", nil
}

func autoTrust(_ context.Context, _ string, _ []string) (string, error) {
	return "trust", nil
}

func autoDeny(_ context.Context, _ string, _ []string) (string, error) {
	return "no", nil
}

func newTestExec(t *testing.T, askFn AskFunc) (*Exec, *permissions.Store) {
	t.Helper()

	dir := t.TempDir()
	store, err := permissions.New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	return New(store, askFn), store
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return string(data)
}

func TestRun_Echo(t *testing.T) {
	e, _ := newTestExec(t, autoApprove)
	tb := e.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "exec_run",
		Arguments: mustJSON(t, runInput{Command: "echo", Args: []string{"hello"}}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "hello")
}

func TestRun_Denied(t *testing.T) {
	e, _ := newTestExec(t, autoDeny)
	tb := e.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "exec_run",
		Arguments: mustJSON(t, runInput{Command: "echo", Args: []string{"hello"}}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "permission denied")
}

func TestRun_Trust(t *testing.T) {
	e, store := newTestExec(t, autoTrust)
	tb := e.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "exec_run",
		Arguments: mustJSON(t, runInput{Command: "echo", Args: []string{"first"}}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.True(t, store.IsCommandTrusted("echo"))

	// Subsequent calls bypass the ask — switch to deny to prove it.
	e.ask = autoDeny

	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "exec_run",
		Arguments: mustJSON(t, runInput{Command: "echo", Args: []string{"second"}}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "second")
}

func TestRun_CommandFailed(t *testing.T) {
	e, _ := newTestExec(t, autoApprove)
	tb := e.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "exec_run",
		Arguments: mustJSON(t, runInput{Command: "false"}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "exec_run:")
}

func TestRun_EmptyCommand(t *testing.T) {
	e, _ := newTestExec(t, autoApprove)
	tb := e.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "exec_run",
		Arguments: `{"command":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "command is required")
}

func TestRun_CommandNotFound(t *testing.T) {
	e, _ := newTestExec(t, autoApprove)
	tb := e.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "exec_run",
		Arguments: mustJSON(t, runInput{Command: "nonexistent_command_xyz_12345"}),
	})

	assert.True(t, tr.IsError)
}
