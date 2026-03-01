# engine

Framework composition root that assembles all Shelly components from configuration and exposes them through a frontend-agnostic API. Frontends (CLI, web, desktop) interact with `Engine` and `Session` types, observe activity through an `EventBus`, and never import lower-level packages directly.

## Architecture

```
engine/
├── config.go          Config structs + YAML loader + validation
├── effects.go         Effect factory registry + buildEffects() + sort/priority
├── engine.go          Engine type: wiring, lifecycle, session management
├── session.go         Session type: user interaction, Send(), Respond(), chat access
├── event.go           EventBus, Event, EventKind, Subscription
├── provider.go        Provider factory registry (kind -> Completer), rate limiting, context window resolution
├── doc.go             Package documentation
├── engine_test.go     Engine + session integration tests
├── config_test.go     Config loading and validation tests
├── event_test.go      EventBus tests
├── provider_test.go   Context window resolution tests
```

### Engine

The composition root. Initializes the `.shelly/` directory (via `shellydir`), migrates legacy permissions, creates provider adapters (with optional rate limiting), connects MCP clients, loads skills from `.shelly/skills/`, loads project context (via `projectctx`), registers agent factories, and manages sessions. Skills loading, project context loading, and MCP connections run in parallel during startup to reduce latency.

```go
eng, err := engine.New(ctx, cfg)
defer eng.Close()

session, err := eng.NewSession("")  // uses entry agent
reply, err := session.Send(ctx, "Hello")
```

#### Engine Methods

| Method | Description |
|---|---|
| `New(ctx, cfg)` | Creates an Engine from config. Validates, wires all components, returns ready engine. |
| `Events()` | Returns the `*EventBus` for subscribing to engine events. |
| `State()` | Returns the shared `*state.Store`, or nil if no agent references the `state` toolbox. |
| `Tasks()` | Returns the shared `*tasks.Store`, or nil if no agent references the `tasks` toolbox. |
| `NewSession(agentName)` | Creates a new session. Empty name falls back to `EntryAgent`, then first agent. |
| `Session(id)` | Retrieves an existing session by ID. |
| `RemoveSession(id)` | Removes a session from the engine. Returns whether it existed. |
| `Close()` | Waits for in-flight sends to complete, cancels the engine context, closes browser and MCP clients. Returns the first error encountered. Idempotent via `sync.Once`. |

### Session

One interactive conversation. Owns a chat and agent instance.

```go
session.Send(ctx, "Hello")                           // text shorthand
session.SendParts(ctx, content.Text{Text: "Hello"})  // explicit parts
session.Chat()                                        // direct chat access
session.Completer()                                   // underlying completer for usage reporting
session.Respond(questionID, "yes")                    // answer a pending ask_user question
session.AgentName()                                   // name of the session's agent
```

Only one `Send` may be active per session at a time. Concurrent `Send` calls return an error.

#### Session Methods

| Method | Description |
|---|---|
| `ID()` | Returns the session identifier. |
| `AgentName()` | Returns the name of the session's agent. |
| `Send(ctx, text)` | Appends a text user message and runs the agent's ReAct loop. Returns the agent's reply. |
| `SendParts(ctx, ...parts)` | Like `Send` but accepts explicit `content.Part` values. |
| `Chat()` | Returns the underlying `*chat.Chat` for direct observation. |
| `Completer()` | Returns the session's `modeladapter.Completer` for usage reporting. |
| `Respond(questionID, response)` | Delivers a user response to a pending `ask_user` question. |

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
| `ask_user` | An agent asks the user a question (Data: `ask.Question`) |
| `file_change` | A file is modified (Data: string message) |
| `compaction` | Context window compaction occurred (Data: string message) |
| `error` | An error occurs (Data: `error`) |

Non-blocking publish: slow subscribers drop events instead of stalling the agent loop.

#### EventBus Methods

| Method | Description |
|---|---|
| `NewEventBus()` | Creates a new EventBus. |
| `Subscribe(bufSize)` | Creates a subscription with the given channel buffer size. Returns `*Subscription`. |
| `Unsubscribe(sub)` | Removes the subscription and closes its channel. Safe to call twice. |
| `Publish(e)` | Sends an event to all subscribers. Non-blocking; drops events for full subscriber buffers. |

### Configuration

