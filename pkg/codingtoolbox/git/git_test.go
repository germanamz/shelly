package git

import (
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
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

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return string(data)
}

// initRepo creates a temporary git repo with an initial commit.
func initRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}

	for _, args := range cmds {
		cmd := osexec.Command(args[0], args[1:]...) //nolint:gosec // test setup
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	// Create initial commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o600))

	cmd := osexec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = osexec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	return dir
}

func newTestGit(t *testing.T, askFn AskFunc, workDir string) (*Git, *permissions.Store) {
	t.Helper()

	permDir := t.TempDir()
	store, err := permissions.New(filepath.Join(permDir, "perms.json"))
	require.NoError(t, err)

	return New(store, askFn, workDir), store
}

func TestStatus(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_status",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "nothing to commit")
}

func TestStatus_Short(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	// Create an untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_status",
		Arguments: mustJSON(t, statusInput{Short: true}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "new.txt")
}

func TestDiff(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	// Modify a tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# changed"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_diff",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "changed")
}

func TestLog(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_log",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "initial")
}

func TestLog_CustomFormat(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_log",
		Arguments: mustJSON(t, logInput{Format: "short"}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "initial")
}

func TestLog_UnsupportedFormat(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_log",
		Arguments: mustJSON(t, logInput{Format: "format:%H%n%ae"}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "unsupported format")
}

func TestLog_AllAllowedFormats(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	for _, format := range []string{"oneline", "short", "medium", "full", "fuller", "reference", "email", "raw"} {
		tr := tb.Call(context.Background(), content.ToolCall{
			ID:        "tc-" + format,
			Name:      "git_log",
			Arguments: mustJSON(t, logInput{Format: format}),
		})

		assert.False(t, tr.IsError, "format %s should be allowed: %s", format, tr.Content)
	}
}

func TestCommit(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	// Create and stage a new file.
	newFile := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(newFile, []byte("new content"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "git_commit",
		Arguments: mustJSON(t, commitInput{
			Message: "add new file",
			Files:   []string{"new.txt"},
		}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "add new file")
}

func TestCommit_All(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	// Modify a tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# updated"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "git_commit",
		Arguments: mustJSON(t, commitInput{
			Message: "update readme",
			All:     true,
		}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "update readme")
}

func TestCommit_FilesAndAll(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "git_commit",
		Arguments: mustJSON(t, commitInput{
			Message: "bad",
			Files:   []string{"a.txt"},
			All:     true,
		}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "cannot use both files and all")
}

func TestCommit_EmptyMessage(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoApprove, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_commit",
		Arguments: `{"message":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "message is required")
}

func TestDenied(t *testing.T) {
	dir := initRepo(t)
	g, _ := newTestGit(t, autoDeny, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_status",
		Arguments: `{}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "permission denied")
}

func TestLimitedBuffer(t *testing.T) {
	var buf limitedBuffer

	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", buf.String())
	assert.Equal(t, 5, buf.Len())

	big := make([]byte, maxBufferSize+100)
	for i := range big {
		big[i] = 'x'
	}

	n, err = buf.Write(big)
	require.NoError(t, err)
	assert.Equal(t, len(big), n)
	assert.Equal(t, maxBufferSize, buf.Len())
}

func TestTrust(t *testing.T) {
	dir := initRepo(t)
	g, store := newTestGit(t, autoTrust, dir)
	tb := g.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "git_status",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.True(t, store.IsCommandTrusted("git"))

	// Subsequent calls bypass the ask — switch to deny to prove it.
	g.ask = autoDeny

	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "git_status",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
}
