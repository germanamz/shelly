# Tool System

The tool system spans two package trees: **`pkg/tools/`** (core abstraction, MCP protocol) and **`pkg/codingtoolbox/`** (built-in coding tools with permission gating).

---

## Core Abstraction: `pkg/tools/toolbox/`

### Tool Type

```go
// Handler executes a tool with JSON input and returns a text result.
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage   // JSON Schema for input
    Handler     Handler
}
```

Every tool — built-in or MCP-sourced — is a `Tool` value. Input/output is always `json.RawMessage` → `string`.

### ToolBox

Ordered collection of tools with name-based deduplication:

| Method | Purpose |
|--------|---------|
| `New() *ToolBox` | Create empty toolbox |
| `Register(tools ...Tool)` | Add tools (overwrites by name) |
| `Get(name string) (Tool, bool)` | Lookup by name |
| `Merge(other *ToolBox)` | Copy all tools from another toolbox |
| `Tools() []Tool` | Insertion-ordered list |
| `Len() int` | Tool count |
| `Filter(names []string) *ToolBox` | New toolbox with only named tools |

Tools are stored in insertion order via `items []Tool` with an `index map[string]int` for O(1) lookup.

---

## JSON Schema Generation: `pkg/tools/schema/`

Generates JSON Schema from Go struct types via reflection:

```go
func Generate[T any]() json.RawMessage
```

Struct tag conventions:
- **`json:"name"`** → field name in schema
- **`json:"name,omitempty"`** → field is optional (not in `required`)
- **`desc:"..."`** → field description
- Supports: `string`, `int`, `float64`, `bool`, `[]T`, nested structs, `map[string]T`

Used by all built-in tools to define their input schemas from Go structs.

---

## MCP Client: `pkg/tools/mcpclient/`

Communicates with external MCP tool servers. Two transport modes:

```go
// Stdio transport — spawns a subprocess
func New(ctx context.Context, command string, args ...string) (*Client, error)

// HTTP/SSE transport — connects to a URL
func NewHTTP(ctx context.Context, url string) (*Client, error)
```

### Key Methods

| Method | Purpose |
|--------|---------|
| `ListTools(ctx) ([]Tool, error)` | Discover tools from server, converts MCP schemas to `toolbox.Tool` |
| `CallTool(ctx, name, args) (string, error)` | Execute a tool, returns text content |
| `Close()` | Shutdown client (kills stdio subprocess, cancels HTTP) |
| `AddRoots(roots...)` | Expose filesystem roots to server |
| `RemoveRoots(uris...)` | Remove roots |
| `MakeRootURI(path) string` | Helper to create `file://` URIs |

Wraps the official `github.com/modelcontextprotocol/go-sdk/mcp` SDK. Stdio manages subprocess lifecycle (spawn + reap). HTTP uses SSE transport. `sync.Once` ensures clean shutdown. `ListTools` converts MCP tools → `toolbox.Tool` by marshaling schemas and creating handlers that delegate to `CallTool`.

---

## MCP Server: `pkg/tools/mcpserver/`

Exposes `toolbox.Tool` values over MCP protocol:

```go
func New(name, version string, opts ...Option) *Server

// Option: WithRootsChangedHandler(func([]*mcp.Root))
```

### Key Methods

| Method | Purpose |
|--------|---------|
| `Register(tools ...Tool)` | Register tools as MCP tool handlers |
| `Serve(ctx, in, out) error` | Serve over stdio (reader/writer pair) |

`Register` converts each `toolbox.Tool` into an MCP tool handler. `Serve` runs the MCP protocol over provided I/O streams. Supports the optional `WithRootsChangedHandler` for tracking client workspace roots.

---

## Built-in Coding Tools: `pkg/codingtoolbox/`

### Package Structure

