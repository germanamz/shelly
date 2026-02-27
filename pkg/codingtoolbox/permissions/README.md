# permissions

Package `permissions` provides a shared, thread-safe store for persisting
permission grants to a single JSON file. It is the central trust mechanism
used by the `filesystem`, `exec`, `search`, `git`, `http`, and `browser` tool packages.

## Architecture

The **`Store`** manages three categories of grants:

- **Filesystem directories** -- approving a directory implicitly approves all
  its subdirectories (ancestor walk up the path tree). Used by `filesystem` and `search`.
- **Trusted commands** -- exact-match lookup by program name. Used by `exec` and `git`.
- **Trusted domains** -- exact-match lookup by hostname. Used by `http` and `browser`.

All categories are persisted to a single JSON file. Changes are written
atomically on every mutation via a temp-file-then-rename pattern, ensuring
the file is never left in a partial state.

## Exported API

### Types

- **`Store`** -- manages permission grants persisted to a JSON file. Thread-safe for concurrent use.

### Functions

- **`New(filePath string) (*Store, error)`** -- creates a Store backed by the given file. The path is resolved to absolute. Existing data is loaded immediately. Both the current object format and the legacy flat-array format are supported on read. Returns an error if the file exists but cannot be parsed.

### Methods on Store

**Filesystem directories:**
- **`IsDirApproved(dir string) bool`** -- reports whether `dir` or any of its ancestors has been approved.
- **`ApproveDir(dir string) error`** -- marks `dir` as approved and persists the change.

**Trusted commands:**
- **`IsCommandTrusted(cmd string) bool`** -- reports whether a command has been trusted.
- **`TrustCommand(cmd string) error`** -- marks a command as trusted and persists the change.

**Trusted domains:**
- **`IsDomainTrusted(domain string) bool`** -- reports whether a domain has been trusted.
- **`TrustDomain(domain string) error`** -- marks a domain as trusted and persists the change.

## File Format

```json
{
  "fs_directories": ["/home/user/projects"],
  "trusted_commands": ["git", "npm"],
  "trusted_domains": ["api.example.com"]
}
```

For backward compatibility the store also reads the legacy flat-array format
(a JSON array of directory strings) produced by earlier versions of the
filesystem tool. Files without `trusted_domains` are handled gracefully
(the field is treated as empty). Empty files are treated as having no grants.

## Usage

```go
store, err := permissions.New(".shelly/local/permissions.json")

// Filesystem permission checks.
store.IsDirApproved("/home/user/projects/foo") // true (ancestor match)
store.ApproveDir("/tmp/scratch")               // persists immediately

// Command trust checks.
store.IsCommandTrusted("git") // true
store.TrustCommand("npm")    // persists immediately

// Domain trust checks.
store.IsDomainTrusted("api.example.com") // true
store.TrustDomain("cdn.example.com")     // persists immediately
```

## Thread Safety

All methods are safe for concurrent use. Read operations (`Is*` methods) use
a read lock (`sync.RWMutex`). Mutations hold a write lock for the in-memory
update, take a snapshot, then release the lock before performing blocking I/O
(file write). This ensures persistence does not hold the mutex. Parent
directories for the permissions file are created automatically on first write.
