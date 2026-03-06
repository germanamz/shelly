# Composition Layer: Skills and Engine

The composition layer brings all Shelly components together into a cohesive, configurable system:

- **`pkg/skill/`** — Folder-based skill loading with YAML frontmatter for procedural knowledge
- **`pkg/engine/`** — Composition root that wires everything from YAML config (Engine/Session/EventBus API)

---

## Skills (`pkg/skill/`)

### Skill Structure

Each skill lives in its own directory under a skills folder (e.g. `.shelly/skills/<name>/`). The only required file is `SKILL.md`—the entry point. Additional files (docs, scripts, templates) can be colocated in the folder.

`SKILL.md` supports an **optional YAML frontmatter** block (delimited by `---`) at the top:

```yaml
---
name: my-skill         # overrides directory name
description: Summary   # shown in skill listings
available_skills:      # skills this skill can load
  - other-skill
---

# Actual markdown content...
```

### Key Types (`skill.go`)

```go
type Skill struct {
    Name            string   // from frontmatter or directory name
    Description     string   // from frontmatter
    Content         string   // full markdown body (after frontmatter stripped)
    AvailableSkills []string // skill names this skill can reference
}
```

**Loading:** `Load(dir string) (Skill, error)` reads `<dir>/SKILL.md`, parses optional YAML frontmatter via `parseFrontmatter()`, and populates the struct. If `name` is empty in frontmatter, the directory base name is used. Returns error if SKILL.md is missing.

**`LoadAll(dirs ...string) ([]Skill, error)`** scans multiple directories, loading from each subdirectory that contains a SKILL.md. Skips subdirs without SKILL.md silently.

### Skill Store (`store.go`)

```go
type Store struct {
    skills  map[string]Skill
    workDir string
}
```

**`NewStore(skills []Skill, workDir string) *Store`** — Creates the store from loaded skills.

**Tool integration:** `Store.Tool() toolbox.Tool` returns a `load_skill` tool that agents can invoke at runtime to retrieve skill content by name. The tool:
- Accepts `{"name": "skill-name"}` as JSON input
- Returns the skill's full markdown content
- Lists available skills when a requested name is not found
- Includes an `available_skills` section listing all loaded skill names + descriptions in the tool description

**`Store.SkillsInfo() string`** — Returns a formatted listing of all available skills with names and descriptions.

**`Store.AvailableSkillNames(allowed []string) []string`** — Filters store skills to just those in the allowed list. Used to constrain which skills a specific agent can see.

---

## Engine (`pkg/engine/`)

The engine is the **composition root** — it wires all lower-level packages (providers, tools, agents, skills, state, tasks, sessions, project context) into a runtime. Frontends (CLI, TUI, web) interact only with `Engine` and `Session` types.

### File Layout

| File | Purpose |
|------|---------|
| `config.go` | Config structs, YAML loading, validation, env expansion |
| `provider.go` | Provider factory, builtin context windows, batch Completer |
| `effects.go` | Effect wiring (config → concrete Effect instances) |
| `engine.go` | Engine struct, New(), Close(), state/task/skill stores |
| `init.go` | Parallel init (skills, project context, MCP servers) |
| `session.go` | Interactive Session lifecycle |
| `batch_session.go` | Batch processing session (JSONL input/output) |
| `event.go` | EventBus, EventKind constants, typed Event struct |
| `registration.go` | Agent registration with registry (factory functions) |
| `toolbox_wiring.go` | Per-agent toolbox assembly from config |
| `mcp.go` | MCP server connection management |
| `doc.go` | Package documentation |

### Configuration (`config.go`)

**Top-level `Config`:**
```go
type Config struct {
    ShellyDir             string           `yaml:"-"`
    Providers             []ProviderConfig `yaml:"providers"`
    MCPServers            []MCPConfig      `yaml:"mcp_servers"`
    Agents                []AgentConfig    `yaml:"agents"`
    MaxTurns              int              `yaml:"max_turns"`
    DefaultSessionTimeout time.Duration    `yaml:"default_session_timeout"`
}
```

**`ProviderConfig`** — Name, type (`anthropic`/`openai`/`grok`/`gemini`), model, API key (env ref), max tokens, context window, temperature, thinking budget, extended thinking, batch options.

**`AgentConfig`** — Name, description, instructions, provider, icon/color display metadata, tools (include/exclude patterns), available skills, max turns, effects configuration, session timeout, MCP servers list.

**`MCPConfig`** — Name, type (`stdio`/`streamable_http`), command/args/env (stdio), URL/headers (HTTP).

