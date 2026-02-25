# tools

Tool execution and MCP (Model Context Protocol) integration for Shelly. The tools package provides a `Tool` type, a `ToolBox` orchestrator for agents to call tools, and MCP client/server implementations built on the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

## Architecture

```
tools/
├── toolbox/     Tool type and ToolBox orchestrator
├── mcpclient/   MCP client — communicates with external MCP server processes
└── mcpserver/   MCP server — exposes tools over the MCP protocol
```

**Dependency graph**: `toolbox` is the foundation. Both `mcpclient` and `mcpserver` depend on `toolbox` for the `Tool` type but are independent of each other. Both wrap the official MCP Go SDK for protocol handling.

### `toolbox` — Tool and ToolBox

`Tool` represents an executable tool with a name, description, JSON Schema, and handler:

| Field         | Type              | Description                          |
|---------------|-------------------|--------------------------------------|
| `Name`        | `string`          | Unique tool identifier               |
| `Description` | `string`          | Human-readable description           |
| `InputSchema` | `json.RawMessage` | JSON Schema for the tool's input     |
| `Handler`     | `Handler`         | Function that executes the tool      |

`Handler` is defined as `func(ctx context.Context, input json.RawMessage) (string, error)`.

`ToolBox` orchestrates a flat collection of tools (no parent-child relationships). Toolbox inheritance during agent delegation is handled by the agent layer (see `pkg/agent` README).

- `New()` — creates a new ToolBox
- `Register(...Tool)` — adds tools (replaces by name)
- `Get(name) (Tool, bool)` — retrieves a tool
- `Tools() []Tool` — lists all tools
- `Call(ctx, ToolCall) ToolResult` — executes a tool call and returns a result

### `mcpclient` — MCP Client

`MCPClient` communicates with an external MCP server process using the official MCP Go SDK:

- `New(ctx, command, ...args) (*MCPClient, error)` — spawns a server process and connects via stdio (initialization is automatic)
- `NewHTTP(ctx, url) (*MCPClient, error)` — connects to a Streamable HTTP MCP server at the given URL
- `ListTools(ctx) ([]toolbox.Tool, error)` — fetches tools (handlers call back through the client)
- `CallTool(ctx, name, arguments) (string, error)` — calls a tool on the server
- `Close() error` — terminates the session

### `mcpserver` — MCP Server

`MCPServer` serves tools over the MCP protocol using the official MCP Go SDK:

- `New(name, version) *MCPServer` — creates a server
- `Register(...toolbox.Tool)` — adds tools
- `Serve(ctx, in, out) error` — handles requests until the transport closes or ctx is cancelled

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
// Connect to an MCP server (ctx is required, initialization is automatic)
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

### Using HTTP-Based MCP Tools

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

// Serve over stdin/stdout
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
