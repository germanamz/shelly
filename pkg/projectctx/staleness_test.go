package projectctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/shellydir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsKnowledgeStale_MissingContextFile(t *testing.T) {
	tmp := t.TempDir()
	d := shellydir.New(filepath.Join(tmp, ".shelly"))

	assert.True(t, IsKnowledgeStale(tmp, d))
}

func TestIsKnowledgeStale_NonGitDir(t *testing.T) {
	tmp := t.TempDir()
	shellyRoot := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.MkdirAll(shellyRoot, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(shellyRoot, "context.md"), []byte("# Context"), 0o600))

	d := shellydir.New(shellyRoot)
	// Not a git repo — should fail open (not stale).
	assert.False(t, IsKnowledgeStale(tmp, d))
}

func TestIsKnowledgeStale_NewerCommit(t *testing.T) {
	tmp := t.TempDir()
	shellyRoot := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.MkdirAll(shellyRoot, 0o750))

	// Create context.md with an old mtime.
	contextPath := filepath.Join(shellyRoot, "context.md")
	require.NoError(t, os.WriteFile(contextPath, []byte("# Context"), 0o600))
	oldTime := time.Now().Add(-24 * time.Hour)
	require.NoError(t, os.Chtimes(contextPath, oldTime, oldTime))

	// Initialize a git repo and make a commit.
	initGitRepo(t, tmp)

	d := shellydir.New(shellyRoot)
	assert.True(t, IsKnowledgeStale(tmp, d), "should be stale when commit is newer than context.md")
}

func TestIsKnowledgeStale_FreshContext(t *testing.T) {
	tmp := t.TempDir()
	shellyRoot := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.MkdirAll(shellyRoot, 0o750))

	// Initialize a git repo and make a commit.
	initGitRepo(t, tmp)

	// Create context.md AFTER the commit (newer mtime).
	contextPath := filepath.Join(shellyRoot, "context.md")
	require.NoError(t, os.WriteFile(contextPath, []byte("# Context"), 0o600))
	// Ensure mtime is clearly in the future relative to the commit.
	futureTime := time.Now().Add(1 * time.Second)
	require.NoError(t, os.Chtimes(contextPath, futureTime, futureTime))

	d := shellydir.New(shellyRoot)
	assert.False(t, IsKnowledgeStale(tmp, d), "should not be stale when context.md is newer than commit")
}

// initGitRepo creates a minimal git repo with one commit in the given directory.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // test code
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, string(out))
	}
}