YAML-based with validation. Environment variables referenced as `${VAR}` or `$VAR` are expanded before parsing, allowing secrets to live in environment variables or a `.env` file.

```yaml
providers:
  - name: default
    kind: anthropic
    api_key: ${ANTHROPIC_API_KEY}
    model: claude-sonnet-4-20250514
    context_window: 200000  # max context tokens (omit = provider default, 0 = no compaction)
    rate_limit:
      input_tpm: 100000     # input tokens per minute (0 = no limit)
      output_tpm: 50000     # output tokens per minute (0 = no limit)
      rpm: 60               # requests per minute (0 = no limit)
      max_retries: 3        # max retries on 429
      base_delay: "1s"      # initial backoff delay

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
    toolboxes:
      - filesystem
      - exec
      - search
      - name: git                      # object form: only expose specific tools
        tools: [git_status, git_diff]
      - http
      - browser
      - state
      - tasks
      - notes
    skills: [coder-workflow]  # per-agent skill filter (empty = all engine-level skills)
    effects:
      - kind: trim_tool_results
        params:
          max_result_length: 500  # trim old tool results to 500 chars
          preserve_recent: 4      # keep last 4 tool messages untrimmed
      - kind: compact
        params:
          threshold: 0.8  # compact at 80% of context window
      - kind: loop_detect
        params:
          threshold: 3    # number of repeated tool calls before injecting warning
          window_size: 6  # sliding window size
      - kind: sliding_window
        params:
          threshold: 0.7  # trigger at 70% of context window
          recent_zone: 8  # messages in recent zone (untrimmed)
          medium_zone: 8  # messages in medium zone (partially trimmed)
          trim_length: 200
      - kind: observation_mask
        params:
          threshold: 0.6
          recent_window: 4
      - kind: reflection
        params:
          failure_threshold: 3
      - kind: progress
        params:
          interval: 5
    options:
      max_iterations: 100
      max_delegation_depth: 5
      context_threshold: 0.8  # legacy shorthand for compact threshold
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
# Built-in defaults: anthropic=200000, openai=128000, grok=131072, gemini=1048576.
default_context_windows:
  anthropic: 180000   # override the built-in anthropic default
  custom-llm: 64000   # add a default for a custom provider kind

filesystem:
  permissions_file: perms.yaml
git:
  work_dir: /path/to/repo
browser:
  headless: true
```

#### Config Types

| Type | Description |
|---|---|
| `Config` | Top-level engine configuration. Contains providers, MCP servers, agents, entry agent, filesystem/git/browser settings, default context windows, and an optional `StatusFunc` callback for progress messages during initialization. `ShellyDir` is set by the CLI (not from YAML). |
| `ProviderConfig` | Describes an LLM provider instance: name, kind, base URL, API key, model, optional context window (`*int`: nil = use default, 0 = disable compaction), and rate limit settings. |
| `RateLimitConfig` | Per-provider rate limiting: `InputTPM`, `OutputTPM`, `RPM`, `MaxRetries`, and `BaseDelay` (duration string). When any field is non-zero, the completer is wrapped with `modeladapter.NewRateLimitedCompleter`. |
| `MCPConfig` | Describes an MCP server: name, command + args (stdio transport) or URL (SSE transport). Command and URL are mutually exclusive. |
| `ToolboxRef` | References a toolbox by name with an optional `Tools` whitelist. Supports both plain string ("filesystem") and object form (`{name: git, tools: [git_status]}`) in YAML. |
| `AgentConfig` | Agent registration: name, description, instructions, provider reference, toolbox list (`[]ToolboxRef`), skills filter, effects list, options, and display prefix. |
| `AgentOptions` | Optional agent behaviour: `MaxIterations`, `MaxDelegationDepth`, `ContextThreshold` (fraction in (0, 1) or 0 to disable). |
| `EffectConfig` | A single effect: `Kind` string and `Params` map. |
| `FilesystemConfig` | Filesystem tool settings (permissions file path). |
| `GitConfig` | Git tool settings (working directory). |
| `BrowserConfig` | Browser tool settings (`Headless` bool). |

#### Config Functions

