# agents

Agent orchestration for Shelly. The agents package defines the `Agent` interface and the embeddable `Base` struct. Concrete agent types embed `Base` to inherit shared functionality and implement the `Agent` interface for uniform usage.

## Architecture

```
agents/
├── agent.go     Agent interface + Base struct
├── doc.go       Package documentation
└── react/       ReAct loop — iterative completion and tool execution
```

### `Agent` interface

```go
type Agent interface {
    Run(ctx context.Context) (message.Message, error)
}
```

All agent types implement this interface, enabling polymorphic usage by upstream code.

### `Base` struct

`Base` provides shared functionality for agent types. Embed it in concrete agent structs to inherit `Complete`, `CallTools`, and `Tools` methods.

| Field          | Type                 | Description                           |
|----------------|----------------------|---------------------------------------|
| `Name`         | `string`             | Agent identifier (used as Sender)     |
| `ModelAdapter` | `modeladapter.Completer` | LLM completion adapter            |
| `ToolBoxes`    | `[]*toolbox.ToolBox` | Tool registries searched in order     |
| `Chat`         | `*chat.Chat`         | Conversation state                    |

- `NewBase(name, adapter, chat, ...toolboxes)` — creates a Base value
- `Complete(ctx) (Message, error)` — calls the adapter, appends the reply to Chat, sets the Sender
- `CallTools(ctx, msg) []ToolResult` — executes all tool calls in the message, appends results to Chat
- `Tools() []Tool` — returns all tools from all ToolBoxes

`Base` is **not** safe for concurrent use; callers must synchronize externally.

### `react` — ReAct Loop

Implements the Reason + Act pattern: the agent reasons (LLM completion), acts (tool execution), observes (tool results fed back), and repeats until the provider returns a final answer with no tool calls.

- `Agent` struct embedding `agents.Base`
- `New(base, opts) *Agent` — creates a ReAct agent
- `Run(ctx) (Message, error)` — drives the ReAct loop
- `Options{MaxIterations}` — limits the number of cycles (zero means no limit)
- `ErrMaxIterations` — returned when the limit is reached

## Examples

### Basic Agent Usage

```go
p := myAdapter()
c := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("user", role.User, "Hello!"),
)

base := agents.NewBase("bot", p, c)
// Use base.Complete(ctx) for single-shot completion
```

### ReAct Agent with Tools

```go
tb := toolbox.New()
tb.Register(toolbox.Tool{
    Name:        "search",
    Description: "Searches the web",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
    Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
        return "results", nil
    },
})

base := agents.NewBase("bot", p, c, tb)
a := react.New(base, react.Options{MaxIterations: 10})

reply, err := a.Run(ctx)
fmt.Println(reply.TextContent())
```

### ReAct Loop with Iteration Limit

```go
base := agents.NewBase("bot", p, c, tb)
a := react.New(base, react.Options{MaxIterations: 10})

reply, err := a.Run(ctx)
if errors.Is(err, react.ErrMaxIterations) {
    fmt.Println("Agent did not converge within 10 iterations")
}
```

### Polymorphic Usage

```go
func runAgent(ctx context.Context, a agents.Agent) {
    reply, err := a.Run(ctx)
    // ...
}

base := agents.NewBase("bot", p, c, tb)
a := react.New(base, react.Options{})
runAgent(ctx, a)
```
