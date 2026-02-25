package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type copyInput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func (f *FS) copyTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_copy",
		Description: "Copy a file or directory. Directories are copied recursively.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"source":{"type":"string","description":"Source path"},"destination":{"type":"string","description":"Destination path"}},"required":["source","destination"]}`),
		Handler:     f.handleCopy,
	}
}

func (f *FS) handleCopy(ctx context.Context, input json.RawMessage) (string, error) {
	var in copyInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_copy: invalid input: %w", err)
	}

	if in.Source == "" {
		return "", fmt.Errorf("fs_copy: source is required")
	}

	if in.Destination == "" {
		return "", fmt.Errorf("fs_copy: destination is required")
	}

	if err := f.checkPermission(ctx, in.Source); err != nil {
		return "", err
	}

	if err := f.checkPermission(ctx, in.Destination); err != nil {
		return "", err
	}

	absSrc, err := filepath.Abs(in.Source)
	if err != nil {
		return "", fmt.Errorf("fs_copy: %w", err)
	}

	absDst, err := filepath.Abs(in.Destination)
	if err != nil {
		return "", fmt.Errorf("fs_copy: %w", err)
	}

	f.locker.LockPair(absSrc, absDst)
	defer f.locker.UnlockPair(absSrc, absDst)

	diff := fmt.Sprintf("Copy: %s -> %s", absSrc, absDst)
	if err := f.confirmChange(ctx, absDst, diff); err != nil {
		return "", fmt.Errorf("fs_copy: %w", err)
	}

	info, err := os.Stat(absSrc)
	if err != nil {
		return "", fmt.Errorf("fs_copy: %w", err)
	}

	if info.IsDir() {
		if err := copyDir(absSrc, absDst); err != nil {
			return "", fmt.Errorf("fs_copy: %w", err)
		}
	} else {
		if err := copyFile(absSrc, absDst, info.Mode()); err != nil {
			return "", fmt.Errorf("fs_copy: %w", err)
		}
	}

	return "ok", nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	in, err := os.Open(src) //nolint:gosec // path is approved by user
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // best-effort close on read

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) //nolint:gosec // path is approved by user
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)

	closeErr := out.Close()

	if copyErr != nil {
		return copyErr
	}

	return closeErr
}

func copyDir(src, dst string) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolve source: %w", err)
	}

	// Resolve the source root through symlinks so comparisons work on systems
	// where temp directories are behind symlinks (e.g. /var -> /private/var on macOS).
	absSrc, err = filepath.EvalSymlinks(absSrc)
	if err != nil {
		return fmt.Errorf("resolve source symlinks: %w", err)
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Resolve symlinks and verify the real path stays within the source tree.
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}

		if resolved != absSrc && !strings.HasPrefix(resolved, absSrc+string(filepath.Separator)) {
			// Symlink points outside the source tree; skip it.
			if d.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		return copyFile(path, target, info.Mode())
	})
}
