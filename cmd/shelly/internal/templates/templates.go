// Package templates provides embedded project templates for `shelly init`.
package templates

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/shellydir"
	"gopkg.in/yaml.v3"
)

//go:embed settings/*.yaml skills/*.md
var templateFS embed.FS

// TemplateMeta holds display metadata for a template.
type TemplateMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// TemplateSkill holds an embedded skill to be written to .shelly/skills/.
// Content is resolved from the embedded skills/ directory at load time.
type TemplateSkill struct {
	Name    string `yaml:"name"`
	Content string `yaml:"-"`
}

// Template wraps an engine.Config with metadata and optional embedded skills.
type Template struct {
	Meta           TemplateMeta    `yaml:"template"`
	Config         engine.Config   `yaml:"config"`
	EmbeddedSkills []TemplateSkill `yaml:"embedded_skills"`
}

// List returns metadata for all available templates.
func List() []TemplateMeta {
	entries, err := templateFS.ReadDir("settings")
	if err != nil {
		return nil
	}

	var metas []TemplateMeta
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		t, err := load("settings/" + e.Name())
		if err != nil {
			continue
		}
		metas = append(metas, t.Meta)
	}

	return metas
}

// Get loads a template by name. Returns an error if the template is not found.
func Get(name string) (Template, error) {
	entries, err := templateFS.ReadDir("settings")
	if err != nil {
		return Template{}, fmt.Errorf("templates: read dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		t, err := load("settings/" + e.Name())
		if err != nil {
			continue
		}
		if t.Meta.Name == name {
			return t, nil
		}
	}

	return Template{}, fmt.Errorf("templates: %q not found", name)
}

// Apply writes the template's config and embedded skills to the given shellyDir.
// If force is true, existing config and skill files are overwritten.
func Apply(t Template, shellyDirPath string, force bool) error {
	dir := shellydir.New(shellyDirPath)

	// Create root + skills dir.
	if err := os.MkdirAll(dir.Root(), 0o750); err != nil {
		return fmt.Errorf("templates: create root: %w", err)
	}
	if err := os.MkdirAll(dir.SkillsDir(), 0o750); err != nil {
		return fmt.Errorf("templates: create skills dir: %w", err)
	}

	// Ensure local/ and .gitignore.
	if err := shellydir.EnsureStructure(dir); err != nil {
		return fmt.Errorf("templates: ensure structure: %w", err)
	}

	// Marshal and write config.
	data, err := yaml.Marshal(t.Config)
	if err != nil {
		return fmt.Errorf("templates: marshal config: %w", err)
	}

	configPath := dir.ConfigPath()
	if !force {
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("templates: config already exists at %s (use --force to overwrite)", configPath)
		}
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil { //nolint:gosec // config file, not secret
		return fmt.Errorf("templates: write config: %w", err)
	}

	// Write embedded skills.
	for _, sk := range t.EmbeddedSkills {
		skillDir := filepath.Join(dir.SkillsDir(), sk.Name)
		if err := os.MkdirAll(skillDir, 0o750); err != nil {
			return fmt.Errorf("templates: create skill dir %q: %w", sk.Name, err)
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(sk.Content), 0o644); err != nil { //nolint:gosec // skill content, not secret
			return fmt.Errorf("templates: write skill %q: %w", sk.Name, err)
		}
	}

	return nil
}

// load parses a template from the embedded filesystem and resolves skill
// content from the embedded skills/ directory.
func load(filename string) (Template, error) {
	data, err := templateFS.ReadFile(filename)
	if err != nil {
		return Template{}, err
	}

	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return Template{}, err
	}

	// Resolve skill content from embedded files.
	for i := range t.EmbeddedSkills {
		sk := &t.EmbeddedSkills[i]
		content, err := templateFS.ReadFile("skills/" + sk.Name + ".md")
		if err != nil {
			return Template{}, fmt.Errorf("templates: skill %q: %w", sk.Name, err)
		}
		sk.Content = string(content)
	}

	return t, nil
}