| Function | Description |
|---|---|
| `LoadConfig(path)` | Reads a YAML file, expands `${VAR}` environment variables, and returns a `Config`. |
| `LoadConfigRaw(path)` | Reads a YAML file without expanding environment variables. Preserves `${VAR}` references, useful for config editing round-trips. |
| `Config.Validate()` | Validates internal consistency: requires at least one provider and one agent, checks for duplicate names, verifies provider/toolbox/entry agent references, validates context window and threshold ranges, and validates effect kinds. |
| `KnownProviderKinds()` | Returns the sorted list of registered provider kind strings. |
| `KnownEffectKinds()` | Returns the sorted list of recognised effect kind strings. |
| `BuiltinToolboxNames()` | Returns the sorted list of built-in toolbox names. |
| `ToolboxRefNames(refs)` | Extracts the `Name` field from each `ToolboxRef`. |
| `ToolboxRefsFromNames(names)` | Creates plain `ToolboxRef` values (no tools filter) from a list of names. |

### Effects

Agents support pluggable **effects** -- per-iteration hooks that run inside the ReAct loop. Effects are configured per-agent in YAML via the `effects:` list, or auto-generated from legacy options for backward compatibility.

When the effective context window is non-zero and no explicit effects are configured, the engine auto-generates both a `trim_tool_results` effect (lightweight, runs after each completion) and a `compact` effect with the agent's `context_threshold` (default 0.8). This graduated approach trims tool results first, then falls back to full summarisation only when needed.

Known provider kinds have built-in default context windows (anthropic: 200k, openai: 128k, grok: 131k, gemini: 1M). When `context_window` is omitted from the YAML, the default for the provider kind is used -- meaning compaction works out of the box. Set `context_window: 0` explicitly to disable compaction.

Effects are sorted by priority before execution: compaction-class effects (`compact`, `sliding_window`) run first so that effects injecting messages (e.g., `reflection`, `loop_detect`) are not immediately summarized away in the same iteration.

Available effect kinds:

| Kind | Description | Key Params |
|---|---|---|
| `compact` | Full context summarisation when threshold is reached. | `threshold` (default 0.8) |
| `trim_tool_results` | Trims old tool results to save tokens. | `max_result_length`, `preserve_recent` |
| `loop_detect` | Detects repeated tool calls and injects a warning. | `threshold`, `window_size` |
| `sliding_window` | Tiered message trimming with recent/medium zones. | `threshold` (default 0.7), `recent_zone`, `medium_zone`, `trim_length` |
| `observation_mask` | Masks observations beyond a context threshold. | `threshold`, `recent_window` |
| `reflection` | Injects reflection prompts after repeated failures. | `failure_threshold` |
| `progress` | Periodic progress checkpoint. | `interval` |

See `pkg/agent/effects/` for implementation details.

#### Effect Extension Points

| Type | Description |
|---|---|
| `EffectWiringContext` | Provides engine-level resources to effect factories: `ContextWindow`, `AgentName`, `AskFunc`, `NotifyFunc`. |
| `EffectFactory` | Function type `func(params map[string]any, wctx EffectWiringContext) (agent.Effect, error)`. Maps YAML params to a concrete `agent.Effect`. |

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

Additionally, an `EventFunc` is wired into each agent to publish fine-grained loop events (`tool_call_start`, `tool_call_end`, `message_added`) during the ReAct loop.

### Agent Reflections

When the `.shelly/` directory exists, the engine resolves a reflections directory (via `shellydir.Dir.ReflectionsDir()`) and passes it to each agent via `agent.Options.ReflectionDir`. This enables agents to persist and retrieve reflection data across sessions.

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

The `dev-team` config template pre-assigns workflow skills to each agent (lead-workflow, explorer-workflow, planner-workflow, coder-workflow, reviewer-workflow). See `pkg/skill/README.md` for skill authoring details.

### Toolbox Assignment and Inheritance

Each agent's `toolboxes` list in YAML is resolved at config load time. Toolbox entries can be plain strings (all tools) or objects with a `tools` whitelist to expose only specific tools:

```yaml
toolboxes:
  - filesystem                      # plain string = all tools
  - name: git
    tools: [git_status, git_diff]   # only expose specific tools
  - exec
```

The engine maps toolbox names to `ToolBox` instances (built-in ones like `filesystem`, `exec`, `search`, `git`, `http`, `browser`, `state`, `tasks`, `notes`, plus any MCP server toolboxes), applies any per-agent tool whitelist via `ToolBox.Filter`, and captures them in the agent's factory closure. The `ask` toolbox is always implicitly included. This means the toolboxes an agent is created with are fixed at startup.

