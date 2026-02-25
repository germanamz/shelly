package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type moveInput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func (f *FS) moveTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_move",
		Description: "Move or rename a file or directory.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"source":{"type":"string","description":"Source path"},"destination":{"type":"string","description":"Destination path"}},"required":["source","destination"]}`),
		Handler:     f.handleMove,
	}
}

func (f *FS) handleMove(ctx context.Context, input json.RawMessage) (string, error) {
	var in moveInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_move: invalid input: %w", err)
	}

	if in.Source == "" {
		return "", fmt.Errorf("fs_move: source is required")
	}

	if in.Destination == "" {
		return "", fmt.Errorf("fs_move: destination is required")
	}

	if err := f.checkPermission(ctx, in.Source); err != nil {
		return "", err
	}

	if err := f.checkPermission(ctx, in.Destination); err != nil {
		return "", err
	}

	absSrc, err := filepath.Abs(in.Source)
	if err != nil {
		return "", fmt.Errorf("fs_move: %w", err)
	}

	absDst, err := filepath.Abs(in.Destination)
	if err != nil {
		return "", fmt.Errorf("fs_move: %w", err)
	}

	f.locker.LockPair(absSrc, absDst)
	defer f.locker.UnlockPair(absSrc, absDst)

	diff := fmt.Sprintf("Move: %s -> %s", absSrc, absDst)
	if err := f.confirmChange(ctx, absSrc, diff); err != nil {
		return "", fmt.Errorf("fs_move: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absDst), 0o750); err != nil {
		return "", fmt.Errorf("fs_move: create dirs: %w", err)
	}

	if err := os.Rename(absSrc, absDst); err != nil {
		return "", fmt.Errorf("fs_move: %w", err)
	}

	return "ok", nil
}
