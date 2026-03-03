// Package projectctx loads curated context files and checks knowledge graph
// staleness. The combined context is injected into agent system prompts so
// agents understand the project they are working in.
package projectctx

import (
	"os"
	"strings"

	"github.com/germanamz/shelly/pkg/shellydir"
)

// MaxContextRunes is the default maximum number of runes for the combined
// context string. Approximately 8000 tokens.
const MaxContextRunes = 32000

// Context holds the assembled project context.
type Context struct {
	External string // Content from external AI tool context files (CLAUDE.md, .cursorrules, etc.).
	Curated  string // Content from curated *.md files.
	MaxRunes int    // Override for MaxContextRunes. If > 0, used instead of the default.
}

// String returns the combined context for injection into a system prompt.
// External context appears first, followed by curated — so project-specific
// Shelly context takes precedence by appearing later.
// If the combined length exceeds the rune limit, the result is truncated
// with a marker appended.
func (c Context) String() string {
	var b strings.Builder

	if c.External != "" {
		b.WriteString(c.External)
	}

	if c.Curated != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}

		b.WriteString(c.Curated)
	}

	result := b.String()

	limit := c.MaxRunes
	if limit <= 0 {
		limit = MaxContextRunes
	}

	runes := []rune(result)
	if len(runes) > limit {
		return string(runes[:limit]) + "\n\n[truncated — context exceeds limit]"
	}

	return result
}

// Load assembles project context from external tool files and curated files.
// All sources are best-effort: missing files are silently skipped.
func Load(d shellydir.Dir, projectRoot string) Context {
	return Context{
		External: LoadExternal(projectRoot),
		Curated:  LoadCurated(d),
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
