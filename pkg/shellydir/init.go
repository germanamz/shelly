package shellydir

import (
	"fmt"
	"os"
)

const gitignoreContent = "local/\n"

// EnsureStructure creates the local/ directory and .gitignore file if they are
// missing. It is safe to call multiple times (idempotent). It does NOT create
// the .shelly/ root itself — the caller decides whether to bootstrap from
// scratch or only set up an existing directory.
func EnsureStructure(d Dir) error {
	if err := os.MkdirAll(d.LocalDir(), 0o750); err != nil {
		return fmt.Errorf("shellydir: create local dir: %w", err)
	}

	if err := ensureGitignore(d); err != nil {
		return fmt.Errorf("shellydir: gitignore: %w", err)
	}

	return nil
}

// ensureGitignore creates the .gitignore file if it does not exist.
func ensureGitignore(d Dir) error {
	path := d.GitignorePath()

	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	return os.WriteFile(path, []byte(gitignoreContent), 0o600)
}