**`EffectConfig`** — Name (e.g., `compaction`, `trimming`, `loop_detection`, `observation_masking`, `offloading`), priority, typed config as `map[string]any`.

**Loading:** `LoadConfig(path)` reads YAML and calls `cfg.Validate()`. `Validate()` checks for duplicate names, required fields, valid provider types, timeout parsing, effect config parsing, and sorts effects by priority. Environment variable expansion is done via `expandEnvValue()` (supports `$VAR` / `${VAR}` patterns and `env:VAR` format for provider API keys).

**Agent defaults:** `applyDefaults()` copies top-level `max_turns` and `default_session_timeout` into agents that don't set their own.

### Provider Factory (`provider.go`)

**`BuiltinContextWindows`** — A `map[string]int` mapping well-known model names to context window sizes (e.g., `claude-sonnet-4-20250514` → 200000, `gpt-4.1` → 1047576, `gemini-2.5-flash` → 1048576).

**`buildProviderCompleter(cfg ProviderConfig) (modeladapter.Completer, error)`** — Creates a provider-specific Completer based on `cfg.Type`:
- `anthropic` → `anthropic.New()` with optional thinking budget/extended thinking
- `openai` → `openai.New()`
- `grok` → `grok.New()`  
- `gemini` → `gemini.New()`

Wraps the Completer with `batch.NewCompleter()` if batch config is present (rate-limited request batching with configurable window/max-batch/max-tokens).

**Context window resolution:** Explicit config → builtin lookup → default 200,000.

**Completer caching:** `providerCompleters` map + `sync.Once` per provider prevents redundant construction. `getOrBuildCompleter(name)` handles thread-safe lazy initialization.

### Effects Wiring (`effects.go`)

**`EffectWiringContext`** — Provides engine resources to effect factories:
```go
type EffectWiringContext struct {
    ContextWindow int
    AgentName     string
    StorageDir    string
    Completer     modeladapter.Completer
}
```

**Registered effect factories** (in `effectFactories` map):

| Effect | Config Keys | Purpose |
|--------|-------------|---------|
| `compaction` | `summary_ratio`, `trigger_ratio`, `summary_model` | Summarizes old messages when token usage exceeds threshold |
| `trimming` | `keep_recent`, `trigger_ratio`, `strategy` (tail/sliding_window) | Drops old messages to stay within context window |
| `loop_detection` | `window`, `threshold`, `similarity` | Detects repetitive agent behavior |
| `observation_masking` | `keep_recent`, `mask_after_tokens` | Masks large tool outputs in older messages |
| `offloading` | `trigger_ratio`, `base_dir` | Offloads messages to disk when context grows too large |

Each factory parses its config from `map[string]any`, applies defaults, and returns an `agent.Effect`. Effect configs are parsed and validated during `Config.Validate()`.

### Engine Struct (`engine.go`)

```go
type Engine struct {
    cfg           Config
    registry      *agent.Registry
    completers    map[string]*completerEntry  // provider name → cached Completer
    skills        *skill.Store
    state         *state.Store
    taskBoard     *tasks.Board
    bus           *EventBus
    mcpClients    map[string]*mcpclient.Client
    sessionsStore *sessions.Store
    projectCtx    string
    agentConfigs  map[string]AgentConfig
    // ...sync primitives
}
```

**`New(ctx, cfg) (*Engine, error)`** — The main constructor:
1. Creates state store, task board, event bus, sessions store
2. Calls `parallelInit()` for concurrent initialization
3. Registers all agents with the registry
4. Returns wired Engine

**`parallelInit()`** (`init.go`) — Runs three operations concurrently via goroutines + `sync.WaitGroup`:
1. **Skills loading** — `skill.LoadAll()` from `.shelly/skills/` + configured skill dirs
2. **Project context** — `projectctx.Load()` from `.shelly/`
3. **MCP server connections** — Connects to all configured MCP servers in parallel (each with 15s timeout)

Errors are collected through a `sync.Mutex`-protected slice, joined with semicolons.

**`Close()`** — Shuts down MCP clients with 5-second timeout per client.

### Agent Registration (`registration.go`)

**`registerAgents(ctx)`** — Iterates over `cfg.Agents`, creating a factory function for each agent that:
1. Builds the agent's toolbox via `buildToolbox()` (includes coding tools, MCP tools, skills, task board tools)
2. Gets or builds the provider Completer
3. Wires effects from config
4. Constructs `agent.Config` with system prompt (instructions + project context + available skills info)
5. Returns `agent.New(agentCfg)`

The first agent in config is registered as the **main agent** (`registry.Register` with `agent.IsMain`).

### Toolbox Wiring (`toolbox_wiring.go`)

