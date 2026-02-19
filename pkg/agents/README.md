# agents

Agent orchestration for Shelly. The agents package provides the `Agent` type that combines a Provider, ToolBoxes, and a Chat into a single unit, and the `reactor` sub-package that implements the ReAct (Reason + Act) loop for autonomous tool-using agents.

## Architecture

```
agents/
├── agent/    Agent type — orchestrates Provider, ToolBoxes, and Chat
└── reactor/  ReAct loop — iterative completion and tool execution
```

**Dependency graph**: `agent` is the foundation, depending on `chatty`, `providers`, and `tools`. `reactor` depends on `agent` and drives the ReAct loop.

### `agent` — Agent

`Agent` orchestrates a `Provider`, one or more `ToolBox` instances, and a `Chat`:

| Field       | Type                 | Description                           |
|-------------|----------------------|---------------------------------------|
| `Name`      | `string`             | Agent identifier (used as Sender)     |
| `Provider`  | `provider.Provider`  | LLM completion provider               |
| `ToolBoxes` | `[]*toolbox.ToolBox` | Tool registries searched in order      |
| `Chat`      | `*chat.Chat`         | Conversation state                    |

- `New(name, provider, chat, ...toolboxes)` — creates an Agent
- `Complete(ctx) (Message, error)` — calls the provider, appends the reply to Chat, sets the Sender
- `CallTools(ctx, msg) []ToolResult` — executes all tool calls in the message, appends results to Chat
- `Tools() []Tool` — returns all tools from all ToolBoxes

`Agent` is **not** safe for concurrent use; callers must synchronize externally.

### `reactor` — ReAct Loop

Implements the Reason + Act pattern: the agent reasons (LLM completion), acts (tool execution), observes (tool results fed back), and repeats until the provider returns a final answer with no tool calls.

- `Run(ctx, agent, opts) (Message, error)` — drives the ReAct loop
- `Options{MaxIterations}` — limits the number of cycles (zero means no limit)
- `ErrMaxIterations` — returned when the limit is reached

## Examples

### Basic Agent Usage

```go
p := myProvider()
c := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("user", role.User, "Hello!"),
)

a := agent.New("bot", p, c)
reply, err := a.Complete(ctx)
fmt.Println(reply.TextContent())
```

### Agent with Tools

```go
tb := toolbox.New()
tb.Register(toolbox.Tool{
    Name:        "search",
    Description: "Searches the web",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
    Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
        // ... execute search ...
        return "results", nil
    },
})

a := agent.New("bot", p, c, tb)
reply, _ := a.Complete(ctx)
results := a.CallTools(ctx, reply)
```

### Multiple ToolBoxes

```go
localTools := toolbox.New()
localTools.Register(myLocalTool)

mcpTools := toolbox.New()
remoteTools, _ := mcpClient.ListTools(ctx)
mcpTools.Register(remoteTools...)

// Agent searches toolboxes in order
a := agent.New("bot", p, c, localTools, mcpTools)
```

### ReAct Loop

```go
a := agent.New("bot", p, c, tb)

// Run until the provider produces a final answer
reply, err := reactor.Run(ctx, a, reactor.Options{})
fmt.Println(reply.TextContent())
```

### ReAct Loop with Iteration Limit

```go
reply, err := reactor.Run(ctx, a, reactor.Options{MaxIterations: 10})
if errors.Is(err, reactor.ErrMaxIterations) {
    fmt.Println("Agent did not converge within 10 iterations")
}
```
