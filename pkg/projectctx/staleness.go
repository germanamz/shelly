package projectctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/germanamz/shelly/pkg/shellydir"
)

// IsKnowledgeStale checks if the knowledge graph entry point is outdated
// relative to the latest git commit. Returns true if context.md is missing
// or older than the most recent commit in the repository.
// If git is not available or the project is not a git repository, returns
// false (fail open — no staleness detection without git).
func IsKnowledgeStale(projectRoot string, d shellydir.Dir) bool {
	contextPath := filepath.Join(d.Root(), "context.md")

	info, err := os.Stat(contextPath)
	if err != nil {
		return true // missing = stale
	}

	contextMtime := info.ModTime()

	// Get latest commit timestamp.
	cmd := exec.Command("git", "-C", projectRoot, "log", "-1", "--format=%ct") //nolint:gosec // projectRoot is caller-provided
	out, err := cmd.Output()
	if err != nil {
		return false // git not available or not a repo — fail open
	}

	timestamp := strings.TrimSpace(string(out))
	if timestamp == "" {
		return false // no commits
	}

	epoch, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	commitTime := time.Unix(epoch, 0)

	return commitTime.After(contextMtime)
}
