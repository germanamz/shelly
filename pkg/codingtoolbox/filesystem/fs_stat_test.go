package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStat_File(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "info.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_stat",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out statOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Equal(t, "info.txt", out.Name)
	assert.Equal(t, int64(5), out.Size)
	assert.False(t, out.IsDir)
	assert.NotEmpty(t, out.Mode)
	assert.NotEmpty(t, out.ModTime)
}

func TestStat_Directory(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	subDir := filepath.Join(dir, "mydir")
	require.NoError(t, os.Mkdir(subDir, 0o750))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_stat",
		Arguments: mustJSON(t, pathInput{Path: subDir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out statOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Equal(t, "mydir", out.Name)
	assert.True(t, out.IsDir)
}

func TestStat_NotFound(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_stat",
		Arguments: mustJSON(t, pathInput{Path: filepath.Join(dir, "nope")}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "no such file")
}

func TestStat_Denied(t *testing.T) {
	fs, dir := newTestFS(t, autoDeny)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "x.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_stat",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestStat_EmptyPath(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_stat",
		Arguments: `{"path":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "path is required")
}
