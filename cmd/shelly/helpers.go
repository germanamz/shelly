package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// loadDotEnv loads environment variables from path. Missing files are ignored.
func loadDotEnv(path string) error {
	err := godotenv.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// resolveConfigPath returns the config file to use. Priority:
// 1. Explicit --config flag (non-empty)
// 2. .shelly/config.yaml (if it exists)
// 3. shelly.yaml (legacy fallback)
func resolveConfigPath(explicit, shellyDirPath string) string {
	if explicit != "" {
		return explicit
	}

	shellyConfig := filepath.Join(shellyDirPath, "config.yaml")
	if _, err := os.Stat(shellyConfig); err == nil {
		return shellyConfig
	}

	return "shelly.yaml"
}
