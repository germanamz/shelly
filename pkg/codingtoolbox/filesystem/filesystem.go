// Package filesystem provides tools that give agents controlled access to the
// local filesystem. Every directory access is gated by explicit user permission:
// when an agent first touches a path, the user is asked to approve. Approving a
// directory implicitly approves all its subdirectories. Granted permissions are
// persisted to the shared permissions store so they survive restarts.
package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// FS provides filesystem tools with permission gating.
type FS struct {
	store *permissions.Store
	ask   AskFunc
}

// New creates an FS backed by the given shared permissions store.
func New(store *permissions.Store, askFn AskFunc) *FS {
	return &FS{
		store: store,
		ask:   askFn,
	}
}

// Tools returns a ToolBox containing the filesystem tools.
func (f *FS) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(
		f.readTool(), f.writeTool(), f.editTool(), f.listTool(),
		f.deleteTool(), f.moveTool(), f.copyTool(), f.statTool(),
		f.diffTool(), f.patchTool(), f.mkdirTool(),
	)

	return tb
}

// --- permission helpers ---

// checkPermission ensures the directory of target is approved. It asks the user
// if not yet approved.
func (f *FS) checkPermission(ctx context.Context, target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("filesystem: resolve path: %w", err)
	}

	dir := abs
	info, statErr := os.Stat(abs)
	if statErr == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	} else if statErr != nil {
		// Path doesn't exist yet — use parent directory.
		dir = filepath.Dir(abs)
	}

	if f.store.IsDirApproved(dir) {
		return nil
	}

	resp, err := f.ask(ctx, fmt.Sprintf("Allow filesystem access to %s?", dir), []string{"yes", "no"})
	if err != nil {
		return fmt.Errorf("filesystem: ask permission: %w", err)
	}

	if !strings.EqualFold(resp, "yes") {
		return fmt.Errorf("filesystem: access denied to %s", dir)
	}

	return f.store.ApproveDir(dir)
}

// --- tool definitions ---

type pathInput struct {
	Path string `json:"path"`
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type editInput struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

func (f *FS) readTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_read",
		Description: "Read the contents of a file at the given path.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to read"}},"required":["path"]}`),
		Handler:     f.handleRead,
	}
}

func (f *FS) handleRead(ctx context.Context, input json.RawMessage) (string, error) {
	var in pathInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_read: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_read: path is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_read: %w", err)
	}

	data, err := os.ReadFile(abs) //nolint:gosec // path is approved by user
	if err != nil {
		return "", fmt.Errorf("fs_read: %w", err)
	}

	return string(data), nil
}

func (f *FS) writeTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_write",
		Description: "Write content to a file, creating parent directories as needed.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to write"},"content":{"type":"string","description":"Content to write"}},"required":["path","content"]}`),
		Handler:     f.handleWrite,
	}
}

func (f *FS) handleWrite(ctx context.Context, input json.RawMessage) (string, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_write: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_write: path is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_write: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", fmt.Errorf("fs_write: create dirs: %w", err)
	}

	if err := os.WriteFile(abs, []byte(in.Content), 0o600); err != nil {
		return "", fmt.Errorf("fs_write: %w", err)
	}

	return "ok", nil
}

func (f *FS) editTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_edit",
		Description: "Find and replace text in a file. The old_text must appear exactly once.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to edit"},"old_text":{"type":"string","description":"Text to find (must appear exactly once)"},"new_text":{"type":"string","description":"Replacement text"}},"required":["path","old_text","new_text"]}`),
		Handler:     f.handleEdit,
	}
}

func (f *FS) handleEdit(ctx context.Context, input json.RawMessage) (string, error) {
	var in editInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_edit: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_edit: path is required")
	}

	if in.OldText == "" {
		return "", fmt.Errorf("fs_edit: old_text is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_edit: %w", err)
	}

	data, err := os.ReadFile(abs) //nolint:gosec // path is approved by user
	if err != nil {
		return "", fmt.Errorf("fs_edit: %w", err)
	}

	content := string(data)
	count := strings.Count(content, in.OldText)

	if count == 0 {
		return "", fmt.Errorf("fs_edit: old_text not found in file")
	}

	if count > 1 {
		return "", fmt.Errorf("fs_edit: old_text appears %d times, must be unique", count)
	}

	newContent := strings.Replace(content, in.OldText, in.NewText, 1)
	if err := os.WriteFile(abs, []byte(newContent), 0o600); err != nil {
		return "", fmt.Errorf("fs_edit: %w", err)
	}

	return "ok", nil
}

// listEntry is returned by fs_list for each directory entry.
type listEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

func (f *FS) listTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_list",
		Description: "List entries in a directory. Returns JSON with name, type, and size.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the directory to list"}},"required":["path"]}`),
		Handler:     f.handleList,
	}
}

func (f *FS) handleList(ctx context.Context, input json.RawMessage) (string, error) {
	var in pathInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("fs_list: invalid input: %w", err)
	}

	if in.Path == "" {
		return "", fmt.Errorf("fs_list: path is required")
	}

	if err := f.checkPermission(ctx, in.Path); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return "", fmt.Errorf("fs_list: %w", err)
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", fmt.Errorf("fs_list: %w", err)
	}

	result := make([]listEntry, 0, len(entries))
	for _, e := range entries {
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}

		typ := "file"
		if e.IsDir() {
			typ = "dir"
		}

		result = append(result, listEntry{
			Name: e.Name(),
			Type: typ,
			Size: info.Size(),
		})
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("fs_list: marshal: %w", err)
	}

	return string(data), nil
}
