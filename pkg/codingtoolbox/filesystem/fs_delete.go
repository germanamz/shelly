package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type deleteInput struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

func (f *FS) deleteTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_delete",
		Description: "Delete a file or directory. Set recursive to true to delete a directory and all its contents.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to delete"},"recursive":{"type":"boolean","description":"If true, delete directory and all contents"}},"required":["path"]}`),
		Handler:     f.handleDelete,
	}
}

func (f *FS) handleDelete(ctx context.Context, input json.RawMessage) (string, error) {
	var in deleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_delete: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_delete: path is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_delete: %w", err)
	}

	f.locker.Lock(abs)
	defer f.locker.Unlock(abs)

	diff := fmt.Sprintf("Delete: %s", abs)
	if in.Recursive {
		diff = fmt.Sprintf("Delete: %s (recursive)", abs)
	}

	if err := f.confirmChange(ctx, abs, diff); err != nil {
		return "", fmt.Errorf("fs_delete: %w", err)
	}

	if in.Recursive {
		if err := os.RemoveAll(abs); err != nil {
			return "", fmt.Errorf("fs_delete: %w", err)
		}
	} else {
		if err := os.Remove(abs); err != nil {
			return "", fmt.Errorf("fs_delete: %w", err)
		}
	}

	return "ok", nil
}
