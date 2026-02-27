# filesystem

Package `filesystem` provides tools that give agents controlled access to the local filesystem.

## Permission Model

Every directory access is gated by explicit user permission. When an agent first accesses a path, the tool asks the user to approve the directory. Approving a directory implicitly approves all its subdirectories. Granted permissions are persisted to a JSON file so they survive restarts.

Symlinks are resolved to their real paths, and both the logical and real directories must be approved.

### File Change Confirmation

Write operations (`fs_write`, `fs_edit`, `fs_patch`, `fs_delete`, `fs_move`, `fs_copy`, `fs_mkdir`) show a diff or description of the change and ask the user for confirmation before applying. The user has three options:

- **yes** -- approve this single change.
- **no** -- deny the change.
- **trust this session** -- approve this change and skip confirmation for all future changes in this session.

Session trust is managed by the **`SessionTrust`** type and propagated via `context.Context`.

## Exported API

### Types

- **`FS`** -- provides filesystem tools with permission gating. Embeds a `FileLocker` for per-path concurrency safety.
- **`AskFunc`** -- `func(ctx context.Context, question string, options []string) (string, error)` callback for permission and confirmation prompts.
- **`NotifyFunc`** -- `func(ctx context.Context, message string)` non-blocking callback for displaying file changes when the session is trusted.
- **`FileLocker`** -- provides per-path mutual exclusion for filesystem operations. Lazily allocates a mutex for each path on first use.
- **`SessionTrust`** -- tracks whether the user has opted to trust all file changes for the current session. Thread-safe.

### Functions

- **`New(store *permissions.Store, askFn AskFunc, notifyFn NotifyFunc) *FS`** -- creates an FS backed by the given shared permissions store.
- **`NewFileLocker() *FileLocker`** -- creates a new FileLocker (used internally by `New`).
- **`WithSessionTrust(ctx context.Context, st *SessionTrust) context.Context`** -- returns a new context carrying the given SessionTrust.

### Methods on FS

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox containing all 11 filesystem tools.

### Methods on FileLocker

- **`Lock(path string)`** / **`Unlock(path string)`** -- acquire/release the mutex for a path.
- **`LockPair(p1, p2 string)`** / **`UnlockPair(p1, p2 string)`** -- acquire/release mutexes for two paths in sorted order to avoid deadlocks.

### Methods on SessionTrust

- **`IsTrusted() bool`** -- reports whether the session is trusted.
- **`Trust()`** -- marks the session as trusted.

## Tools

| Tool | Description |
|------|-------------|
| `fs_read` | Read file contents |
| `fs_write` | Write content to a file, creating parent directories as needed. Preserves existing file permissions; new files default to 0600. |
| `fs_edit` | Find-and-replace text in a file (old_text must appear exactly once). Supports modify, delete (omit new_text), and insert (include context in old_text). |
| `fs_list` | List directory entries (non-recursive) as JSON (`name`, `type`, `size`) |
| `fs_delete` | Delete a file or directory (set `recursive` for non-empty directories) |
| `fs_move` | Move or rename a file or directory |
| `fs_copy` | Copy a file or directory (directories copied recursively; symlinks outside source tree are skipped) |
| `fs_stat` | Get file/directory metadata (`name`, `size`, `mode`, `mod_time`, `is_dir`) |
| `fs_diff` | Show a unified diff between two files (3 lines of context) |
| `fs_patch` | Apply multiple find-and-replace hunks to a file in one atomic operation |
| `fs_mkdir` | Create a directory, including any necessary parent directories |

## Concurrency Safety

All file-modifying tools (`fs_write`, `fs_edit`, `fs_patch`, `fs_delete`, `fs_move`, `fs_copy`, `fs_mkdir`) acquire a per-path mutex before their read-modify-write cycle. This prevents concurrent agents (e.g. spawned via `delegate`) from clobbering each other's changes to the same file.

Two-path operations (`fs_move`, `fs_copy`) lock paths in sorted order to avoid deadlocks. Read-only tools (`fs_read`, `fs_list`, `fs_stat`, `fs_diff`) do not acquire locks since OS-level reads are atomic.

The `FileLocker` is created internally by `New()` -- no external wiring needed.

## Usage

```go
fs := filesystem.New(permStore, askFn, notifyFn)
tb := fs.Tools() // *toolbox.ToolBox with all 11 tools

// Enable session trust via context:
st := &filesystem.SessionTrust{}
ctx := filesystem.WithSessionTrust(context.Background(), st)
```

The `AskFunc` callback is called whenever permission for a new directory is needed or a file change requires confirmation. The `NotifyFunc` callback is called for non-blocking display of file changes when the session is trusted.

## Dependencies

- `pkg/codingtoolbox/permissions` -- shared permissions store (directory approval)
- `pkg/tools/toolbox` -- Tool and ToolBox types
- `github.com/pmezard/go-difflib` -- unified diff generation
