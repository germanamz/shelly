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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// fileMode returns the existing file's permission bits, or 0o600 for new files.
func fileMode(path string) fs.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0o600
	}

	return info.Mode().Perm()
}

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// pendingResult holds the outcome of a single in-flight permission prompt so
// that concurrent callers waiting on the same directory can share the result.
type pendingResult struct {
	done chan struct{}
	err  error
}

// FS provides filesystem tools with permission gating.
type FS struct {
	store      *permissions.Store
	ask        AskFunc
	notify     NotifyFunc
	locker     *FileLocker
	pendingMu  sync.Mutex
	pendingDir map[string]*pendingResult
}

// New creates an FS backed by the given shared permissions store.
func New(store *permissions.Store, askFn AskFunc, notifyFn NotifyFunc) *FS {
	return &FS{
		store:      store,
		ask:        askFn,
		notify:     notifyFn,
		locker:     NewFileLocker(),
		pendingDir: make(map[string]*pendingResult),
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
// if not yet approved. Concurrent calls for the same directory coalesce into a
// single prompt so the user is never asked the same question multiple times.
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
		dir = filepath.Dir(abs)
	}

	if err := f.approveDir(ctx, dir); err != nil {
		return err
	}

	realAbs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("filesystem: resolve symlink: %w", err)
		}
		return nil
	}

	if realAbs == abs {
		return nil
	}

	realDir := realAbs
	realInfo, statErr := os.Stat(realAbs)
	if statErr == nil && !realInfo.IsDir() {
		realDir = filepath.Dir(realAbs)
	} else if statErr != nil {
		realDir = filepath.Dir(realAbs)
	}

	return f.approveDir(ctx, realDir)
}

func (f *FS) approveDir(ctx context.Context, dir string) error {
	if f.store.IsDirApproved(dir) {
		return nil
	}

	f.pendingMu.Lock()
	if f.store.IsDirApproved(dir) {
		f.pendingMu.Unlock()
		return nil
	}

	if pr, ok := f.pendingDir[dir]; ok {
		f.pendingMu.Unlock()
		<-pr.done

		if pr.err != nil {
			return pr.err
		}

		return nil // Permission was granted (either "yes" or "trust")
	}

	pr := &pendingResult{done: make(chan struct{})}
	f.pendingDir[dir] = pr
	f.pendingMu.Unlock()

	pr.err = f.askAndApproveDir(ctx, dir)

	close(pr.done)
	f.pendingMu.Lock()
	delete(f.pendingDir, dir)
	f.pendingMu.Unlock()

	return pr.err
}

// askAndApproveDir prompts the user and approves the directory on "yes".
func (f *FS) askAndApproveDir(ctx context.Context, dir string) error {
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
		Description: "Read the contents of a file at the given path. Use this to inspect file contents before editing. Returns the full file content as text.",
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

	file, err := os.Open(abs) //nolint:gosec // path is approved by user
	if err != nil {
		return "", fmt.Errorf("fs_read: %w", err)
	}
	defer file.Close() //nolint:errcheck // best-effort close on read

	const maxReadSize = 10 << 20 // 10 MB
	data, err := io.ReadAll(io.LimitReader(file, int64(maxReadSize)+1))
	if err != nil {
		return "", fmt.Errorf("fs_read: %w", err)
	}

	if len(data) > maxReadSize {
		return "", fmt.Errorf("fs_read: file exceeds maximum read size of 10 MB")
	}

	return string(data), nil
}

func (f *FS) writeTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_write",
		Description: "Write content to a file, creating parent directories as needed. Use for creating new files or full file rewrites. For targeted edits to existing files, prefer fs_edit instead.",
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

	f.locker.Lock(abs)
	defer f.locker.Unlock(abs)

	// Read existing content for diff (empty if file doesn't exist yet).
	oldContent := ""
	if data, readErr := os.ReadFile(abs); readErr == nil { //nolint:gosec // path is approved by user
		oldContent = string(data)
	}

	diff := computeDiff(abs, oldContent, in.Content)
	if diff != "" {
		if err := f.confirmChange(ctx, abs, diff); err != nil {
			return "", fmt.Errorf("fs_write: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", fmt.Errorf("fs_write: create dirs: %w", err)
	}

	if err := os.WriteFile(abs, []byte(in.Content), fileMode(abs)); err != nil {
		return "", fmt.Errorf("fs_write: %w", err)
	}

	return "ok", nil
}

func (f *FS) editTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "fs_edit",
		Description: "Edit a file by finding and replacing text. The old_text must appear exactly once. Supports three operations: modify (replace old_text with new_text), delete (omit new_text to remove old_text), and insert (include surrounding context in old_text and add new content in new_text). Use for targeted edits. For full rewrites, use fs_write instead. Always read the file first with fs_read to get exact text to match.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to edit"},"old_text":{"type":"string","description":"Text to find (must appear exactly once)"},"new_text":{"type":"string","description":"Replacement text. Omit or set to empty string to delete the matched text."}},"required":["path","old_text"]}`),
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

	f.locker.Lock(abs)
	defer f.locker.Unlock(abs)

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

	diff := computeDiff(abs, content, newContent)
	if diff != "" {
		if err := f.confirmChange(ctx, abs, diff); err != nil {
			return "", fmt.Errorf("fs_edit: %w", err)
		}
	}

	if err := os.WriteFile(abs, []byte(newContent), fileMode(abs)); err != nil {
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
		Description: "List entries in a directory (non-recursive). Returns JSON with name, type, and size. For recursive file discovery, use search_files instead.",
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
