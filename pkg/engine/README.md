# engine

Framework composition root that assembles all Shelly components from configuration and exposes them through a frontend-agnostic API. Frontends (CLI, web, desktop) interact with `Engine` and `Session` types, observe activity through an `EventBus`, and never import lower-level packages directly.

## Architecture

```
engine/
├── config.go          Config structs + YAML loader
├── engine.go          Engine type: wiring, lifecycle, session management
├── session.go         Session type: user interaction, Send(), chat access
├── event.go           EventBus, Event, EventKind, Subscription
├── provider.go        Provider factory registry (kind -> Completer)
├── doc.go             Package documentation
├── engine_test.go     Engine + session integration tests
├── config_test.go     Config loading and validation tests
├── event_test.go      EventBus tests
```

### Engine

The composition root. Initializes the `.shelly/` directory (via `shellydir`), migrates legacy permissions, creates provider adapters, connects MCP clients, loads skills from `.shelly/skills/`, loads project context (via `projectctx`), registers agent factories, and manages sessions.

```go
eng, err := engine.New(ctx, cfg)
defer eng.Close()

session, err := eng.NewSession("")  // uses entry agent
reply, err := session.Send(ctx, "Hello")
```

### Session

One interactive conversation. Owns a chat and agent instance.

```go
session.Send(ctx, "Hello")                           // text shorthand
session.SendParts(ctx, content.Text{Text: "Hello"})  // explicit parts
session.Chat()                                        // direct chat access
```

Only one `Send` may be active per session at a time.

### EventBus

Channel-based push model for observing engine activity.

```go
sub := eng.Events().Subscribe(64)
defer eng.Events().Unsubscribe(sub)

for e := range sub.C {
    fmt.Println(e.Kind, e.Agent)
}
```

| EventKind | When |
|---|---|
| `message_added` | A message is appended to a chat |
| `tool_call_start` | A tool call begins |
| `tool_call_end` | A tool call completes |
| `agent_start` | An agent starts processing |
| `agent_end` | An agent finishes processing |
| `error` | An error occurs |

Non-blocking publish: slow subscribers drop events instead of stalling the agent loop.

### Configuration

YAML-based with validation:

```yaml
providers:
  - name: default
    kind: anthropic
    api_key: sk-xxx
    model: claude-sonnet-4-20250514

mcp_servers:
  - name: web-search
    command: mcp-search

agents:
  - name: coder
    description: A coding expert
    instructions: You are a coding expert.
    provider: default
    toolboxes: [filesystem, exec, search, git, http, state, tasks]
    options:
      max_iterations: 100
  - name: planner
    description: A planning expert
    instructions: You are a planning expert.
    provider: default
    toolboxes: [filesystem, search, state]
    options:
      max_iterations: 50
  - name: assistant
    description: A helpful assistant
    instructions: Be helpful and concise.
    provider: default
    toolboxes: [filesystem, exec, search, git, http, state, tasks, web-search]
    options:
      max_iterations: 10

entry_agent: assistant

filesystem:
  permissions_file: perms.yaml
git:
  work_dir: /path/to/repo
```

### Toolbox Assignment and Inheritance

Each agent's `toolboxes` list in YAML is resolved at config load time. The engine maps toolbox names to `ToolBox` instances (built-in ones like `filesystem`, `exec`, `search`, `git`, `http`, `state`, `tasks`, plus any MCP server toolboxes) and captures them in the agent's factory closure. This means the toolboxes an agent is created with are fixed at startup.

However, at delegation time the parent agent appends its own toolboxes to the child (see `pkg/agent` README for details). This means a child agent effectively gets a **union** of its configured toolboxes and the parent's toolboxes, with the child's own tools taking precedence on name collisions.

When designing agent configs, keep in mind:
- An agent's YAML `toolboxes` defines its **minimum** tool set.
- Delegation from a more-privileged parent will grant the child additional tools at runtime.
- To restrict a child's tools strictly to its config, avoid delegating from agents with broader toolbox sets, or adjust the delegation logic.

### Provider Factory

Maps provider `kind` strings to factory functions. Built-in: `anthropic`, `openai`, `grok`. Extensible via `RegisterProvider`.

```go
engine.RegisterProvider("custom", func(cfg engine.ProviderConfig) (modeladapter.Completer, error) {
    return myProvider(cfg), nil
})
```

## Frontend Integration

No `Frontend` interface. Frontends compose from:

- `session.Send()` — synchronous request/response
- `session.Chat()` — chat history + `Wait()` for async observation
- `engine.Events().Subscribe()` — reactive event stream

### CLI

```go
eng, _ := engine.New(ctx, cfg)
sub := eng.Events().Subscribe(64)
go renderEvents(sub)
session, _ := eng.NewSession("")
reply, _ := session.Send(ctx, input)
```

### Web

```go
// POST /sessions       -> eng.NewSession()
// POST /sessions/{id}  -> session.Send()
// GET  /sessions/{id}/events -> SSE from eng.Events().Subscribe()
```
