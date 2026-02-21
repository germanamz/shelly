package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type statOutput struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
}

func (f *FS) statTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_stat",
		Description: "Get file or directory metadata (name, size, mode, modification time, is_dir).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to stat"}},"required":["path"]}`),
		Handler:     f.handleStat,
	}
}

func (f *FS) handleStat(ctx context.Context, input json.RawMessage) (string, error) {
	var in pathInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_stat: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_stat: path is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_stat: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("fs_stat: %w", err)
	}

	out := statOutput{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		IsDir:   info.IsDir(),
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("fs_stat: marshal: %w", err)
	}

	return string(data), nil
}
