package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/pmezard/go-difflib/difflib"
)

type diffInput struct {
	FileA string `json:"file_a"`
	FileB string `json:"file_b"`
}

func (f *FS) diffTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_diff",
		Description: "Show a unified diff between two files.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"file_a":{"type":"string","description":"Path to the first file"},"file_b":{"type":"string","description":"Path to the second file"}},"required":["file_a","file_b"]}`),
		Handler:     f.handleDiff,
	}
}

func (f *FS) handleDiff(ctx context.Context, input json.RawMessage) (string, error) {
	var in diffInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_diff: invalid input: %w", err)
	}

	if in.FileA == "" {
		return "", fmt.Errorf("fs_diff: file_a is required")
	}

	if in.FileB == "" {
		return "", fmt.Errorf("fs_diff: file_b is required")
	}

	if err := f.checkPermission(ctx, in.FileA); err != nil {
		return "", err
	}

	if err := f.checkPermission(ctx, in.FileB); err != nil {
		return "", err
	}

	absA, err := filepath.Abs(in.FileA)
	if err != nil {
		return "", fmt.Errorf("fs_diff: %w", err)
	}

	absB, err := filepath.Abs(in.FileB)
	if err != nil {
		return "", fmt.Errorf("fs_diff: %w", err)
	}

	dataA, err := os.ReadFile(absA) //nolint:gosec // path is approved by user
	if err != nil {
		return "", fmt.Errorf("fs_diff: %w", err)
	}

	dataB, err := os.ReadFile(absB) //nolint:gosec // path is approved by user
	if err != nil {
		return "", fmt.Errorf("fs_diff: %w", err)
	}

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(dataA)),
		B:        difflib.SplitLines(string(dataB)),
		FromFile: in.FileA,
		ToFile:   in.FileB,
		Context:  3,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return "", fmt.Errorf("fs_diff: %w", err)
	}

	if result == "" {
		return "files are identical", nil
	}

	return result, nil
}
