package shellydir

import (
	"fmt"
	"os"
	"path/filepath"
)

const gitignoreContent = "local/\n"

const skeletonConfig = `providers:
  - name: default
    kind: anthropic        # anthropic | openai | grok
    api_key: ${ANTHROPIC_API_KEY}
    model: claude-sonnet-4-20250514

agents:
  - name: assistant
    description: A helpful assistant
    instructions: You are a helpful assistant. Be concise and accurate.
    provider: default
    options:
      max_iterations: 10
      max_delegation_depth: 2

entry_agent: assistant
`

// Bootstrap creates the .shelly/ directory from scratch with a full initial
// structure: root, skills/, local/, .gitignore, and a skeleton config.yaml.
// Existing files are never overwritten, making it safe to run on an already
// initialised directory.
func Bootstrap(d Dir) error {
	return BootstrapWithConfig(d, []byte(skeletonConfig))
}

// BootstrapWithConfig creates the .shelly/ directory from scratch using the
// provided config content instead of the skeleton default. The directory
// structure (root, skills/, local/, .gitignore) is identical to Bootstrap.
// Existing files are never overwritten.
func BootstrapWithConfig(d Dir, config []byte) error {
	if err := os.MkdirAll(d.Root(), 0o750); err != nil {
		return fmt.Errorf("shellydir: create root: %w", err)
	}

	if err := os.MkdirAll(d.SkillsDir(), 0o750); err != nil {
		return fmt.Errorf("shellydir: create skills dir: %w", err)
	}

	if err := EnsureStructure(d); err != nil {
		return err
	}

	if err := ensureFile(d.ConfigPath(), config); err != nil {
		return fmt.Errorf("shellydir: config: %w", err)
	}

	return nil
}

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

// ensureFile creates a file with the given content only if it does not exist.
// It uses O_CREATE|O_EXCL to atomically check and create, avoiding a
// TOCTOU race between stat and write.
func ensureFile(path string, content []byte) error {
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	_, writeErr := f.Write(content)
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}

// ensureGitignore creates the .gitignore file if it does not exist.
func ensureGitignore(d Dir) error {
	return ensureFile(d.GitignorePath(), []byte(gitignoreContent))
}
