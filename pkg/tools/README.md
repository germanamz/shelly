# tools

Tool execution and MCP (Model Context Protocol) integration for Shelly. This package tree provides the `Tool` type, a `ToolBox` orchestrator for registering and dispatching tool calls, and MCP client/server implementations built on the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk). The top-level `tools` package itself (`doc.go`) is a documentation-only umbrella; all functionality lives in the subpackages.

## Architecture

```
tools/
├── doc.go         Documentation-only umbrella package
├── toolbox/       Tool type, Handler type, and ToolBox orchestrator
├── mcpclient/     MCP client — connects to external MCP servers (stdio or Streamable HTTP)
└── mcpserver/     MCP server — exposes toolbox.Tool instances over the MCP protocol
```

**Dependency graph**: `toolbox` is the foundation layer. Both `mcpclient` and `mcpserver` depend on `toolbox` for the `Tool` type but are independent of each other. Both wrap the official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk/mcp`) for protocol handling.

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

`ToolBox` orchestrates a flat, name-keyed collection of tools (no parent-child or hierarchical relationships). Toolbox inheritance during agent delegation is handled by the agent layer (`pkg/agent`), not here.

| Function / Method                                        | Description                                                              |
|----------------------------------------------------------|--------------------------------------------------------------------------|
| `New() *ToolBox`                                         | Creates a new empty ToolBox                                              |
| `(*ToolBox) Register(tools ...Tool)`                     | Adds one or more tools; replaces existing tools with the same name       |
| `(*ToolBox) Get(name string) (Tool, bool)`               | Retrieves a tool by name; returns false if not found                     |
| `(*ToolBox) Merge(other *ToolBox)`                       | Copies all tools from another ToolBox into this one; replaces by name    |
| `(*ToolBox) Tools() []Tool`                              | Returns all registered tools as a slice                                  |
| `(*ToolBox) Call(ctx context.Context, tc content.ToolCall) content.ToolResult` | Executes a tool call; returns a ToolResult with IsError on failure |

`Call` never returns a Go error. It returns a `content.ToolResult` with `IsError: true` in two cases: tool not found, or handler error. This allows the agent loop to always send a result back to the LLM.

Dependencies: `pkg/chats/content` (for `ToolCall` and `ToolResult` types).

### `mcpclient` -- MCP Client

`MCPClient` wraps the SDK's `mcp.Client` and `mcp.ClientSession`. It supports two transport modes:

1. **Command (stdio)** -- spawns a subprocess via `mcp.CommandTransport` and communicates over stdin/stdout
2. **Streamable HTTP** -- connects to a remote MCP server via `mcp.StreamableClientTransport`

Both constructors share an internal `newFromTransport` helper that creates the SDK client (implementation name `"shelly"`, version `"0.1.0"`), connects, and returns an initialized `MCPClient`.

| Function / Method                                                                      | Description                                                                 |
|----------------------------------------------------------------------------------------|-----------------------------------------------------------------------------|
| `New(ctx context.Context, command string, args ...string) (*MCPClient, error)`         | Spawns a server process and connects via stdio                              |
| `NewHTTP(ctx context.Context, url string) (*MCPClient, error)`                         | Connects to a Streamable HTTP MCP server at the given URL                   |
| `(*MCPClient) ListTools(ctx context.Context) ([]toolbox.Tool, error)`                  | Fetches tools; each returned Tool's Handler closure calls back through CallTool |
| `(*MCPClient) CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error)` | Calls a named tool on the server; joins multiple TextContent items with newlines |
| `(*MCPClient) Close() error`                                                           | Terminates the session (subprocess cleanup is handled by the SDK)           |

Dependencies: `pkg/tools/toolbox`, `github.com/modelcontextprotocol/go-sdk/mcp`.

### `mcpserver` -- MCP Server

`MCPServer` wraps the SDK's `mcp.Server`. Tools are registered via `Register`, which converts each `toolbox.Tool` to an SDK tool registration. Handler errors are mapped to MCP error responses (`IsError: true` with the error message as `TextContent`) rather than protocol-level errors.

| Function / Method                                                                    | Description                                                                        |
|--------------------------------------------------------------------------------------|------------------------------------------------------------------------------------|
| `New(name, version string) *MCPServer`                                               | Creates a server with the given implementation name and version                    |
| `(*MCPServer) Register(tools ...toolbox.Tool)`                                       | Adds one or more tools to the server                                               |
| `(*MCPServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error`         | Starts serving MCP requests; blocks until ctx is cancelled or the transport closes |

Internally, `Serve` wraps the reader/writer in an `mcp.IOTransport` (using a `nopWriteCloser` adapter for the writer) and delegates to the SDK's `server.Run`.

Dependencies: `pkg/tools/toolbox`, `github.com/modelcontextprotocol/go-sdk/mcp`.

## Use Cases

- **Agent tool dispatch**: The agent layer owns a `ToolBox`, registers built-in and MCP tools, and uses `Call` to dispatch `ToolCall` requests from the LLM in the ReAct loop.
- **MCP integration**: `mcpclient` fetches remote tools and converts them into `toolbox.Tool` instances, making MCP tools indistinguishable from built-in tools at the `ToolBox` level.
- **MCP server exposure**: `mcpserver` exposes any set of `toolbox.Tool` instances over the MCP protocol for external clients.

## Examples

### Registering and Calling Tools

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

// Agent receives a tool call from the LLM
tc := content.ToolCall{ID: "1", Name: "greet", Arguments: `{"name":"World"}`}
result := tb.Call(context.Background(), tc)
fmt.Println(result.Content) // "Hello, World!"
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

// Call MCP tools through the ToolBox like any other tool
tc := content.ToolCall{ID: "1", Name: "read_file", Arguments: `{"path":"/tmp/data.txt"}`}
result := tb.Call(ctx, tc)
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

### Agent Tool-Use Loop

```go
tb := toolbox.New()
// ... register tools ...

reply, _ := provider.Complete(ctx, chat)
chat.Append(reply)

for _, tc := range reply.ToolCalls() {
    result := tb.Call(ctx, tc)
    chat.Append(message.New("", role.Tool, result))
}

final, _ := provider.Complete(ctx, chat)
```
