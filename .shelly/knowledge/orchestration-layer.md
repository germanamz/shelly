# Orchestration Layer

Packages that enable multi-agent coordination, project awareness, and session persistence. All are in `pkg/`.

## pkg/state — Shared Key-Value Store

**File:** `store.go` | **Deps:** `tools/toolbox`

Thread-safe blackboard-pattern KV store. The zero value (`&state.Store{}`) is ready to use — internal maps and channels are lazily initialized via `sync.Once`.

### Data Model

All values are stored as `json.RawMessage`. Every Get/Set/Snapshot returns **deep copies** (`slices.Clone`) to prevent data races.

### Core API

| Method | Description |
|--------|-------------|
| `Get(key) (json.RawMessage, bool)` | Read a key (deep copy) |
| `Set(key, json.RawMessage)` | Write a key, notify watchers |
| `Delete(key)` | Remove a key, notify watchers |
| `Keys() []string` | Sorted list of all keys |
| `Snapshot() map[string]json.RawMessage` | Deep copy of entire store |
| `Watch(ctx, key) (json.RawMessage, error)` | Block until key exists or ctx canceled |

### Watch Mechanism

Uses a signal channel (`chan struct{}`). On every `Set` or `Delete`, the current channel is closed and a new one is created. Watchers select on `<-sig` and re-check the store.

### Tool Integration

`Store.Tools(namespace)` returns a `*toolbox.ToolBox` with three tools:
- `{namespace}_state_get` — get by key
- `{namespace}_state_set` — set key/value
- `{namespace}_state_list` — list all keys

Agents interact with shared state through their normal tool-calling loop.

---

## pkg/tasks — Shared Task Board

**File:** `store.go` | **Deps:** `agentctx`, `tools/toolbox`, `google/uuid`

Multi-agent task coordination. Same lazy-init pattern as state store.

### Task Lifecycle

```
pending → in_progress → completed
                      → failed
                      → canceled
```

Terminal states: `completed`, `failed`, `canceled` — no further transitions allowed.

### Task Struct

```go
type Task struct {
    ID          string         // "task-<uuid>", auto-assigned
    Title       string
    Description string
    Status      Status
    Assignee    string         // agent name, set by Claim/Reassign
    BlockedBy   []string       // IDs of blocking tasks
    Metadata    map[string]any // arbitrary KV pairs
    CreatedBy   string         // from agentctx
}
```

### Core API

| Method | Description |
|--------|-------------|
| `Create(task) (id, error)` | Add task as pending. ID & status forced. Validates BlockedBy refs. |
| `Get(id) (Task, bool)` | Copy of task |
| `List(filter) []Task` | Filter by status, assignee, blocked state. Sorted by ID. |
| `Update(id, Update) error` | Partial update: status, description, blocked_by, metadata |
| `Claim(id, agent) error` | Atomic assign + set in_progress. Rejects if blocked/terminal/other-assigned. |
| `Reassign(id, agent) error` | Like Claim but overrides existing assignee (for delegation transfer). |
| `Cancel(id) error` | Set canceled if pending/in_progress |
| `IsBlocked(id) bool` | True if any BlockedBy dep is not completed |

### Blocking Watchers

- **`WatchCompleted(ctx, id)`** — blocks until completed/failed/canceled. Has a 15s unclaimed timeout: if no agent claims the task within 15s, returns an error.
- **`WatchCanceled(ctx, id)`** — returns a channel closed when the task is canceled. Used to propagate cancellation to child work.
- **`Changes()`** — returns a channel closed on any store mutation. Call again after receive for next signal.

### Dual Signal Channels

The store maintains two channels:
- `signal` — wakes `WatchCompleted` / `WatchCanceled` (task-level signals)
- `changeCh` — wakes `Changes()` listeners (store-level change feed)

### Tool Integration

`Store.Tools(namespace)` exposes 7 tools: `create`, `list`, `get`, `claim`, `update`, `watch`, `cancel` — all prefixed with `{namespace}_tasks_`. The `claim` and `create` handlers use `agentctx.AgentNameFromContext(ctx)` to identify the calling agent.

### Key Design Decisions

- **Create rejects non-zero Status/Assignee** — prevents caller bugs from being silently discarded.
- **BlockedBy validated at creation** — self-references and missing IDs are errors.
- **Deep copies everywhere** — `copyTask` clones BlockedBy and Metadata.

---

## pkg/projectctx — Project Context Assembly

**Files:** `projectctx.go`, `external.go`, `staleness.go` | **Deps:** `shellydir`

Assembles project context for injection into agent system prompts.

### Context Struct

```go
type Context struct {
    External string // From CLAUDE.md, .cursorrules, .cursor/rules/*.mdc
    Curated  string // From .shelly/*.md files
    MaxRunes int    // Override for truncation limit (default: 32000 ≈ 8k tokens)
}
```

`Context.String()` concatenates External + Curated (curated last = higher precedence). Truncates at rune limit with `[truncated]` marker.

### Load Pipeline

```go
Load(dir, projectRoot, maxExternalFileSize) → Context{
    External: LoadExternal(projectRoot, maxSize),
    Curated:  LoadCurated(dir),
}
```

### External Context Sources (`external.go`)

Reads context files from other AI coding tools:

| Source | Path |
|--------|------|
| Claude Code | `{root}/CLAUDE.md` |
| Cursor legacy | `{root}/.cursorrules` |
| Cursor modern | `{root}/.cursor/rules/*.mdc` (sorted, frontmatter stripped) |

