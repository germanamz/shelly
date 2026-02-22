// Package skill provides loading of markdown-based skill files that teach
// agents step-by-step procedures. A Skill is a named block of markdown
// content derived from a .md file.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a procedure loaded from a markdown file.
type Skill struct {
	Name        string // Derived from filename without extension (e.g., "code-review").
	Description string // From YAML frontmatter; empty if no frontmatter.
	Content     string // Body after frontmatter (or full content if none).
}

// HasDescription reports whether the skill has a description from frontmatter.
func (s Skill) HasDescription() bool { return s.Description != "" }

// Load reads a single markdown file and returns a Skill. The skill name is
// derived from the filename with the extension stripped. If the file contains
// YAML frontmatter (delimited by ---), the frontmatter is parsed for name and
// description fields.
func Load(path string) (Skill, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-provided, not user input
	if err != nil {
		return Skill{}, fmt.Errorf("skill: load %q: %w", path, err)
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return Skill{}, fmt.Errorf("skill: load %q: %w", path, err)
	}

	if fm.Name != "" {
		name = fm.Name
	}

	return Skill{
		Name:        name,
		Description: fm.Description,
		Content:     body,
	}, nil
}

// frontmatter holds optional YAML metadata from the top of a skill file.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseFrontmatter extracts YAML frontmatter from raw content. If the content
// does not start with "---\n" or no closing "---" is found, the entire content
// is returned as the body with an empty frontmatter.
func parseFrontmatter(raw string) (frontmatter, string, error) {
	// Normalise \r\n → \n so the delimiter check works on Windows.
	raw = strings.ReplaceAll(raw, "\r\n", "\n")

	if !strings.HasPrefix(raw, "---\n") {
		return frontmatter{}, raw, nil
	}

	// Find closing delimiter after the opening "---\n".
	rest := raw[4:]

	var fmBlock, body string

	if before, after, found := strings.Cut(rest, "\n---\n"); found {
		fmBlock = before
		body = after
	} else if strings.HasSuffix(rest, "\n---") {
		fmBlock = rest[:len(rest)-4]
		body = ""
	} else {
		return frontmatter{}, raw, nil
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(fmBlock), &fm); err != nil {
		return frontmatter{}, "", fmt.Errorf("invalid frontmatter: %w", err)
	}

	return fm, body, nil
}

// LoadDir reads all .md files from the given directory (non-recursive) and
// returns them as skills sorted by filename. It returns an error if the
// directory cannot be read.
func LoadDir(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skill: load dir %q: %w", dir, err)
	}

	var skills []Skill

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		s, err := Load(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}

		skills = append(skills, s)
	}

	return skills, nil
}
