# Orchestration Layer

Packages that enable multi-agent coordination, project awareness, and session persistence. All are in `pkg/`.

## pkg/state — Shared Key-Value Store

**File:** `store.go` | **Deps:** `tools/toolbox`

Thread-safe blackboard-pattern KV store. The zero value (`&state.Store{}`) is ready to use — internal maps and channels are lazily initialized via `sync.Once`.

### Data Model

- **Keys** are arbitrary strings; **values** are `json.RawMessage`.
- All operations are protected by `sync.RWMutex`.
- Supports **blocking watch**: `Watch(ctx, key)` blocks until the key is set or context cancels.

### API

```go
s := &state.Store{}
s.Set(ctx, "key", jsonValue)               // Write a key
val, err := s.Get(ctx, "key")              // Read a key (ErrNotFound if missing)
val, err := s.Watch(ctx, "key")            // Block until key exists, then return value
keys, err := s.List(ctx)                   // List all keys sorted alphabetically
err := s.Delete(ctx, "key")               // Remove a key
tools := s.Tools()                         // Returns []toolbox.Tool for agent use
```

### Watch Mechanism

`Watch` checks if the key exists; if so, returns immediately. Otherwise, it creates a notification channel per key (stored in `s.waiters[key]`) and blocks on it. When `Set` is called, it closes all waiter channels for that key, waking all blocked watchers.

### Tool Exposure

`Tools()` returns 5 tools for agent integration:
- `shared_state_get` — read a key
- `shared_state_set` — write a key (value is `json.RawMessage`)
- `shared_state_delete` — remove a key
- `shared_state_list` — list all keys
- `shared_state_watch` — block until key is available

---

## pkg/tasks — Shared Task Board

**File:** `store.go` | **Deps:** `agentctx`, `tools/toolbox`

Thread-safe task board for multi-agent coordination. Agents create, discover, claim, and complete tasks through normal tool-calling.

### Data Model

```go
type Task struct {
    ID          string            // UUID
    Title       string
    Description string
    Status      string            // pending | in_progress | completed | failed | canceled
    Assignee    string            // Agent name that claimed the task
    BlockedBy   []string          // Task IDs this task depends on
    Metadata    map[string]string // Arbitrary KV metadata
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

**Status transitions:** `pending` → `in_progress` (via `Claim`) → `completed`|`failed` (via `Update`). `pending`|`in_progress` → `canceled` (via `Cancel`). `Claim` sets `Assignee` from `agentctx.Name(ctx)`.

### Blocking Dependencies

A task is **blocked** if any of its `BlockedBy` task IDs are in a non-terminal status (`pending` or `in_progress`). The `List` method can filter by `blocked: true/false`.

### API

```go
store := tasks.NewStore()
task, err := store.Create(ctx, CreateParams{Title, Description, BlockedBy, Metadata})
task, err := store.Get(ctx, id)
tasks, err := store.List(ctx, ListParams{Status, Assignee, Blocked})
task, err := store.Claim(ctx, id)            // Sets status=in_progress, assignee=caller
task, err := store.Update(ctx, id, UpdateParams{Status, Description, BlockedBy, Metadata})
task, err := store.Cancel(ctx, id)           // Sets status=canceled
task, err := store.Watch(ctx, id)            // Block until task reaches terminal status
tools := store.Tools()                        // Returns []toolbox.Tool for agent use
```

### Watch Mechanism

`Watch` registers a waiter channel for a task ID. When any `Update` or `Cancel` brings a task to a terminal status (`completed`, `failed`, `canceled`), all waiters for that task are woken. Returns `ErrNotFound` if the task ID doesn't exist.

### Tool Exposure

`Tools()` returns 7 tools:
- `shared_tasks_create` — create a task with title, description, blocked_by, metadata
- `shared_tasks_list` — list tasks with optional status/assignee/blocked filters
- `shared_tasks_get` — get a single task by ID
- `shared_tasks_claim` — claim a pending task (sets assignee from agent context)
- `shared_tasks_update` — update status, description, blocked_by, metadata
- `shared_tasks_watch` — block until task reaches terminal status
- `shared_tasks_cancel` — cancel a pending or in-progress task

---

## pkg/sessions — Session Persistence

**Files:** `store.go`, `serialize.go`, `attachments.go` | **Deps:** `chats/message`, `chats/content`, `chats/role`

File-based session persistence using JSON serialization. Stores conversation history and binary attachments.

### Store

```go
type Store struct {
    dir string // Base directory for session files
}

