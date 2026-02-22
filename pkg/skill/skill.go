// Package skill provides loading of folder-based skill definitions that teach
// agents step-by-step procedures. Each skill lives in its own directory with a
// mandatory SKILL.md entry point and optional supplementary files.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a procedure loaded from a skill folder.
type Skill struct {
	Name        string // Derived from folder name (e.g., "code-review").
	Description string // From YAML frontmatter; empty if no frontmatter.
	Content     string // Body after frontmatter (or full content if none).
	Dir         string // Absolute path to the skill folder.
}

// HasDescription reports whether the skill has a description from frontmatter.
func (s Skill) HasDescription() bool { return s.Description != "" }

// Load reads a skill from a folder. The folder must contain a SKILL.md file.
// The skill name is derived from the folder name. If SKILL.md contains YAML
// frontmatter (delimited by ---), the frontmatter is parsed for name and
// description fields.
func Load(path string) (Skill, error) {
	entryPoint := filepath.Join(path, "SKILL.md")

	data, err := os.ReadFile(entryPoint) //nolint:gosec // path is caller-provided, not user input
	if err != nil {
		return Skill{}, fmt.Errorf("skill: load %q: %w", path, err)
	}

	name := filepath.Base(path)

	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return Skill{}, fmt.Errorf("skill: load %q: %w", path, err)
	}

	if fm.Name != "" {
		name = fm.Name
	}

	absDir, err := filepath.Abs(path)
	if err != nil {
		return Skill{}, fmt.Errorf("skill: load %q: %w", path, err)
	}

	return Skill{
		Name:        name,
		Description: fm.Description,
		Content:     body,
		Dir:         absDir,
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

// LoadDir reads all subdirectories from the given directory and returns skills
// for each that contains a SKILL.md file. Subdirectories without SKILL.md are
// silently skipped. It returns an error if the directory cannot be read.
func LoadDir(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skill: load dir %q: %w", dir, err)
	}

	var skills []Skill

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, e.Name())

		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			continue
		}

		s, err := Load(skillDir)
		if err != nil {
			return nil, err
		}

		skills = append(skills, s)
	}

	return skills, nil
}
