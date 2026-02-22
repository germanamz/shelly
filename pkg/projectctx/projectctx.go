// Package projectctx loads curated context files and generates/caches a
// structural project index. The combined context is injected into agent
// system prompts so agents understand the project they are working in.
package projectctx

import (
	"fmt"
	"os"
	"strings"

	"github.com/germanamz/shelly/pkg/shellydir"
)

// Context holds the assembled project context.
type Context struct {
	Curated   string // Content from curated *.md files.
	Generated string // Auto-generated structural index.
}

// String returns the combined context for injection into a system prompt.
func (c Context) String() string {
	var b strings.Builder

	if c.Curated != "" {
		b.WriteString(c.Curated)
	}

	if c.Generated != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}

		b.WriteString(c.Generated)
	}

	return b.String()
}

// Load assembles project context from curated files and the generated cache.
// Both sources are best-effort: missing files are silently skipped.
func Load(d shellydir.Dir) Context {
	return Context{
		Curated:   LoadCurated(d),
		Generated: loadGenerated(d),
	}
}

// LoadCurated reads all *.md files from the .shelly/ root and concatenates
// their contents. Returns empty string if no files exist.
func LoadCurated(d shellydir.Dir) string {
	files := d.ContextFiles()
	if len(files) == 0 {
		return ""
	}

	var parts []string

	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // path comes from Dir.ContextFiles glob
		if err != nil {
			continue
		}

		content := strings.TrimSpace(string(data))
		if content != "" {
			parts = append(parts, content)
		}
	}

	return strings.Join(parts, "\n\n")
}

// loadGenerated reads the cached structural index from local/context-cache.json.
func loadGenerated(d shellydir.Dir) string {
	data, err := os.ReadFile(d.ContextCachePath())
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// Generate creates a structural project index and writes it to the cache file.
// It inspects the project root for entry points, package structure, and
// Go module info.
func Generate(projectRoot string, d shellydir.Dir) (string, error) {
	index := generateIndex(projectRoot)
	if index == "" {
		return "", nil
	}

	// Write cache.
	if err := os.MkdirAll(d.LocalDir(), 0o750); err != nil {
		return "", fmt.Errorf("projectctx: create cache dir: %w", err)
	}

	if err := os.WriteFile(d.ContextCachePath(), []byte(index), 0o600); err != nil {
		return "", fmt.Errorf("projectctx: write cache: %w", err)
	}

	return index, nil
}

// IsStale checks whether the cached index is older than the project's go.mod.
// Returns true if cache is missing or stale.
func IsStale(projectRoot string, d shellydir.Dir) bool {
	cacheInfo, err := os.Stat(d.ContextCachePath())
	if err != nil {
		return true
	}

	modInfo, err := os.Stat(fmt.Sprintf("%s/go.mod", projectRoot))
	if err != nil {
		return false // No go.mod, can't determine staleness.
	}

	return modInfo.ModTime().After(cacheInfo.ModTime())
}