func NewStore(dir string) *Store
```

### Session Lifecycle

| Method | Purpose |
|--------|---------|
| `Save(id, messages, meta)` | Serialize messages to `<dir>/<id>.json` with provider metadata |
| `Load(id)` | Deserialize messages from JSON file |
| `List()` | Return `[]SessionInfo` sorted by modification time (newest first) |
| `Delete(id)` | Remove session file and its attachment directory |

### SessionInfo & ProviderMeta

```go
type ProviderMeta struct {
    Kind  string `json:"kind"`   // Provider name (e.g., "anthropic")
    Model string `json:"model"`  // Model identifier
}

type SessionInfo struct {
    ID        string
    Title     string       // First 100 chars of first user message
    CreatedAt time.Time
    UpdatedAt time.Time
    Provider  ProviderMeta
}
```

### JSON Serialization (`serialize.go`)

Messages are serialized to a custom JSON format that preserves all content part types:

- **Content parts** are serialized with a `kind` discriminator: `text`, `tool_use`, `tool_result`, `image`, `document`, `thinking`, `redacted_thinking`, `server_tool_use`, `url`
- **Binary data** (images, documents) is offloaded to attachments via `AttachmentWriter`; the JSON stores a `ref` key pointing to the attachment
- **Deserialization** uses the `kind` field to reconstruct the correct `content.Part` type, loading binary data back through `AttachmentReader`

### Attachments (`attachments.go`)

Content-addressable storage for binary data (images, documents):

```go
type AttachmentWriter interface {
    WriteAttachment(data []byte, mediaType string) (ref string, err error)
}

type AttachmentReader interface {
    ReadAttachment(ref string) (data []byte, mediaType string, err error)
}
```

The `Store` implements both interfaces. Storage path: `<dir>/<sessionID>_attachments/<sha256hex>.<ext>`. The SHA-256 hash ensures deduplication. Media type is inferred from the file extension on read (the extension is derived from the media type on write).

---

## pkg/projectctx — Project Context Loading

**Files:** `projectctx.go`, `external.go`, `staleness.go` | **Deps:** `shellydir`

Loads curated context files and checks knowledge graph staleness. Combined context is injected into agent system prompts.

### Context Struct

```go
type Context struct {
    External string // Content from external AI tool context files
    Curated  string // Content from curated *.md files in .shelly/
    MaxRunes int    // Override for MaxContextRunes (default: 32000 ≈ 8000 tokens)
}
```

`Context.String()` concatenates External + Curated (external first, curated last so project-specific Shelly context takes precedence). Truncates at `MaxRunes` with a `[truncated]` marker.

### Loading

```go
ctx := projectctx.Load(dir, projectRoot, maxExternalFileSize)
```

1. **`LoadExternal(projectRoot, maxFileSize)`** — reads context files from other AI tools:
   - `CLAUDE.md` at project root
   - `.cursorrules` at project root (Cursor legacy)
   - `.cursor/rules/*.mdc` sorted alphabetically (Cursor modern, frontmatter stripped)
   - Each file capped at `DefaultMaxExternalFileSize` (512 KB)

2. **`LoadCurated(dir)`** — reads all `*.md` files from `.shelly/` root via `Dir.ContextFiles()` glob

### Staleness Detection

```go
stale := projectctx.IsKnowledgeStale(projectRoot, dir)
```

Compares `context.md` modification time against the latest git commit timestamp. Returns `true` if:
- `context.md` is missing
- Latest git commit is newer than `context.md`

Returns `false` (fail-open) if git is unavailable or the project isn't a git repo.

---

## pkg/mcproots — MCP Roots Utilities

**File:** `mcproots.go` | **Deps:** none (zero-dependency)

Shared utilities for MCP Roots protocol support. Used by both client side (sending approved directories) and server side (constraining filesystem access).

### Context Plumbing

```go
type contextKey struct{}
WithContext(ctx, roots []string) context.Context  // Store root paths in context
FromContext(ctx) []string                          // Retrieve root paths from context
```

### Path Checking

```go
IsPathAllowed(absPath string, roots []string) bool
```

Returns `true` if `absPath` equals or is under any of the provided root directories. Uses `filepath.Rel` to check containment — relative paths starting with `..` are rejected. Also returns `false` for empty roots list.

This is the mechanism used by `codingtoolbox/filesystem` and other tools to enforce root-based access control.
