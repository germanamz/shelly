package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
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

// noopNotify discards notifications.
func noopNotify(_ context.Context, _ string) {}

func newTestFS(t *testing.T, askFn AskFunc) (*FS, string) {
	t.Helper()

	return newTestFSWithNotify(t, askFn, noopNotify)
}

func newTestFSWithNotify(t *testing.T, askFn AskFunc, notifyFn NotifyFunc) (*FS, string) {
	t.Helper()

	dir := t.TempDir()
	permFile := filepath.Join(dir, ".shelly", "permissions.json")
	store, err := permissions.New(permFile)
	require.NoError(t, err)

	fs := New(store, askFn, notifyFn)

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
	fs1 := New(store1, autoApprove, noopNotify)

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
	fs2 := New(store2, autoDeny, noopNotify)

	tb2 := fs2.Tools()
	tr = tb2.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "fs_read",
		Arguments: mustJSON(t, pathInput{Path: filePath}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "data", tr.Content)
}

func TestReadLines(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	fileContent := "line1\nline2\nline3\nline4\nline5\n"
	filePath := filepath.Join(dir, "multi.txt")
	require.NoError(t, os.WriteFile(filePath, []byte(fileContent), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read_lines",
		Arguments: mustJSON(t, readLinesInput{Path: filePath, Offset: 2, Limit: 3}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "[Lines 2-4 of 5]")
	assert.Contains(t, tr.Content, "2→line2")
	assert.Contains(t, tr.Content, "4→line4")
	assert.NotContains(t, tr.Content, "line1")
	assert.NotContains(t, tr.Content, "line5")
}

func TestReadLines_DefaultLimit(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "small.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("a\nb\nc\n"), 0o600))

	// No offset or limit: should default to offset=1, limit=100.
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read_lines",
		Arguments: mustJSON(t, readLinesInput{Path: filePath}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "[Lines 1-3 of 3]")
	assert.Contains(t, tr.Content, "1→a")
	assert.Contains(t, tr.Content, "3→c")
}

func TestReadLines_OffsetBeyondFile(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "small.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("one\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_read_lines",
		Arguments: mustJSON(t, readLinesInput{Path: filePath, Offset: 100}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "empty range")
}

func TestReadLines_EmptyPath(t *testing.T) {
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

func TestEdit_Delete(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("line1\nline2\nline3\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: mustJSON(t, editInput{Path: filePath, OldText: "line2\n", NewText: ""}),
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "line1\nline3\n", string(data))
}

func TestEdit_DeleteOmitNewText(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("aaa bbb ccc"), 0o600))

	// Omit new_text entirely — should default to "" and delete old_text.
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: `{"path":"` + filePath + `","old_text":" bbb"}`,
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "aaa ccc", string(data))
}

func TestEdit_Insert(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o600))

	// Insert a new line after the opening brace by including context.
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_edit",
		Arguments: mustJSON(t, editInput{
			Path:    filePath,
			OldText: "func main() {\n",
			NewText: "func main() {\n\tx := 42\n",
		}),
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "func main() {\n\tx := 42\n\tfmt.Println(\"hello\")\n}\n", string(data))
}

func TestWrite_PreservesPermissions(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "script.sh")
	require.NoError(t, os.WriteFile(filePath, []byte("#!/bin/sh\necho old"), 0o600))
	require.NoError(t, os.Chmod(filePath, 0o755)) //nolint:gosec // test needs 0o755 to verify permission preservation

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_write",
		Arguments: mustJSON(t, writeInput{Path: filePath, Content: "#!/bin/sh\necho new"}),
	})

	assert.False(t, tr.IsError, tr.Content)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestWrite_NewFileDefault0600(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "brand-new.txt")

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_write",
		Arguments: mustJSON(t, writeInput{Path: filePath, Content: "hello"}),
	})

	assert.False(t, tr.IsError, tr.Content)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestEdit_PreservesPermissions(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "script.sh")
	require.NoError(t, os.WriteFile(filePath, []byte("old content"), 0o600))
	require.NoError(t, os.Chmod(filePath, 0o755)) //nolint:gosec // test needs 0o755 to verify permission preservation

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: mustJSON(t, editInput{Path: filePath, OldText: "old", NewText: "new"}),
	})

	assert.False(t, tr.IsError, tr.Content)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
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

func TestWrite_ConfirmDenied(t *testing.T) {
	// First call is dir permission (approve), second is file change confirmation (deny).
	calls := 0
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		calls++
		if calls == 1 {
			return "yes", nil // directory permission
		}
		return "no", nil // file change denied
	}
	fs, dir := newTestFS(t, askFn)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "denied.txt")

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_write",
		Arguments: mustJSON(t, writeInput{Path: filePath, Content: "new content"}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "denied")

	// File should not have been created.
	_, err := os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestWrite_TrustedSession(t *testing.T) {
	var notified string
	notifyFn := func(_ context.Context, msg string) {
		notified = msg
	}
	fs, dir := newTestFSWithNotify(t, autoApprove, notifyFn)
	tb := fs.Tools()

	st := &SessionTrust{}
	st.Trust()
	ctx := WithSessionTrust(context.Background(), st)

	filePath := filepath.Join(dir, "trusted.txt")

	tr := tb.Call(ctx, content.ToolCall{
		ID:        "tc1",
		Name:      "fs_write",
		Arguments: mustJSON(t, writeInput{Path: filePath, Content: "trusted content"}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, notified, "trusted.txt")
}

func TestEdit_ConfirmDenied(t *testing.T) {
	calls := 0
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		calls++
		if calls == 1 {
			return "yes", nil // directory permission
		}
		return "no", nil // file change denied
	}
	fs, dir := newTestFS(t, askFn)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("original"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_edit",
		Arguments: mustJSON(t, editInput{Path: filePath, OldText: "original", NewText: "changed"}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "denied")

	// File should remain unchanged.
	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "original", string(data))
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return string(data)
}
