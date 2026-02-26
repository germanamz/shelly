package projectctx

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadExternal reads context files from external AI coding tools (Claude Code,
// Cursor) and returns their concatenated content. Missing files are silently
// skipped.
func LoadExternal(projectRoot string) string {
	var parts []string

	// Claude Code: CLAUDE.md at project root.
	if s := readFileContent(filepath.Join(projectRoot, "CLAUDE.md")); s != "" {
		parts = append(parts, s)
	}

	// Cursor legacy: .cursorrules at project root.
	if s := readFileContent(filepath.Join(projectRoot, ".cursorrules")); s != "" {
		parts = append(parts, s)
	}

	// Cursor modern: .cursor/rules/*.mdc sorted alphabetically.
	for _, f := range globSorted(filepath.Join(projectRoot, ".cursor", "rules", "*.mdc")) {
		s := readFileContent(f)
		if s == "" {
			continue
		}
		parts = append(parts, stripFrontmatter(s))
	}

	return strings.Join(parts, "\n\n")
}

// readFileContent reads a file and returns its trimmed content.
// Returns empty string on any error.
func readFileContent(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // paths are constructed from project root
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// globSorted returns matched paths sorted alphabetically.
func globSorted(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	sort.Strings(matches)

	return matches
}

// stripFrontmatter removes YAML frontmatter delimited by --- lines from the
// beginning of a string. If no frontmatter is found, the original string is
// returned unchanged. Windows-style \r\n line endings are normalised to \n
// before processing.
func stripFrontmatter(raw string) string {
	// Normalise Windows line endings so the delimiter matching works
	// regardless of line-ending style.
	raw = strings.ReplaceAll(raw, "\r\n", "\n")

	if !strings.HasPrefix(raw, "---") {
		return raw
	}

	// Find the closing --- after the opening one.
	rest := raw[3:]
	_, after, found := strings.Cut(rest, "\n---")
	if !found {
		return raw
	}

	return strings.TrimSpace(after)
}
