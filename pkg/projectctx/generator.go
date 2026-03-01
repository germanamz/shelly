package projectctx

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxTreeDepth = 4

// generateIndex produces a structural overview of the project.
func generateIndex(projectRoot string) string {
	var b strings.Builder

	// Module info.
	if mod := readModule(projectRoot); mod != "" {
		fmt.Fprintf(&b, "Go module: %s\n", mod)
	}

	// Entry points.
	entries := findEntryPoints(projectRoot)
	if len(entries) > 0 {
		b.WriteString("\nEntry points:\n")
		for _, e := range entries {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}

	// Package tree.
	pkgs := findPackages(projectRoot)
	if len(pkgs) > 0 {
		b.WriteString("\nPackages:\n")
		for _, p := range pkgs {
			if p.Desc != "" {
				fmt.Fprintf(&b, "- %s — %s\n", p.Path, p.Desc)
			} else {
				fmt.Fprintf(&b, "- %s\n", p.Path)
			}
		}
	}

	return strings.TrimSpace(b.String())
}

// readModule extracts the module path from go.mod.
func readModule(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod")) //nolint:gosec // root is caller-provided project path
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if mod, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(mod)
		}
	}

	return ""
}

// findEntryPoints looks for cmd/*/main.go files.
func findEntryPoints(root string) []string {
	pattern := filepath.Join(root, "cmd", "*", "main.go")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	var entries []string
	for _, m := range matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			continue
		}

		entries = append(entries, rel)
	}

	sort.Strings(entries)

	return entries
}

type packageInfo struct {
	Path string // e.g. "pkg/agent"
	Desc string // first-sentence description, may be empty
}

// findPackages lists pkg/ subdirectories that contain .go files.
func findPackages(root string) []packageInfo {
	pkgDir := filepath.Join(root, "pkg")

	info, err := os.Stat(pkgDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	var pkgs []packageInfo

	err = filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip inaccessible paths
		}

		if !info.IsDir() {
			return nil
		}

		// Depth limit.
		rel, relErr := filepath.Rel(pkgDir, path)
		if relErr != nil {
			return nil
		}

		if rel == "." {
			return nil
		}

		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > maxTreeDepth {
			return filepath.SkipDir
		}

		// Check for .go files.
		goFiles, _ := filepath.Glob(filepath.Join(path, "*.go"))
		if len(goFiles) > 0 {
			pkgs = append(pkgs, packageInfo{
				Path: "pkg/" + rel,
				Desc: readPackageDescription(path),
			})
		}

		return nil
	})
	if err != nil {
		return nil
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Path < pkgs[j].Path
	})

	return pkgs
}

// readPackageDescription extracts a short description from a package directory.
// It tries README.md first, then falls back to Go doc comments.
func readPackageDescription(dir string) string {
	if desc := extractReadmeDescription(filepath.Join(dir, "README.md")); desc != "" {
		return desc
	}

	return extractDocComment(dir)
}

// extractReadmeDescription reads a README.md and returns the first sentence
// of the first paragraph after the heading.
func extractReadmeDescription(path string) string {
	f, err := os.Open(path) //nolint:gosec // path is constructed from project root
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)

	// Skip heading line(s) and blank lines after them.
	pastHeading := false
	for scanner.Scan() {
		line := scanner.Text()
		if !pastHeading {
			if strings.HasPrefix(line, "#") {
				pastHeading = true

				continue
			}
			// No heading yet, skip blank lines before heading.
			if strings.TrimSpace(line) == "" {
				continue
			}
			// Non-heading, non-blank line before any heading — treat as paragraph.
			return firstSentence(line)
		}

		// Past heading — skip blank lines until first paragraph.
		if strings.TrimSpace(line) == "" {
			continue
		}

		return firstSentence(line)
	}

	return ""
}

// extractDocComment looks for a Go package doc comment in doc.go or the first
// alphabetical .go file, and returns the first sentence.
func extractDocComment(dir string) string {
	// Try doc.go first.
	if desc := extractGoDocFromFile(filepath.Join(dir, "doc.go")); desc != "" {
		return desc
	}

	// Fall back to first alphabetical .go file.
	goFiles, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	sort.Strings(goFiles)

	for _, f := range goFiles {
		if desc := extractGoDocFromFile(f); desc != "" {
			return desc
		}
	}

	return ""
}

// extractGoDocFromFile reads a .go file and extracts the first sentence from
// a "// Package <name> ..." comment block.
func extractGoDocFromFile(path string) string {
	f, err := os.Open(path) //nolint:gosec // path is constructed from project root
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)

	var commentLines []string
	inComment := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "// Package ") {
			inComment = true
			// Strip the "// " prefix.
			commentLines = append(commentLines, strings.TrimPrefix(line, "// "))

			continue
		}

		if inComment {
			if text, ok := strings.CutPrefix(line, "//"); ok {
				text = strings.TrimPrefix(text, " ")
				commentLines = append(commentLines, text)

				continue
			}
			// End of comment block.
			break
		}

		// Skip blank lines and other comments before the package doc.
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Hit non-comment code, stop.
		break
	}

	if len(commentLines) == 0 {
		return ""
	}

	full := strings.Join(commentLines, " ")
	// Strip "Package <name> " prefix to get just the description.
	if _, rest, ok := strings.Cut(full, " "); ok {
		if _, rest, ok = strings.Cut(rest, " "); ok {
			return firstSentence(rest)
		}
	}

	return ""
}

// firstSentence returns text up to and including the first period that is
// followed by a space, end of string, or newline.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)

	for i, ch := range s {
		if ch == '.' {
			// Period at end of string.
			if i+1 >= len(s) {
				return strings.TrimSpace(s[:i+1])
			}
			// Period followed by space or newline.
			next := s[i+1]
			if next == ' ' || next == '\n' || next == '\r' {
				return strings.TrimSpace(s[:i+1])
			}
		}
	}

	// No sentence-ending period found; return the whole text.
	return strings.TrimSpace(s)
}
