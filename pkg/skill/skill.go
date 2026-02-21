// Package skill provides loading of markdown-based skill files that teach
// agents step-by-step procedures. A Skill is a named block of markdown
// content derived from a .md file.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a procedure loaded from a markdown file.
type Skill struct {
	Name    string // Derived from filename without extension (e.g., "code-review").
	Content string // Raw markdown content.
}

// Load reads a single markdown file and returns a Skill. The skill name is
// derived from the filename with the extension stripped.
func Load(path string) (Skill, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-provided, not user input
	if err != nil {
		return Skill{}, fmt.Errorf("skill: load %q: %w", path, err)
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	return Skill{
		Name:    name,
		Content: string(data),
	}, nil
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