| Sub-package | Tools | Permission Gate |
|-------------|-------|-----------------|
| `filesystem/` | `fs_read`, `fs_read_lines`, `fs_write`, `fs_edit`, `fs_list`, `fs_copy`, `fs_delete`, `fs_diff`, `fs_mkdir`, `fs_move`, `fs_patch`, `fs_stat` | Directory approval |
| `exec/` | `exec_run` | Command trust |
| `search/` | `search_content`, `search_files` | Directory approval |
| `git/` | `git_status`, `git_diff`, `git_log`, `git_commit` | Command trust (for "git") |
| `http/` | `http_fetch` | Domain approval |
| `notes/` | `write_note`, `read_note`, `list_notes` | None (writes to `.shelly/local/notes/`) |
| `ask/` | `ask_user` | None |
| `permissions/` | (no tools — shared store) | N/A |
| `defaults/` | (no tools — toolbox builder) | N/A |

### Permission & Approval System

#### Permissions Store (`permissions/store.go`)

Thread-safe, JSON-persisted store managing two kinds of trust:

```go
type Store struct {
    mu   sync.Mutex
    path string         // JSON file on disk
    data permissionsData // { dirs: [], commands: [] }
}
```

- **`IsDirAllowed(path) bool`** — checks if path (or any ancestor) is approved
- **`AddDir(path)`** — grants directory access
- **`IsCommandAllowed(cmd) bool`** — checks if program name is trusted
- **`AddCommand(cmd)`** — trusts a command
- **`IsDomainAllowed(domain) bool`** — checks if HTTP domain is trusted
- **`AddDomain(domain)`** — trusts a domain

Changes are persisted atomically to disk. Approving a directory approves all subdirectories.

#### Approver (`approver.go`)

Coalesces concurrent approval requests for the same key:

```go
type Approver struct {
    mu      sync.Mutex
    pending map[string]*pendingApproval
}

func (a *Approver) Ensure(ctx context.Context, key string, isApproved func() bool, approveFn func(ctx) ApprovalOutcome) error
```

Flow: `Ensure` checks `isApproved()` → if false, calls `approveFn` → concurrent callers on the same key share the result via a channel.

**Filesystem/Search** → directory approval (subdirs inherit). **Exec** → command trust (by program name). **Git** → command trust for "git". **HTTP** → domain approval.

### Filesystem Tools (`filesystem/`)

Central type: **`FS`** struct holding:
- `perms *permissions.Store` — shared permission store
- `approver *codingtoolbox.Approver` — deduplicates approval prompts  
- `askFn codingtoolbox.AskFunc` — prompts user for approval
- `sessionTrust *SessionTrust` — if trusted, skip confirmations for writes
- `notifyFn NotifyFunc` — non-blocking notification for trusted sessions
- `locker *FileLocker` — prevents concurrent writes to the same file

**FileLocker**: Per-file mutex using `sync.Map` of `*sync.Mutex`. Ensures only one tool writes to a given path at a time.

**SessionTrust**: Boolean flag (thread-safe) — when trusted, write operations show a diff notification instead of asking for confirmation.

**Write confirmation flow**: For `fs_write` and `fs_edit` — if session not trusted, computes a unified diff and asks user to approve the change via `confirmChange()`.

**Constructor**: `New(perms, approver, askFn, opts...)` with options:
- `WithSessionTrust(*SessionTrust)` — enables session trust mode
- `WithNotifyFunc(NotifyFunc)` — sets notification callback

### Exec Tool (`exec/`)

```go
type Exec struct {
    perms    *permissions.Store
    approver *codingtoolbox.Approver
    askFn    codingtoolbox.AskFunc
}
```

Runs shell commands via `codingtoolbox.RunCmd()`. Output captured with `LimitedBuffer` (1MB max). Uses the shared approver for command trust.

### Search Tools (`search/`)

Two tools:
- **`search_content`** — regex search in file contents with optional context lines
- **`search_files`** — glob-based file name search (supports `**`)