- Max file size: 512KB default (`DefaultMaxExternalFileSize`)
- Uses `io.LimitReader` for bounded reads
- `stripFrontmatter()` removes YAML `---` delimiters from `.mdc` files

### Curated Context (`projectctx.go`)

`LoadCurated(dir)` reads all `*.md` files from the `.shelly/` root (via `Dir.ContextFiles()`) and joins them with double newlines.

### Staleness Detection (`staleness.go`)

`IsKnowledgeStale(projectRoot, dir) bool` — compares `.shelly/context.md` mtime against the latest git commit timestamp (`git log -1 --format=%ct`).

- Missing context.md → stale (true)
- No git / not a repo → fail open (false)
- Commit newer than context.md → stale (true)

---

## pkg/mcproots — MCP Roots Utilities

**File:** `mcproots.go` | **Deps:** none (zero-dependency)

Shared utilities for MCP Roots protocol — context plumbing and path-checking for both client-side and server-side MCP code.

### API

| Function | Description |
|----------|-------------|
| `WithRoots(ctx, roots) context.Context` | Store root paths in context |
| `FromContext(ctx) []string` | Extract roots from context (`nil` = unconstrained) |
| `IsPathAllowed(absPath, roots) bool` | Check if path falls under any root |

### Semantics

| `roots` | Meaning |
|---------|---------|
| `nil` | Unconstrained — all paths allowed |
| `[]string{}` | Empty set — nothing allowed |
| `[]string{"/a"}` | Only paths equal to or under `/a` |

Path matching uses `filepath.Clean` and ensures the parent ends with a separator to prevent `/tmp` matching `/tmpfoo`.

### Why Zero-Dependency

Imported by `mcpclient`, `mcpserver`, and filesystem tools. Keeping it free of MCP SDK imports avoids circular dependencies. SDK-specific conversions (`DirToRoot`, `RootPaths`) live in their respective packages.

---

## pkg/sessions — Session Persistence

**Files:** `serialize.go`, `store.go`, `attachments.go` | **Deps:** `chats/content`, `chats/message`, `chats/role`

File-based session persistence using a directory-per-session (v2) layout.

### On-Disk Layout (v2)

```
{store_dir}/
  {session_id}/
    meta.json           # SessionInfo (ID, agent, provider, timestamps, preview)
    messages.json       # Serialized message array
    attachments/        # Content-addressable binary files
      {sha256}.png
      {sha256}.pdf
```

### SessionInfo

```go
type SessionInfo struct {
    ID        string       // session identifier
    Agent     string       // agent name
    Provider  ProviderMeta // {Kind, Model}
    CreatedAt time.Time
    UpdatedAt time.Time
    Preview   string       // first message preview
    MsgCount  int
}
```

### Store API

| Method | Description |
|--------|-------------|
| `New(dir, ...StoreOption)` | Create store. Options: `WithMaxAttachmentSize(n)` |
| `Save(info, msgs) error` | Atomic write via temp-file + rename. Cleans orphan attachments. |
| `Load(id) (info, msgs, error)` | Read session from directory |
| `List(opts) ([]SessionInfo, error)` | List all sessions, sorted by UpdatedAt desc. Supports Limit/Offset. |
| `Delete(id) error` | Remove session directory |
| `CleanAttachments(id) error` | Remove unreferenced attachment files |
| `MigrateV1() (int, error)` | Migrate legacy single-file `{id}.json` sessions to v2 layout |

### Serialization (`serialize.go`)

Uses a discriminated-union envelope (`jsonPart.Kind`):

| Kind | Content Type | Fields |
|------|-------------|--------|
| `text` | `content.Text` | text |
| `image` | `content.Image` | url, data/attachment_ref, media_type |
| `document` | `content.Document` | url (path), data/attachment_ref, media_type |
| `tool_call` | `content.ToolCall` | id, name, arguments, metadata |
| `tool_result` | `content.ToolResult` | tool_call_id, content, is_error |

Two serialization paths:
- `MarshalMessages` / `UnmarshalMessages` — inline binary data
- `MarshalMessagesWithAttachments` / `UnmarshalMessagesWithAttachments` — extract binaries to `AttachmentWriter`/`AttachmentReader`

### Attachment System (`attachments.go`)

**`FileAttachmentStore`** — content-addressable storage:
- **Write:** SHA-256 hash of data → `{hash}.{ext}`. Deduplicates (skips if file exists). Uses `atomicWrite`.
- **Read:** Infers media type from file extension.
- **Size limiting:** `sizeLimitedWriter` wraps a writer and rejects attachments over a configured limit.

### Atomic Writes

All file writes use `atomicWrite(tmpDir, target, data)`: create temp file in same directory → write → close → `os.Rename`. Rename is atomic on POSIX, ensuring no partial reads.

---

## Cross-Package Patterns

### Signal Channel Pattern (state + tasks)
Both `state.Store` and `tasks.Store` use the same notification pattern: close a `chan struct{}` to wake all waiters, then allocate a new channel. This provides efficient broadcast without goroutine leaks.

### Tool Integration Pattern
Both `state` and `tasks` expose a `Tools(namespace)` method returning a `*toolbox.ToolBox`. Tools are namespaced as `{namespace}_{store}_{operation}`. This lets agents interact with coordination primitives through their standard tool-calling loop.

### Zero-Value Ready
Both `state.Store` and `tasks.Store` use `sync.Once` for lazy initialization, making the zero value safe to use directly.

### Deep Copy Safety
Both stores clone all data on read/write boundaries to prevent callers from causing data races through shared references.
