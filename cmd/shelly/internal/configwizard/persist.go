package configwizard

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/shellydir"
	"gopkg.in/yaml.v3"
)

// SaveConfig marshals cfg to YAML, ensures the .shelly/ directory exists, and
// writes the config file. If path is empty, it defaults to <shellyDirPath>/config.yaml.
func SaveConfig(cfg engine.Config, path, shellyDirPath string) error {
	if shellyDirPath == "" {
		shellyDirPath = ".shelly"
	}

	dir := shellydir.New(shellyDirPath)

	// Bootstrap .shelly/ directory structure if needed.
	if err := os.MkdirAll(dir.Root(), 0o750); err != nil {
		return fmt.Errorf("configwizard: create shelly dir: %w", err)
	}
	if err := shellydir.EnsureStructure(dir); err != nil {
		return fmt.Errorf("configwizard: ensure structure: %w", err)
	}

	if path == "" {
		path = dir.ConfigPath()
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("configwizard: create parent dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("configwizard: marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // config file, not secret
		return fmt.Errorf("configwizard: write config: %w", err)
	}

	return nil
}
