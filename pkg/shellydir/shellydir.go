// Package shellydir encapsulates all path knowledge for the .shelly/ project
// directory. It provides a Dir value object with accessors for config, context,
// skills, permissions, and local runtime state paths.
package shellydir

import (
	"os"
	"path/filepath"
	"sort"
)

// Dir is a value object that resolves paths within a .shelly/ directory.
type Dir struct {
	root string
}

// New creates a Dir rooted at the given path. The path is converted to an
// absolute path. No I/O is performed; use EnsureStructure to create the
// directory layout.
func New(root string) Dir {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}

	return Dir{root: abs}
}

// Root returns the absolute path to the .shelly/ directory.
func (d Dir) Root() string { return d.root }

// ConfigPath returns the path to the main config file.
func (d Dir) ConfigPath() string { return filepath.Join(d.root, "config.yaml") }

// ContextPath returns the path to the curated context file.
func (d Dir) ContextPath() string { return filepath.Join(d.root, "context.md") }

// SkillsDir returns the path to the skills directory.
func (d Dir) SkillsDir() string { return filepath.Join(d.root, "skills") }

// LocalDir returns the path to the local (gitignored) runtime state directory.
func (d Dir) LocalDir() string { return filepath.Join(d.root, "local") }

// PermissionsPath returns the path to the permissions file inside local/.
func (d Dir) PermissionsPath() string { return filepath.Join(d.root, "local", "permissions.json") }

// ContextCachePath returns the path to the auto-generated context cache.
func (d Dir) ContextCachePath() string { return filepath.Join(d.root, "local", "context-cache.json") }

// NotesDir returns the path to the notes directory inside local/.
func (d Dir) NotesDir() string { return filepath.Join(d.root, "local", "notes") }

// ReflectionsDir returns the path to the reflections directory inside local/.
func (d Dir) ReflectionsDir() string { return filepath.Join(d.root, "local", "reflections") }

// GitignorePath returns the path to the .gitignore file inside .shelly/.
func (d Dir) GitignorePath() string { return filepath.Join(d.root, ".gitignore") }

// ContextFiles returns sorted paths of all *.md files in the .shelly/ root
// directory (non-recursive). Returns nil if the directory does not exist.
func (d Dir) ContextFiles() []string {
	pattern := filepath.Join(d.root, "*.md")

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}

	sort.Strings(matches)

	return matches
}

// Exists reports whether the .shelly/ root directory exists on disk.
func (d Dir) Exists() bool {
	info, err := os.Stat(d.root)

	return err == nil && info.IsDir()
}
