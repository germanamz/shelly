package shellydir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MigratePermissions moves the legacy permissions file from .shelly/permissions.json
// to .shelly/local/permissions.json. The operation is idempotent: it is a no-op
// if the old file does not exist or the new file already exists.
func MigratePermissions(d Dir) error {
	oldPath := filepath.Join(d.Root(), "permissions.json")
	newPath := d.PermissionsPath()

	// Nothing to migrate if old file doesn't exist.
	if _, err := os.Stat(oldPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("shellydir: migrate permissions: stat old path: %w", err)
	}

	// Don't overwrite if new location already has a file.
	if _, err := os.Stat(newPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("shellydir: migrate permissions: stat new path: %w", err)
	}

	// Ensure local/ directory exists.
	if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
		return fmt.Errorf("shellydir: migrate permissions: create dir: %w", err)
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("shellydir: migrate permissions: %w", err)
	}

	return nil
}
