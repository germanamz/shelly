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
    InputSchema json.RawMessage // JSON Schema for the tool's input
    Handler     Handler
}
```

### ToolBox

Ordered collection of tools stored by insertion order with name-based index lookup.

```go
box := toolbox.New()
box.Add(tool)                    // Register a tool (overwrites if name exists, preserves position)
box.Get("name")                  // Retrieve by name → (Tool, bool)
box.All()                        // All tools in insertion order
box.Has("name")                  // Check existence
box.Filter("a", "b")            // New ToolBox with only listed names
box.Exclude("a", "b")           // New ToolBox without listed names
box.Merge(other)                 // Combine two ToolBoxes (other wins on collision)
```

**Key behavior:** `Add` overwrites the handler/description/schema of an existing tool but keeps its original insertion position. `Filter` and `Exclude` return new `ToolBox` instances (non-destructive).

---

## MCP Client: `pkg/tools/mcpclient/`

**Deps:** `tools/toolbox`, `mcp-go-sdk`

Communicates with MCP servers over **stdio** or **HTTP (Streamable HTTP)** transport using the official MCP Go SDK.

### Client Struct

```go
type Client struct {
    client   *mcp.Client
    session  *mcp.ClientSession
    cmd      *exec.Cmd        // non-nil for stdio transport
    mu       sync.Mutex
    toolbox  *toolbox.ToolBox  // lazily populated on first ToolBox() call
}
```

### Lifecycle

| Method | Purpose |
|--------|---------|
| `ConnectStdio(ctx, command, args, env)` | Start subprocess and connect via stdio |
| `ConnectHTTP(ctx, url)` | Connect via Streamable HTTP |
| `ToolBox(ctx)` | Lazily fetch tool list from server, build `toolbox.ToolBox` |
| `CallTool(ctx, name, args)` | Forward a tool call to the MCP server |
| `Close()` | Shut down session, kill process if stdio |

**Tool discovery:** On first `ToolBox()` call, the client calls `mcp.ListTools` on the server. Each MCP tool is wrapped in a `toolbox.Tool` whose `Handler` delegates to `CallTool`. The resulting `ToolBox` is cached.

### Roots Support

```go
c.AddRoots(roots...)          // Adds MCP roots and notifies connected servers
c.RemoveRoots(uris...)        // Removes roots by URI and notifies servers
DirToRoot(absPath) *mcp.Root  // Converts a directory path to a file:// Root
```

`DirToRoot` converts an absolute path to a `file://` URI `mcp.Root` with the directory name as the display name.

---

## MCP Server: `pkg/tools/mcpserver/`

**Deps:** `tools/toolbox`, `mcp-go-sdk`

Serves a `toolbox.ToolBox` over the MCP protocol. Exposes registered tools so external MCP clients can discover and call them.

### Server Struct

```go
type Server struct {
    server *mcp.Server
}
```

### API

```go
srv := mcpserver.New("name", "1.0", opts...)
srv.SetToolBox(box)           // Expose tools to MCP clients
srv.ServeStdio(ctx)           // Serve on stdin/stdout (blocks)
```

### Options

```go
mcpserver.WithRootsChangedHandler(func(roots []*mcp.Root) { ... })
```

Registers a callback that fires when a connected client's root list changes. Also exposed:

```go
RootPaths(roots []*mcp.Root) []string  // Extract filesystem paths from file:// root URIs
```

### Tool Registration

`SetToolBox` converts each `toolbox.Tool` to an `mcp.ServerTool` with a handler that unmarshals the MCP `CallToolParams`, invokes the tool's `Handler`, and wraps the result in `mcp.CallToolResult`. Errors from tools are returned as `isError: true` results rather than protocol-level errors.

---

## Built-in Coding Tools: `pkg/codingtoolbox/`

Each sub-package implements a specific tool category. All tools are registered into a `toolbox.ToolBox`.

### Tool Categories

| Sub-package | Tools | Description |
|------------|-------|-------------|
| `ask` | `ask_user` | Prompts the user and blocks until a response |
| `filesystem` | `fs_read`, `fs_write`, `fs_edit`, `fs_list`, `fs_delete`, `fs_move`, `fs_copy`, `fs_stat`, `fs_diff`, `fs_read_lines`, `fs_patch` | Permission-gated filesystem ops; uses `mcproots.IsPathAllowed` for root-based access control |
| `exec` | `exec_command` | Runs shell commands with timeout and permission gating |
| `search` | `search_files`, `search_content` | Glob-based file search and regex content search |
| `git` | `git_log`, `git_diff`, `git_show`, etc. | Git operations |
| `http` | `http_request` | HTTP client tool |
| `notes` | `shared_notes_read`, `shared_notes_write`, `shared_notes_append` | Persistent notes stored in `.shelly/local/notes/` |
| `permissions` | `permissions_grant` | Runtime permission grants |
| `defaults` | Various | Default tool bundle combining commonly used tools |
| `browser` | `browser_navigate`, `browser_*` | Browser automation tools |
| `skills` | `load_skill` | Load procedural knowledge from skill files |
| `tasks` | Task board tools | Exposed via `tasks.Store.Tools()` |
| `state` | State tools | Exposed via `state.Store.Tools()` |

### Permission Gating

Filesystem and exec tools check permissions before executing. Filesystem tools use `mcproots.IsPathAllowed(absPath, roots)` to restrict access based on MCP roots from context. The roots are extracted via `mcproots.FromContext(ctx)`.

### Registration Pattern

Each sub-package typically exports a function like `Register(box *toolbox.ToolBox, ...)` or `Tools(...) []toolbox.Tool` that the `codingtoolbox/defaults` package or the engine layer uses to assemble the complete toolbox.
