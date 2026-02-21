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

func TestDiff(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("line1\nline2\nline3\n"), 0o600))
	require.NoError(t, os.WriteFile(fileB, []byte("line1\nchanged\nline3\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_diff",
		Arguments: mustJSON(t, diffInput{FileA: fileA, FileB: fileB}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "-line2")
	assert.Contains(t, tr.Content, "+changed")
}

func TestDiff_Identical(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("same"), 0o600))
	require.NoError(t, os.WriteFile(fileB, []byte("same"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_diff",
		Arguments: mustJSON(t, diffInput{FileA: fileA, FileB: fileB}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "files are identical", tr.Content)
}

func TestDiff_FileNotFound(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	fileA := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_diff",
		Arguments: mustJSON(t, diffInput{FileA: fileA, FileB: filepath.Join(dir, "nope.txt")}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "no such file")
}

func TestDiff_Denied(t *testing.T) {
	fs, dir := newTestFS(t, autoDeny)
	tb := fs.Tools()

	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(fileB, []byte("y"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_diff",
		Arguments: mustJSON(t, diffInput{FileA: fileA, FileB: fileB}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestDiff_EmptyFileA(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_diff",
		Arguments: `{"file_a":"","file_b":"/tmp/x"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "file_a is required")
}

func TestDiff_EmptyFileB(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_diff",
		Arguments: `{"file_a":"/tmp/x","file_b":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "file_b is required")
}
