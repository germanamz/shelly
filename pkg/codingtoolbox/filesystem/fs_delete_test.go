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

func TestDelete_File(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "doomed.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("bye"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_delete",
		Arguments: mustJSON(t, deleteInput{Path: filePath}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "ok", tr.Content)

	_, err := os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_Directory_Recursive(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(filepath.Join(subDir, "nested"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "nested", "f.txt"), []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_delete",
		Arguments: mustJSON(t, deleteInput{Path: subDir, Recursive: true}),
	})

	assert.False(t, tr.IsError, tr.Content)

	_, err := os.Stat(subDir)
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_NonRecursive_Directory_Fails(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	subDir := filepath.Join(dir, "notempty")
	require.NoError(t, os.MkdirAll(subDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "f.txt"), []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_delete",
		Arguments: mustJSON(t, deleteInput{Path: subDir}),
	})

	assert.True(t, tr.IsError)
}

func TestDelete_Denied(t *testing.T) {
	fs, dir := newTestFS(t, autoDeny)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "protected.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("safe"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_delete",
		Arguments: mustJSON(t, deleteInput{Path: filePath}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestDelete_EmptyPath(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_delete",
		Arguments: `{"path":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "path is required")
}
