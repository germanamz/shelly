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

func TestPatch(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("alpha beta gamma"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path: filePath,
			Hunks: []hunk{
				{OldText: "alpha", NewText: "ALPHA"},
				{OldText: "gamma", NewText: "GAMMA"},
			},
		}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "ok", tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "ALPHA beta GAMMA", string(data))
}

func TestPatch_HunkNotFound(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path:  filePath,
			Hunks: []hunk{{OldText: "missing", NewText: "x"}},
		}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "not found")
}

func TestPatch_HunkAmbiguous(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("aaa aaa"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path:  filePath,
			Hunks: []hunk{{OldText: "aaa", NewText: "b"}},
		}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "2 times")
}

func TestPatch_DeleteHunk(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("line1\nline2\nline3\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path:  filePath,
			Hunks: []hunk{{OldText: "line2\n", NewText: ""}},
		}),
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "line1\nline3\n", string(data))
}

func TestPatch_DeleteOmitNewText(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("keep remove keep"), 0o600))

	// Omit new_text entirely — should default to "" and delete old_text.
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_patch",
		Arguments: `{"path":"` + filePath + `","hunks":[{"old_text":" remove"}]}`,
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "keep keep", string(data))
}

func TestPatch_InsertAndModify(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("import (\n\t\"fmt\"\n)\n\nfunc old() {}\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path: filePath,
			Hunks: []hunk{
				// Insert a new import.
				{OldText: "\t\"fmt\"\n", NewText: "\t\"fmt\"\n\t\"os\"\n"},
				// Rename function.
				{OldText: "func old()", NewText: "func new()"},
			},
		}),
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "import (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc new() {}\n", string(data))
}

func TestPatch_EmptyHunks(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_patch",
		Arguments: mustJSON(t, patchInput{Path: filePath, Hunks: []hunk{}}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "at least one hunk")
}

func TestPatch_Denied(t *testing.T) {
	fs, dir := newTestFS(t, autoDeny)
	tb := fs.Tools()

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path:  filePath,
			Hunks: []hunk{{OldText: "x", NewText: "y"}},
		}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestPatch_EmptyPath(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path:  "",
			Hunks: []hunk{{OldText: "x", NewText: "y"}},
		}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "path is required")
}

func TestPatch_ConfirmDenied(t *testing.T) {
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

	filePath := filepath.Join(dir, "patch.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("alpha beta"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "fs_patch",
		Arguments: mustJSON(t, patchInput{
			Path:  filePath,
			Hunks: []hunk{{OldText: "alpha", NewText: "ALPHA"}},
		}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "denied")

	// File should remain unchanged.
	data, err := os.ReadFile(filePath) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "alpha beta", string(data))
}
