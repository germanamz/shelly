package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type hunk struct {
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

type patchInput struct {
	Path  string `json:"path"`
	Hunks []hunk `json:"hunks"`
}

func (f *FS) patchTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_patch",
		Description: "Apply multiple edits to a file in one atomic operation. Each hunk finds and replaces text, applied sequentially. Each hunk's old_text must appear exactly once. Supports modify, delete (omit new_text), and insert (include context in old_text, add new content in new_text).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to patch"},"hunks":{"type":"array","items":{"type":"object","properties":{"old_text":{"type":"string","description":"Text to find (must appear exactly once)"},"new_text":{"type":"string","description":"Replacement text. Omit or set to empty string to delete the matched text."}},"required":["old_text"]},"description":"Hunks to apply sequentially"}},"required":["path","hunks"]}`),
		Handler:     f.handlePatch,
	}
}

func (f *FS) handlePatch(ctx context.Context, input json.RawMessage) (string, error) {
	var in patchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_patch: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_patch: path is required")
	}

	if len(in.Hunks) == 0 {
		return "", fmt.Errorf("fs_patch: at least one hunk is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_patch: %w", err)
	}

	data, err := os.ReadFile(abs) //nolint:gosec // path is approved by user
	if err != nil {
		return "", fmt.Errorf("fs_patch: %w", err)
	}

	content := string(data)

	for i, h := range in.Hunks {
		if h.OldText == "" {
			return "", fmt.Errorf("fs_patch: hunk %d: old_text is required", i)
		}

		count := strings.Count(content, h.OldText)
		if count == 0 {
			return "", fmt.Errorf("fs_patch: hunk %d: old_text not found in file", i)
		}

		if count > 1 {
			return "", fmt.Errorf("fs_patch: hunk %d: old_text appears %d times, must be unique", i, count)
		}

		content = strings.Replace(content, h.OldText, h.NewText, 1)
	}

	if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("fs_patch: %w", err)
	}

	return "ok", nil
}
