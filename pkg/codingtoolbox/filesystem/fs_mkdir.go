package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

func (f *FS) mkdirTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_mkdir",
		Description: "Create a directory, including any necessary parent directories.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path of the directory to create"}},"required":["path"]}`),
		Handler:     f.handleMkdir,
	}
}

func (f *FS) handleMkdir(ctx context.Context, input json.RawMessage) (string, error) {
	var in pathInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_mkdir: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_mkdir: path is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_mkdir: %w", err)
	}

	if err := os.MkdirAll(abs, 0o750); err != nil {
		return "", fmt.Errorf("fs_mkdir: %w", err)
	}

	return "ok", nil
}