**`buildToolbox(ctx, agentCfg, askCh)`** — Assembles per-agent tool collection:
1. Starts with built-in coding tools: `filesystem`, `exec`, `search`, `git`, `http`, `notes`, `permissions`, `browser`, `defaults`, `ask` (if askCh provided)
2. Adds `skill.Store.Tool()` if skills available
3. Adds task board tools (`shared_tasks_*`) and state tools
4. Adds tools from configured MCP servers (filtered by agent's `mcp_servers` list)
5. Applies include/exclude patterns from agent config (glob matching on tool names)
6. Wraps with permission gating via `permissions.NewGatedToolbox()`

### Event System (`event.go`)

```go
type EventKind string  // e.g., "message_added", "tool_call_start", "agent_start"
```

**Event kinds:** `EventMessageAdded`, `EventToolCallStart`, `EventToolCallEnd`, `EventAgentStart`, `EventAgentEnd`, `EventUsageUpdate`, `EventStreamDelta`, `EventStreamEnd`, `EventThinking`, `EventPlan`, `EventSummaryLine`, `EventError`.

**`Event` struct** — Contains `Kind`, `AgentName`, `AgentIcon`, `AgentColor`, `ProviderLabel`, `Timestamp`, and a polymorphic `Data` field (message, tool call info, usage, error, etc.).

**`EventBus`** — Thread-safe pub/sub:
- `Subscribe(ch)` / `Unsubscribe(ch)` — register/remove listener channels
- `Publish(event)` — non-blocking send to all subscribers (drops events if channel full)

### Interactive Session (`session.go`)

```go
type Session struct {
    ID          string
    engine      *Engine
    agent       *agent.Agent
    chat        *chat.Chat
    cancel      context.CancelFunc
    askCh       chan ask.Request
    turnCount   int
    // ...sync primitives
}
```

**`Engine.NewSession(ctx, opts)`** — Creates session with random 8-byte hex ID. Options: `WithSessionPrompt`, `WithSessionID`, `WithSessionTimeout`, `WithSessionAttachments`. Builds the main agent via registry, subscribes an `EventNotifier` to the event bus, optionally loads persisted session.

**`Session.Send(ctx, text, attachments)`** — Appends user message to chat, runs `agent.Run()` in a goroutine, returns immediately for async processing. Persists chat after each run.

**`Session.Ask()`** — Returns the ask channel for interactive prompts from agent to user.

**`Session.Wait()`** — Blocks until agent completes current processing.

**`Session.Close()`** — Cancels context, persists chat state.

**Session persistence:** Uses `sessions.Store` to save/load chat history as JSON. Session file path: `<shellyDir>/local/sessions/<id>.json`.

### Batch Session (`batch_session.go`)

**`DefaultBatchConcurrency`** = 8

**`BatchTask`** — JSONL input format:
```go
type BatchTask struct {
    ID      string   `json:"id"`
    Prompt  string   `json:"prompt"`
    Agent   string   `json:"agent"`   // optional, defaults to main
    Timeout string   `json:"timeout"` // optional, per-task
}
```

**`BatchResult`** — JSONL output format with `id`, `status` (ok/error), `response`/`error`, `usage`, `duration`.

**`Engine.RunBatch(ctx, input, output, concurrency)`** — Reads tasks from JSONL reader, runs them concurrently (bounded by semaphore), writes results as JSONL to output writer. Each task:
1. Creates a fresh chat + agent via registry
2. Sends the prompt as a user message
3. Runs `agent.Run()` synchronously
4. Extracts the last assistant text as response
5. Writes `BatchResult` JSON line

### MCP Integration (`mcp.go`)

**`connectMCPServers(ctx, configs)`** — Connects to all configured MCP servers in parallel with 15s timeouts. Supports `stdio` and `streamable_http` transport types. Returns a map of connected `mcpclient.Client` instances.

**Permission gating:** MCP tool calls are wrapped in the same permission system as built-in tools.

---

## Key Patterns

1. **Lazy initialization** — Provider Completers are built on first use via `sync.Once`, not at engine startup
2. **Parallel init** — Skills, project context, and MCP connections load concurrently to reduce startup latency
3. **Factory registration** — Agents are registered as factory functions, not pre-built instances; each session/delegation creates a fresh agent
4. **Event-driven frontend** — The EventBus decouples engine internals from UI; frontends subscribe and react to typed events
5. **Config validation** — Comprehensive validation at load time (duplicate names, required fields, effect parsing, env expansion) prevents runtime errors
6. **Permission gating** — All tools (built-in and MCP) pass through `permissions.NewGatedToolbox()` before being given to agents
