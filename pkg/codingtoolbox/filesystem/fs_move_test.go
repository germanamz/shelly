package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMove(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_move",
		Arguments: mustJSON(t, moveInput{Source: src, Destination: dst}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "ok", tr.Content)

	_, err := os.Stat(src)
	assert.True(t, os.IsNotExist(err))

	data, err := os.ReadFile(dst) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}

func TestMove_CreatesParentDirs(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "nested", "deep", "b.txt")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_move",
		Arguments: mustJSON(t, moveInput{Source: src, Destination: dst}),
	})

	assert.False(t, tr.IsError, tr.Content)
}

func TestMove_Denied(t *testing.T) {
	fs, dir := newTestFS(t, autoDeny)
	tb := fs.Tools()

	src := filepath.Join(dir, "x.txt")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_move",
		Arguments: mustJSON(t, moveInput{Source: src, Destination: filepath.Join(dir, "y.txt")}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestMove_EmptySource(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_move",
		Arguments: `{"source":"","destination":"/tmp/x"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "source is required")
}

func TestMove_EmptyDestination(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_move",
		Arguments: `{"source":"/tmp/x","destination":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "destination is required")
}

func TestMove_ConfirmDenied(t *testing.T) {
	calls := 0
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		calls++
		if calls == 1 {
			return "yes", nil // directory permission (same dir covers both)
		}
		return "no", nil // file change denied
	}
	fs, dir := newTestFS(t, askFn)
	tb := fs.Tools()

	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_move",
		Arguments: mustJSON(t, moveInput{Source: src, Destination: dst}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "denied")

	// Source should still exist.
	_, err := os.Stat(src)
	assert.False(t, os.IsNotExist(err))

	// Destination should not exist.
	_, err = os.Stat(dst)
	assert.True(t, os.IsNotExist(err))
}
