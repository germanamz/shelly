# filesystem

Package filesystem provides tools that give agents controlled access to the local filesystem.

## Permission Model

Every directory access is gated by explicit user permission. When an agent first accesses a path, the tool asks the user to approve the directory. Approving a directory implicitly approves all its subdirectories. Granted permissions are persisted to a JSON file so they survive restarts.

## Tools

| Tool | Description |
|------|-------------|
| `fs_read` | Read file contents |
| `fs_write` | Write content to a file, creating parent directories as needed |
| `fs_edit` | Find-and-replace text in a file (old_text must appear exactly once) |
| `fs_list` | List directory entries as JSON (`name`, `type`, `size`) |
| `fs_delete` | Delete a file or directory (set `recursive` for non-empty directories) |
| `fs_move` | Move or rename a file or directory |
| `fs_copy` | Copy a file or directory (directories are copied recursively) |
| `fs_stat` | Get file/directory metadata (`name`, `size`, `mode`, `mod_time`, `is_dir`) |
| `fs_diff` | Show a unified diff between two files |
| `fs_patch` | Apply multiple find-and-replace hunks to a file |

## Concurrency Safety

All file-modifying tools (`fs_write`, `fs_edit`, `fs_patch`, `fs_delete`, `fs_move`, `fs_copy`, `fs_mkdir`) acquire a per-path mutex before their read-modify-write cycle. This prevents concurrent agents (e.g. spawned via `delegate`) from clobbering each other's changes to the same file.

Two-path operations (`fs_move`, `fs_copy`) lock paths in sorted order to avoid deadlocks. Read-only tools (`fs_read`, `fs_list`, `fs_stat`, `fs_diff`) do not acquire locks since OS-level reads are atomic.

The `FileLocker` is created internally by `FS.New()` — no external wiring needed.

## Usage

```go
fs := filesystem.New(permStore, askFn, notifyFn)
tb := fs.Tools() // *toolbox.ToolBox with all 11 tools
```

The `AskFunc` callback is called whenever permission for a new directory is needed. It receives a question string and options (`["yes", "no"]`) and should return the user's response.
