package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/tools/permissions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// autoApprove always grants permission.
func autoApprove(_ context.Context, _ string, _ []string) (string, error) {
	return "yes", nil
}

// autoDeny always denies permission.
func autoDeny(_ context.Context, _ string, _ []string) (string, error) {
	return "no", nil
}

func newTestFS(t *testing.T, askFn AskFunc) (*FS, string) {
	t.Helper()

	dir := t.TempDir()
	permFile := filepath.Join(dir, ".shelly", "permissions.json")
	store, err := permissions.New(permFile)
	require.NoError(t, err)

	fs := New(store, askFn)

	return fs, dir
}

func TestRead(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello world"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "hello world", tr.Content)
}

func TestRead_FileNotFound(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: filepath.Join(dir, "nope.txt")}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "no such file")
}

func TestRead_Denied(t *testing.T) {
	fs, dir := newTestFS(t, autoDeny)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("secret"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestWrite(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "sub", "out.txt")

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_write",
		Arguments: mustJSON(t, writeInput{Path: filePath, Content: "written"}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "ok", tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "written", string(data))
}

func TestEdit(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("foo bar baz"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: mustJSON(t, editInput{Path: filePath, OldText: "bar", NewText: "qux"}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "ok", tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "foo qux baz", string(data))
}

func TestEdit_NotFound(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: mustJSON(t, editInput{Path: filePath, OldText: "missing", NewText: "x"}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "not found")
}

func TestEdit_Ambiguous(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("aaa aaa"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: mustJSON(t, editInput{Path: filePath, OldText: "aaa", NewText: "b"}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "2 times")
}

func TestList(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o750))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_list",
		Arguments: mustJSON(t, pathInput{Path: dir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &entries))
	assert.GreaterOrEqual(t, len(entries), 2)

	names := make(map[string]string)
	for _, e := range entries {
		names[e.Name] = e.Type
	}

	assert.Equal(t, "file", names["a.txt"])
	assert.Equal(t, "dir", names["sub"])
}

func TestSubdirectoryInheritance(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)

	// Approve parent directory by reading a file in it.
	filePath := filepath.Join(dir, "top.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("top"), 0o600))

	tb := fs.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})
	require.False(t, tr.IsError, tr.Content)

	// Now switch to a deny-ask to prove subdirectory access uses inherited permission.
	fs.ask = autoDeny

	subDir := filepath.Join(dir, "child")
	require.NoError(t, os.MkdirAll(subDir, 0o750))
	subFile := filepath.Join(subDir, "nested.txt")
	require.NoError(t, os.WriteFile(subFile, []byte("nested"), 0o600))

	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: subFile}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "nested", tr.Content)
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	permFile := filepath.Join(dir, ".shelly", "permissions.json")

	// First instance: approve a directory.
	store1, err := permissions.New(permFile)
	require.NoError(t, err)
	fs1 := New(store1, autoApprove)

	filePath := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("data"), 0o600))

	tb := fs1.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})
	require.False(t, tr.IsError, tr.Content)

	// Second instance: should already have the permission (deny would block if not).
	store2, err := permissions.New(permFile)
	require.NoError(t, err)
	fs2 := New(store2, autoDeny)

	tb2 := fs2.Tools()
	tr = tb2.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "data", tr.Content)
}

func TestRead_EmptyPath(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read",
		Arguments: `{"path":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "path is required")
}

func TestWrite_EmptyPath(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_write",
		Arguments: `{"path":"","content":"x"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "path is required")
}

func TestEdit_EmptyOldText(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "e.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("data"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: mustJSON(t, editInput{Path: filePath, OldText: "", NewText: "x"}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "old_text is required")
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return string(data)
}
