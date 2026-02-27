# toolbox

Core tool primitives for Shelly. Defines the `Tool` type representing an executable tool and `ToolBox` which orchestrates a flat collection of tools for registration, retrieval, and execution. This is the foundation layer that all other tool-related packages depend on.

## Architecture

The package has two files:

- `tool.go` -- defines the `Handler` function type and the `Tool` struct
- `toolbox.go` -- defines the `ToolBox` orchestrator that manages a `map[string]Tool`

`ToolBox` maintains a flat, name-keyed map of tools with no parent-child or hierarchical relationships. Toolbox inheritance during agent delegation is handled by the agent layer (`pkg/agent`), not here.

### Dependencies

- `pkg/chats/content` -- for `ToolCall` and `ToolResult` types used by `ToolBox.Call`

## Exported API

### Types

#### `Handler`

```go
type Handler func(ctx context.Context, input json.RawMessage) (string, error)
```

Function signature for tool execution. Receives a context and raw JSON input, returns a text result or an error.

#### `Tool`

```go
type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage
    Handler     Handler
}
```

| Field         | Type              | Description                              |
|---------------|-------------------|------------------------------------------|
| `Name`        | `string`          | Unique tool identifier                   |
| `Description` | `string`          | Human-readable description for the LLM   |
| `InputSchema` | `json.RawMessage` | JSON Schema defining the tool's input    |
| `Handler`     | `Handler`         | Function that executes the tool          |

#### `ToolBox`

Orchestrates a collection of tools. Agents use `ToolBox` to register tools and dispatch `ToolCall` requests from the LLM.

### Functions

| Function / Method                            | Description                                                                 |
|----------------------------------------------|-----------------------------------------------------------------------------|
| `New() *ToolBox`                             | Creates a new empty `ToolBox`                                               |
| `(*ToolBox) Register(tools ...Tool)`         | Adds one or more tools; replaces existing tools with the same name          |
| `(*ToolBox) Get(name string) (Tool, bool)`   | Retrieves a tool by name; returns false if not found                        |
| `(*ToolBox) Merge(other *ToolBox)`           | Copies all tools from another `ToolBox` into this one; replaces by name     |
| `(*ToolBox) Tools() []Tool`                  | Returns all registered tools as a slice                                     |
| `(*ToolBox) Call(ctx, tc ToolCall) ToolResult` | Executes a tool call; returns a `ToolResult` with `IsError` on failure    |

### Error Handling in `Call`

`Call` never returns a Go error. Instead, it returns a `content.ToolResult` with `IsError: true` in two cases:

1. **Tool not found** -- `Content` is set to `"tool not found: <name>"`
2. **Handler error** -- `Content` is set to the error message from the handler

This design allows the agent loop to always send a result back to the LLM, which can then decide how to recover.

## Usage

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

tc := content.ToolCall{ID: "1", Name: "greet", Arguments: `{"name":"World"}`}
result := tb.Call(context.Background(), tc)
// result.Content == "Hello, World!"
// result.IsError == false
```

### Merging ToolBoxes

```go
tb1 := toolbox.New()
tb1.Register(toolA, toolB)

tb2 := toolbox.New()
tb2.Register(toolC)

tb1.Merge(tb2) // tb1 now contains toolA, toolB, and toolC
```

## Consumers

This package is imported widely across Shelly:

- `pkg/modeladapter` -- uses `Tool` for the `ToolAware` interface (declaring tools to LLM providers)
- `pkg/providers/*` -- receive `[]toolbox.Tool` via `SetTools()`
- `pkg/agent` -- owns a `ToolBox` for dispatching tool calls in the ReAct loop
- `pkg/codingtoolbox/*` -- each sub-package builds `toolbox.Tool` instances for built-in coding tools
- `pkg/skill` -- exposes loaded skills as `[]toolbox.Tool`
- `pkg/tools/mcpclient` -- converts MCP server tools into `toolbox.Tool` instances
- `pkg/tools/mcpserver` -- converts `toolbox.Tool` instances into MCP server tool registrations
- `pkg/engine` -- composition root that wires ToolBoxes together