Built-in toolbox names: `ask` (always included), `filesystem`, `exec`, `search`, `git`, `http`, `browser`, `state`, `tasks`, `notes`.

However, at delegation time the parent agent appends its own toolboxes to the child (see `pkg/agent` README for details). This means a child agent effectively gets a **union** of its configured toolboxes and the parent's toolboxes, with the child's own tools taking precedence on name collisions.

When designing agent configs, keep in mind:
- An agent's YAML `toolboxes` defines its **minimum** tool set.
- Delegation from a more-privileged parent will grant the child additional tools at runtime.
- To restrict a child's tools strictly to its config, avoid delegating from agents with broader toolbox sets, or adjust the delegation logic.
- Built-in toolboxes that require filesystem permissions (`filesystem`, `exec`, `search`, `git`, `http`, `browser`) share a single `permissions.Store` instance and an `ask.Responder` for user prompts.

### Task Board Adapter

When the `tasks` toolbox is referenced by at least one agent, the engine creates a `*tasks.Store` and wires a `taskBoardAdapter` into each agent's options. This adapter implements `agent.TaskBoard` by delegating `ClaimTask` and `UpdateTaskStatus` calls to the shared task store, enabling agents to coordinate work through a shared task board.

### Provider Factory

Maps provider `kind` strings to factory functions. Built-in: `anthropic`, `openai`, `grok`, `gemini`. Extensible via `RegisterProvider`.

```go
engine.RegisterProvider("custom", func(cfg engine.ProviderConfig) (modeladapter.Completer, error) {
    return myProvider(cfg), nil
})
```

#### Provider Types and Functions

| Symbol | Description |
|---|---|
| `ProviderFactory` | Function type `func(cfg ProviderConfig) (modeladapter.Completer, error)`. |
| `RegisterProvider(kind, factory)` | Registers a custom provider factory. Can be called before `New`. |
| `BuiltinContextWindows` | Exported `map[string]int` of default context windows per provider kind: `anthropic: 200000`, `openai: 128000`, `grok: 131072`, `gemini: 1048576`. |

Context window resolution order: explicit `context_window` in provider config > `default_context_windows` map in config > `BuiltinContextWindows` built-in defaults > 0 (disabled).

## Frontend Integration

No `Frontend` interface. Frontends compose from:

- `session.Send()` / `session.SendParts()` -- synchronous request/response
- `session.Chat()` -- chat history for direct observation
- `session.Respond()` -- answer pending `ask_user` questions
- `session.Completer()` -- access underlying completer for usage stats
- `engine.Events().Subscribe()` -- reactive event stream

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
// POST /sessions/{id}/respond -> session.Respond(qID, response)
// GET  /sessions/{id}/events  -> SSE from eng.Events().Subscribe()
// DELETE /sessions/{id}       -> eng.RemoveSession(id)
```

## Dependencies

- `pkg/agent` -- agent types, registry, effects interface, event notifier
- `pkg/agent/effects` -- concrete effect implementations
- `pkg/agentctx` -- context key helpers for agent identity
- `pkg/chats` -- chat, message, content, role types
- `pkg/codingtoolbox/ask` -- ask responder for user prompts
- `pkg/codingtoolbox/browser` -- browser automation tools (Playwright-based)
- `pkg/codingtoolbox/exec` -- command execution tools
- `pkg/codingtoolbox/filesystem` -- filesystem tools, session trust
- `pkg/codingtoolbox/git` -- git tools
- `pkg/codingtoolbox/http` -- HTTP request tools
- `pkg/codingtoolbox/notes` -- persistent notes tools
- `pkg/codingtoolbox/permissions` -- shared permission store
- `pkg/codingtoolbox/search` -- search tools
- `pkg/modeladapter` -- Completer interface, rate-limited completer wrapper
- `pkg/projectctx` -- project context loading
- `pkg/providers/anthropic`, `pkg/providers/openai`, `pkg/providers/grok`, `pkg/providers/gemini` -- LLM providers
- `pkg/shellydir` -- `.shelly/` directory path resolution and bootstrapping
- `pkg/skill` -- skill loading and store
- `pkg/state` -- key-value state store
- `pkg/tasks` -- shared task board for multi-agent coordination
- `pkg/tools/mcpclient` -- MCP client connections (stdio and HTTP/SSE)
- `pkg/tools/toolbox` -- tool and toolbox types
