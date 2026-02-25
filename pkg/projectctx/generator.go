package projectctx

import (
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
			fmt.Fprintf(&b, "- %s\n", p)
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

// findPackages lists pkg/ subdirectories that contain .go files.
func findPackages(root string) []string {
	pkgDir := filepath.Join(root, "pkg")

	info, err := os.Stat(pkgDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	var pkgs []string

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
			pkgs = append(pkgs, "pkg/"+rel)
		}

		return nil
	})
	if err != nil {
		return nil
	}

	sort.Strings(pkgs)

	return pkgs
}
