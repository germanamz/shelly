# tools

Tool execution and MCP (Model Context Protocol) integration for Shelly. This package tree provides the `Tool` type, a `ToolBox` orchestrator for registering and looking up tools, and MCP client/server implementations built on the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk). The top-level `tools` package itself (`doc.go`) is a documentation-only umbrella; all functionality lives in the subpackages.

## Architecture

```
tools/
├── doc.go         Documentation-only umbrella package
├── toolbox/       Tool type, Handler type, and ToolBox orchestrator
├── mcpclient/     MCP client — connects to external MCP servers (stdio or Streamable HTTP)
└── mcpserver/     MCP server — exposes toolbox.Tool instances over the MCP protocol
```

**Dependency graph**: `toolbox` is the foundation layer (zero external dependencies). Both `mcpclient` and `mcpserver` depend on `toolbox` for the `Tool` type but are independent of each other. Both wrap the official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk/mcp`) for protocol handling.

### `toolbox` -- Tool and ToolBox

`Handler` is the function signature for tool execution:

```go
type Handler func(ctx context.Context, input json.RawMessage) (string, error)
```

`Tool` represents an executable tool with a name, description, JSON Schema, and handler:

| Field         | Type              | Description                              |
|---------------|-------------------|------------------------------------------|
| `Name`        | `string`          | Unique tool identifier                   |
| `Description` | `string`          | Human-readable description for the LLM   |
| `InputSchema` | `json.RawMessage` | JSON Schema defining the tool's input    |
| `Handler`     | `Handler`         | Function that executes the tool          |

`ToolBox` orchestrates an insertion-ordered, name-indexed collection of tools (no parent-child or hierarchical relationships). Toolbox inheritance during agent delegation is handled by the agent layer (`pkg/agent`), not here.

| Function / Method                                        | Description                                                              |
|----------------------------------------------------------|--------------------------------------------------------------------------|
| `New() *ToolBox`                                         | Creates a new empty ToolBox                                              |
| `(*ToolBox) Register(tools ...Tool)`                     | Adds one or more tools; replaces existing tools in-place                 |
| `(*ToolBox) Get(name string) (Tool, bool)`               | Retrieves a tool by name; returns false if not found                     |
| `(*ToolBox) Merge(other *ToolBox)`                       | Copies all tools from another ToolBox into this one; replaces by name    |
| `(*ToolBox) Filter(names []string) *ToolBox`             | Returns a new ToolBox with only the named tools; nil returns the original, empty slice returns an empty ToolBox |
| `(*ToolBox) Tools() []Tool`                              | Returns all registered tools as a slice in insertion order               |
| `(*ToolBox) Len() int`                                   | Returns the number of registered tools                                   |

Tool dispatch (bridging `content.ToolCall` to `Handler`) is handled by the agent layer (`pkg/agent`), not by `ToolBox`.

### `mcpclient` -- MCP Client

`Client` wraps the SDK's `mcp.Client` and `mcp.ClientSession`. It supports two transport modes:

1. **Command (stdio)** -- spawns a subprocess via `mcp.CommandTransport` and communicates over stdin/stdout
2. **Streamable HTTP** -- connects to a remote MCP server via `mcp.StreamableClientTransport`

Both constructors share an internal `newFromTransport` helper that creates the SDK client (implementation name `"shelly"`, version `"0.1.0"`), connects, and returns an initialized `Client`.

| Function / Method                                                                      | Description                                                                 |
|----------------------------------------------------------------------------------------|-----------------------------------------------------------------------------|
| `New(ctx context.Context, command string, args ...string) (*Client, error)`            | Spawns a server process and connects via stdio                              |
| `NewHTTP(ctx context.Context, url string) (*Client, error)`                            | Connects to a Streamable HTTP MCP server at the given URL                   |
| `(*Client) ListTools(ctx context.Context) ([]toolbox.Tool, error)`                     | Fetches tools; each returned Tool's Handler closure calls back through CallTool |
| `(*Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error)` | Calls a named tool on the server; joins multiple TextContent items with newlines |
| `(*Client) Close() error`                                                              | Terminates the session (subprocess cleanup is handled by the SDK)           |

Dependencies: `pkg/tools/toolbox`, `github.com/modelcontextprotocol/go-sdk/mcp`.

### `mcpserver` -- MCP Server

`Server` wraps the SDK's `mcp.Server`. Tools are registered via `Register`, which converts each `toolbox.Tool` to an SDK tool registration. Handler errors are mapped to MCP error responses (`IsError: true` with the error message as `TextContent`) rather than protocol-level errors.

| Function / Method                                                                    | Description                                                                        |
|--------------------------------------------------------------------------------------|------------------------------------------------------------------------------------|
| `New(name, version string, opts ...Option) *Server`                                  | Creates a server with the given implementation name, version, and options           |
| `(*Server) Register(tools ...toolbox.Tool)`                                          | Adds one or more tools to the server                                               |
| `(*Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error`            | Starts serving MCP requests; blocks until ctx is cancelled or the transport closes |

Internally, `Serve` wraps the reader/writer in an `mcp.IOTransport` (using a `nopWriteCloser` adapter for the writer) and delegates to the SDK's `server.Run`.

Dependencies: `pkg/tools/toolbox`, `github.com/modelcontextprotocol/go-sdk/mcp`.

## Use Cases

- **Agent tool dispatch**: The agent layer owns `ToolBox` instances, registers built-in and MCP tools, and dispatches tool calls from the LLM in the ReAct loop.
- **MCP integration**: `mcpclient` fetches remote tools and converts them into `toolbox.Tool` instances, making MCP tools indistinguishable from built-in tools at the `ToolBox` level.
- **MCP server exposure**: `mcpserver` exposes any set of `toolbox.Tool` instances over the MCP protocol for external clients.

## Examples

### Registering and Looking Up Tools

```go
tb := toolbox.New()
tb.Register(toolbox.Tool{
    Name:        "greet",
    Description: "Returns a greeting",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
    Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
        var p struct{ Name string `json:"name"` }
        json.Unmarshal(input, &p)
        return "Hello, " + p.Name + "!", nil
    },
})

tool, ok := tb.Get("greet")
if ok {
    result, err := tool.Handler(ctx, json.RawMessage(`{"name":"World"}`))
    fmt.Println(result) // "Hello, World!"
}
```

### Using MCP Tools Through ToolBox

```go
// Connect to an MCP server (stdio transport)
client, _ := mcpclient.New(ctx, "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")
defer client.Close()

// Fetch tools and register them in a ToolBox
tools, _ := client.ListTools(ctx)
tb := toolbox.New()
tb.Register(tools...)
```

### Using Streamable HTTP Transport

```go
// Connect to a Streamable HTTP MCP server
client, _ := mcpclient.NewHTTP(ctx, "https://mcp.example.com/mcp?token=xxx")
defer client.Close()

// Same API as command-based clients
tools, _ := client.ListTools(ctx)
tb := toolbox.New()
tb.Register(tools...)
```

### Building an MCP Server

```go
server := mcpserver.New("my-server", "1.0.0")
server.Register(toolbox.Tool{
    Name:        "echo",
    Description: "Echoes the input",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
    Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
        var p struct{ Text string `json:"text"` }
        json.Unmarshal(input, &p)
        return p.Text, nil
    },
})

// Serve over stdin/stdout; blocks until done
server.Serve(context.Background(), os.Stdin, os.Stdout)
```
