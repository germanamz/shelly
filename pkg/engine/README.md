# engine

Framework composition root that assembles all Shelly components from configuration and exposes them through a frontend-agnostic API. Frontends (CLI, web, desktop) interact with `Engine` and `Session` types, observe activity through an `EventBus`, and never import lower-level packages directly.

## Architecture

```
engine/
├── config.go          Config structs + YAML loader
├── effects.go         Effect factory registry + buildEffects()
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
| `agent_start` | An agent starts processing (Data: `AgentEventData{Prefix}`) |
| `agent_end` | An agent finishes processing (Data: `AgentEventData{Prefix}` for sub-agents) |
| `ask_user` | An agent asks the user a question |
| `file_change` | A file is modified |
| `compaction` | Context window compaction occurred |
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
    context_window: 200000  # max context tokens (omit = provider default, 0 = no compaction)

mcp_servers:
  - name: web-search
    command: mcp-search
  - name: bright-data
    url: https://mcp.brightdata.com/mcp?token=${BRIGHTDATA_API_KEY}&groups=advanced_scraping

agents:
  - name: coder
    description: A coding expert
    instructions: You are a coding expert.
    provider: default
    prefix: "🦾"  # display prefix for TUI (default: "🤖")
    toolboxes: [filesystem, exec, search, git, http, state, tasks, notes]
    skills: [coder-workflow]  # per-agent skill filter (empty = all engine-level skills)
    effects:
      - kind: trim_tool_results
        params:
          max_result_length: 500  # trim old tool results to 500 chars
          preserve_recent: 4      # keep last 4 tool messages untrimmed
      - kind: compact
        params:
          threshold: 0.8  # compact at 80% of context window
    options:
      max_iterations: 100
  - name: planner
    description: A planning expert
    instructions: You are a planning expert.
    provider: default
    prefix: "📝"
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

# Override built-in context window defaults or add defaults for custom kinds.
# Built-in defaults: anthropic=200000, openai=128000, grok=131072.
default_context_windows:
  anthropic: 180000   # override the built-in anthropic default
  custom-llm: 64000   # add a default for a custom provider kind

filesystem:
  permissions_file: perms.yaml
git:
  work_dir: /path/to/repo
```

### Effects

Agents support pluggable **effects** — per-iteration hooks that run inside the ReAct loop. Effects are configured per-agent in YAML via the `effects:` list, or auto-generated from legacy options for backward compatibility.

When the effective context window is non-zero and no explicit effects are configured, the engine auto-generates both a `trim_tool_results` effect (lightweight, runs after each completion) and a `compact` effect with the agent's `context_threshold` (default 0.8). This graduated approach trims tool results first, then falls back to full summarisation only when needed.

Known provider kinds have built-in default context windows (anthropic: 200k, openai: 128k, grok: 131k). When `context_window` is omitted from the YAML, the default for the provider kind is used — meaning compaction works out of the box. Set `context_window: 0` explicitly to disable compaction.

Available effect kinds: `compact`, `trim_tool_results`. See `pkg/agent/effects/` for details.

### Agent Display Prefix

Each agent can have a configurable `prefix` (emoji + label) in its YAML config:

```yaml
agents:
  - name: planner
    prefix: "📝"    # renders as "📝 planner >" in the TUI
  - name: coder
    prefix: "🦾"    # renders as "🦾 coder >"
  - name: assistant  # omitted prefix defaults to "🤖"
```

The prefix is passed through `agent.Options.Prefix` and included in `EventAgentStart` events via `agent.AgentEventData{Prefix}`. Frontends read it from the event data to render agent output with the appropriate visual treatment.

### Sub-Agent Event Publishing

The engine wires an `EventNotifier` into every registered agent. When an agent delegates to or spawns child agents, the orchestration tools automatically publish `EventAgentStart` / `EventAgentEnd` events to the `EventBus`. This allows frontends to display sub-agent activity in real time (e.g., windowed containers in the TUI). The notifier is propagated recursively to children, so arbitrarily nested delegation chains are observable.

### Per-Agent Skills

Agents can declare a `skills` list to receive only a subset of the engine-level skills loaded from `.shelly/skills/`:

```yaml
agents:
  - name: orchestrator
    skills: [orchestrator-workflow]
  - name: planner
    skills: [planner-workflow]
  - name: coder
    skills: [coder-workflow]
  - name: assistant  # omitted = receives all engine-level skills
```

When `skills` is non-empty, the engine filters the loaded skills to only those matching the listed names. When empty or omitted, the agent receives all engine-level skills (backward compatible). This keeps each agent's system prompt focused on the workflow skills relevant to its role. Skills with descriptions are available on-demand via the `load_skill` tool; skills without descriptions are inlined into the system prompt.

The `dev-team` config template pre-assigns workflow skills to each agent (orchestrator-workflow, planner-workflow, coder-workflow). See `pkg/skill/README.md` for skill authoring details.

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
