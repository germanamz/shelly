# mcpclient

MCP (Model Context Protocol) client for Shelly. Connects to external MCP server processes (via stdio or Streamable HTTP) and exposes their tools as `toolbox.Tool` instances that can be registered in a `ToolBox` and used like any other tool.

Built as a thin wrapper around the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

## Architecture

`MCPClient` wraps the SDK's `mcp.Client` and `mcp.ClientSession`. It supports two transport modes:

1. **Command (stdio)** -- spawns a subprocess via `mcp.CommandTransport` and communicates over stdin/stdout
2. **Streamable HTTP** -- connects to a remote MCP server via `mcp.StreamableClientTransport`

Both constructors share a common `newFromTransport` helper that creates the SDK client (with implementation name `"shelly"`, version `"0.1.0"`), connects to the transport, and returns an initialized `MCPClient`.

The key design choice is in `ListTools`: each returned `toolbox.Tool` has a `Handler` closure that calls back through `MCPClient.CallTool`, so MCP tools are seamlessly usable through the standard `ToolBox.Call` dispatch.

### Dependencies

- `pkg/tools/toolbox` -- for `Tool` type (the output of `ListTools`)
- `github.com/modelcontextprotocol/go-sdk/mcp` -- official MCP Go SDK

## Exported API

### Types

#### `MCPClient`

```go
type MCPClient struct {
    client  *mcp.Client        // unexported
    session *mcp.ClientSession // unexported
}
```

Communicates with a single MCP server instance.

### Functions

| Function / Method                                                          | Description                                                                                   |
|---------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------|
| `New(ctx context.Context, command string, args ...string) (*MCPClient, error)` | Spawns a server process and connects via stdio; SDK handles initialization automatically |
| `NewHTTP(ctx context.Context, url string) (*MCPClient, error)`            | Connects to a Streamable HTTP MCP server at the given URL                                     |
| `(*MCPClient) ListTools(ctx context.Context) ([]toolbox.Tool, error)`     | Fetches available tools; returns `toolbox.Tool` instances with handlers that call back through the client |
| `(*MCPClient) CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error)` | Calls a named tool on the server with JSON arguments |
| `(*MCPClient) Close() error`                                              | Terminates the session and releases resources (subprocess cleanup is handled by the SDK)      |

### Internal Helpers

| Function                                       | Description                                                        |
|------------------------------------------------|--------------------------------------------------------------------|
| `newFromTransport(ctx, transport) (*MCPClient, error)` | Shared constructor used by both `New` and `NewHTTP`         |
| `fromSDKTool(sdkTool, client) (toolbox.Tool, error)`   | Converts an SDK `*mcp.Tool` to a `toolbox.Tool` with a handler closure |
| `extractText(result) string`                   | Joins all `TextContent` items from a `CallToolResult` with newlines |

## Usage

### Command-based (stdio) transport

```go
// Connect to an MCP server process
client, err := mcpclient.New(ctx, "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")
if err != nil {
    return err
}
defer client.Close()

// Fetch tools and register them in a ToolBox
tools, err := client.ListTools(ctx)
if err != nil {
    return err
}
tb := toolbox.New()
tb.Register(tools...)

// Call MCP tools through the ToolBox like any other tool
tc := content.ToolCall{ID: "1", Name: "read_file", Arguments: `{"path":"/tmp/data.txt"}`}
result := tb.Call(ctx, tc)
```

### Streamable HTTP transport

```go
client, err := mcpclient.NewHTTP(ctx, "https://mcp.example.com/mcp?token=xxx")
if err != nil {
    return err
}
defer client.Close()

// Same API from here on
tools, err := client.ListTools(ctx)
```

### Direct tool call (without ToolBox)

```go
text, err := client.CallTool(ctx, "echo", json.RawMessage(`{"msg":"hello"}`))
```

## Subprocess Lifecycle

When using the command transport, `Close()` chains through the SDK: `session.Close()` -> `jsonrpc2.Connection.Close()` -> `ioConn.Close()` -> `pipeRWC.Close()`, which closes stdin, waits with a timeout, and escalates through SIGTERM/SIGKILL if the process does not exit.

## Testing

Tests use the SDK's `mcp.NewInMemoryTransports()` to create paired in-memory transports, avoiding real subprocess spawning. The `setupTestServer` helper creates a real SDK MCP server, connects a client via in-memory transport, and registers cleanup functions via `t.Cleanup`.
