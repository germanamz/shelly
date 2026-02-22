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

The composition root. Creates provider adapters, connects MCP clients, loads skills, registers agent factories, and manages sessions.

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
  - name: search
    command: mcp-search

agents:
  - name: assistant
    description: A helpful assistant
    provider: default
    toolbox_names: [search, state]
    options:
      max_iterations: 10

entry_agent: assistant
state_enabled: true
tasks_enabled: true

filesystem:
  enabled: true
  permissions_file: .shelly/permissions.json
exec:
  enabled: true
search:
  enabled: true
git:
  enabled: true
  work_dir: /path/to/repo
http:
  enabled: true
```

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
