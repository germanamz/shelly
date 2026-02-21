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

func TestCopy_File(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	src := filepath.Join(dir, "orig.txt")
	dst := filepath.Join(dir, "copy.txt")
	require.NoError(t, os.WriteFile(src, []byte("content"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_copy",
		Arguments: mustJSON(t, copyInput{Source: src, Destination: dst}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "ok", tr.Content)

	// Source still exists.
	_, err := os.Stat(src)
	require.NoError(t, err)

	// Destination has same content.
	data, err := os.ReadFile(dst) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))
}

func TestCopy_Directory(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	srcDir := filepath.Join(dir, "srcdir")
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("b"), 0o600))

	dstDir := filepath.Join(dir, "dstdir")

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_copy",
		Arguments: mustJSON(t, copyInput{Source: srcDir, Destination: dstDir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filepath.Join(dstDir, "a.txt")) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "a", string(data))

	data, err = os.ReadFile(filepath.Join(dstDir, "sub", "b.txt")) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "b", string(data))
}

func TestCopy_Denied(t *testing.T) {
	fs, dir := newTestFS(t, autoDeny)
	tb := fs.Tools()

	src := filepath.Join(dir, "x.txt")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_copy",
		Arguments: mustJSON(t, copyInput{Source: src, Destination: filepath.Join(dir, "y.txt")}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestCopy_SourceNotFound(t *testing.T) {
	fs, dir := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_copy",
		Arguments: mustJSON(t, copyInput{Source: filepath.Join(dir, "nope"), Destination: filepath.Join(dir, "y")}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "no such file")
}

func TestCopy_EmptySource(t *testing.T) {
	fs, _ := newTestFS(t, autoApprove)
	tb := fs.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "fs_copy",
		Arguments: `{"source":"","destination":"/tmp/x"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "source is required")
}
