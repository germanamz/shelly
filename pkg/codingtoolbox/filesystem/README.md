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

## Usage

```go
fs := filesystem.New(permStore, askFn)
tb := fs.Tools() // *toolbox.ToolBox with all 10 tools
```

The `AskFunc` callback is called whenever permission for a new directory is needed. It receives a question string and options (`["yes", "no"]`) and should return the user's response.
