# mcpserver

MCP (Model Context Protocol) server for Shelly. Exposes `toolbox.Tool` instances over the MCP protocol so that external MCP clients can discover and call them. Built as a thin wrapper around the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

## Architecture

`MCPServer` wraps the SDK's `mcp.Server`. Tools are registered via `Register`, which converts each `toolbox.Tool` into SDK tool registrations using two internal helpers:

- `toSDKTool` -- converts a `toolbox.Tool` struct to an `*mcp.Tool` (name, description, input schema)
- `toSDKHandler` -- wraps a `toolbox.Handler` as an SDK `mcp.ToolHandler`, mapping handler errors to MCP error responses (`IsError: true` with the error message as `TextContent`)

The `Serve` method accepts an `io.Reader` and `io.Writer` (typically `os.Stdin` and `os.Stdout`), wraps them in an `mcp.IOTransport`, and runs the server until the context is cancelled or the transport closes. A `nopWriteCloser` adapter is used internally to satisfy the `io.WriteCloser` interface required by the SDK transport.

### Dependencies

- `pkg/tools/toolbox` -- for `Tool` and `Handler` types
- `github.com/modelcontextprotocol/go-sdk/mcp` -- official MCP Go SDK

## Exported API

### Types

#### `MCPServer`

```go
type MCPServer struct {
    server *mcp.Server // unexported
}
```

Serves tools over the MCP protocol.

### Functions

| Function / Method                                                | Description                                                                  |
|------------------------------------------------------------------|------------------------------------------------------------------------------|
| `New(name, version string) *MCPServer`                           | Creates a new server with the given implementation name and version          |
| `(*MCPServer) Register(tools ...toolbox.Tool)`                   | Adds one or more tools to the server                                         |
| `(*MCPServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error` | Starts serving MCP requests; blocks until ctx is cancelled or transport closes |

### Internal Helpers

| Function / Type                          | Description                                                         |
|------------------------------------------|---------------------------------------------------------------------|
| `toSDKTool(t toolbox.Tool) *mcp.Tool`    | Converts a `toolbox.Tool` to an SDK `*mcp.Tool`                    |
| `toSDKHandler(h Handler) mcp.ToolHandler` | Wraps a `toolbox.Handler` as an SDK tool handler                   |
| `nopWriteCloser`                         | Adapter that wraps `io.Writer` as `io.WriteCloser` with a no-op `Close` |

## Handler Error Mapping

When a `toolbox.Handler` returns an error, `toSDKHandler` does not propagate it as a Go error to the SDK. Instead, it returns a successful `*mcp.CallToolResult` with `IsError: true` and the error message as `TextContent`. This follows the MCP convention where tool-level errors are reported in the result payload, not as protocol-level errors.

## Usage

### Serving over stdin/stdout

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

// Blocks until stdin is closed or context is cancelled
err := server.Serve(context.Background(), os.Stdin, os.Stdout)
```

### Registering multiple tools

```go
server := mcpserver.New("my-tools", "2.0.0")
server.Register(toolA, toolB, toolC)
err := server.Serve(ctx, os.Stdin, os.Stdout)
```

## Testing

Tests use the SDK's `mcp.NewInMemoryTransports()` to create paired in-memory transports. The `setupTestClient` helper creates an `MCPServer`, connects an SDK client via in-memory transport, and runs the server in a background goroutine. Tests verify tool listing, successful calls, handler errors, unknown tool calls, and context cancellation.
