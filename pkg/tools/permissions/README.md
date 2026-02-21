# permissions

Package `permissions` provides a shared, thread-safe store for persisting
permission grants to a single JSON file. It is the central trust mechanism
used by the `filesystem` and `exec` tool packages.

## Architecture

The `Store` manages two categories of grants:

- **Filesystem directories** — approving a directory implicitly approves all
  its subdirectories (ancestor walk).
- **Trusted commands** — exact-match lookup by program name.

Both categories are persisted to a single JSON file. Changes are written
atomically on every mutation (approve or trust).

## File Format

```json
{
  "fs_directories": ["/home/user/projects"],
  "trusted_commands": ["git", "npm"]
}
```

For backward compatibility the store also reads the legacy flat-array format
(a JSON array of directory strings) produced by earlier versions of the
filesystem tool.

## Usage

```go
store, err := permissions.New(".shelly/permissions.json")

// Filesystem permission checks.
store.IsDirApproved("/home/user/projects/foo") // true (ancestor match)
store.ApproveDir("/tmp/scratch")               // persists immediately

// Command trust checks.
store.IsCommandTrusted("git") // true
store.TrustCommand("npm")    // persists immediately
```

## Thread Safety

All methods are safe for concurrent use. Read operations use a read lock;
mutations hold a write lock for the duration of the in-memory update and
file write.