Both gated by directory permissions. Skips binary files (non-UTF8), `.git/` directories, and respects max result limits.

### Git Tools (`git/`)

Wraps git commands (`status`, `diff`, `log`, `commit`) with structured input schemas. All gated by command trust for the `git` binary. Runs via `codingtoolbox.RunCmd()`.

### HTTP Tool (`http/`)

Single `http_fetch` tool. Supports `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`. Features:
- Domain-level permission gating
- Response body size limit (1MB)
- Timeout support
- Returns status code + headers + body as formatted text

### Notes Tools (`notes/`)

Persistent markdown notes in `.shelly/local/notes/`. No permission gating needed.
- **`write_note`** — create/overwrite a note
- **`read_note`** — read a note by name
- **`list_notes`** — list all saved notes

Notes survive context compaction — agents use them to preserve important information.

### Ask Tool (`ask/`)

Prompts the user with a question (free-form or multiple choice):

```go
type Responder struct {
    onAsk   OnAskFunc     // callback to display question
    pending sync.Map      // id → response channel
    nextID  atomic.Uint64
}
```

The `Responder` manages pending questions. `OnAskFunc` displays the question to the user; `Respond(id, answer)` delivers the answer back to the waiting tool handler.

### Defaults Builder (`defaults/`)

```go
func New(toolboxes ...*toolbox.ToolBox) *toolbox.ToolBox
```

Merges multiple toolboxes into one. Later toolboxes overwrite earlier ones by name. Used by the engine to compose the complete tool set.

---

## Shared Utilities in `codingtoolbox/`

| Type/Func | Purpose |
|-----------|---------|
| `AskFunc` | `func(ctx, question, options) (string, error)` — user prompt callback |
| `Approver` | Coalesces concurrent approval requests by key |
| `LimitedBuffer` | Byte buffer that silently discards writes beyond a cap |
| `RunCmd(*exec.Cmd) (string, error)` | Executes command with stdout/stderr captured via LimitedBuffer |
| `MaxBufferSize` | 1MB limit for command output capture |

---

## Dependency Flow

```
schema.Generate[T]() ──→ Tool.InputSchema
                              │
toolbox.ToolBox ◄─── filesystem.FS.Tools()
       │              exec.Exec.Tools()
       │              search.Search.Tools()
       │              git.Git.Tools()
       │              http.HTTP.Tools()
       │              notes.New().Tools()
       │              ask.Responder.Tools()
       │
       ├──→ mcpserver.Server.Register()   (expose over MCP)
       │
       └──→ defaults.New(toolboxes...)    (merge for agent use)

mcpclient.Client.ListTools() ──→ []toolbox.Tool  (import from MCP)
                                        │
                                        └──→ toolbox.ToolBox.Register()

permissions.Store ──→ filesystem.FS, exec.Exec, search.Search, git.Git, http.HTTP
codingtoolbox.Approver ──→ filesystem.FS, exec.Exec, search.Search, git.Git, http.HTTP
```

---

## Key Patterns

1. **Uniform Tool Interface**: All tools (built-in + MCP) are `toolbox.Tool` values with `Handler func(ctx, json.RawMessage) (string, error)`
2. **Schema from Structs**: `schema.Generate[T]()` eliminates hand-written JSON schemas
3. **Permission Gating**: All environment-affecting tools require user approval. Approvals are persisted and shared across tool categories
4. **Approval Coalescing**: `Approver.Ensure()` prevents duplicate prompts when multiple tools hit the same resource concurrently
5. **Output Limiting**: `LimitedBuffer` (1MB) prevents runaway output from commands
6. **File Locking**: `FileLocker` prevents concurrent writes to the same file path
7. **Session Trust**: Filesystem writes can skip per-file confirmation when session is trusted
8. **MCP Bridge**: MCP client converts external tools → `toolbox.Tool`; MCP server converts `toolbox.Tool` → MCP protocol. Same type on both sides.
