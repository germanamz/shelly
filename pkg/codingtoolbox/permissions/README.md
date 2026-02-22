# permissions

Package `permissions` provides a shared, thread-safe store for persisting
permission grants to a single JSON file. It is the central trust mechanism
used by the `filesystem`, `exec`, `search`, `git`, and `http` tool packages.

## Architecture

The `Store` manages three categories of grants:

- **Filesystem directories** — approving a directory implicitly approves all
  its subdirectories (ancestor walk). Used by `filesystem` and `search`.
- **Trusted commands** — exact-match lookup by program name. Used by `exec` and `git`.
- **Trusted domains** — exact-match lookup by hostname. Used by `http`.

All categories are persisted to a single JSON file. Changes are written
atomically on every mutation (approve, trust command, or trust domain).

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
(the field is treated as empty).

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

All methods are safe for concurrent use. Read operations use a read lock;
mutations hold a write lock for the duration of the in-memory update and
file write.
