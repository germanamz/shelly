# Shelly — Feature Specification

## 1. Overview

Shelly is a provider-agnostic, multi-agent orchestration framework written in Go (1.25+). It provides a terminal-based interactive interface for building sophisticated LLM chat applications with support for multiple providers, tool execution, multi-agent delegation, context management, and a rich TUI experience.

**Module:** `github.com/germanamz/shelly`

---

## 2. LLM Provider Support

### 2.1 Provider-Agnostic Architecture

All LLM interaction flows through the `Completer` interface, decoupling the framework from any specific provider API:

```go
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)
}
```

Each provider translates between Shelly's internal chat/message types and the provider's wire format.

### 2.2 Supported Providers

| Provider | Kind | Default Context Window | Auth Scheme | Notes |
|----------|------|----------------------|-------------|-------|
| Anthropic | `anthropic` | 200,000 | `x-api-key` header | System prompt as top-level `system` field; `anthropic-version: 2023-06-01` header |
| OpenAI | `openai` | 128,000 | Bearer token | System prompt as `"system"` role message; tool results as separate `"tool"` role messages |
| Grok (xAI) | `grok` | 131,000 | Bearer token | OpenAI-compatible wire format; default base URL `https://api.x.ai` |
| Gemini (Google) | `gemini` | 1,000,000 | `x-goog-api-key` header | Strict user/model role alternation; `thoughtSignature` round-tripping via metadata |

### 2.3 Provider Configuration

Each provider is configured in the YAML config:

```yaml
providers:
  - name: default
    kind: anthropic
    api_key: ${ANTHROPIC_API_KEY}
    model: claude-sonnet-4-20250514
    base_url: https://api.anthropic.com  # optional override
    context_window: 200000               # optional (nil = provider default, 0 = no compaction)
    rate_limit:
      input_tpm: 100000
      output_tpm: 50000
      rpm: 60
      max_retries: 3
      base_delay: "1s"
```

Environment variable expansion (`${VAR}` or `$VAR`) is supported in all config values.

### 2.4 Custom Provider Registration

New provider kinds can be registered before engine initialization via `RegisterProvider(kind, factory)`. This allows extending Shelly with proprietary or self-hosted LLM APIs.

### 2.5 Default Context Window Overrides

Built-in defaults can be overridden at the config level:

```yaml
default_context_windows:
  anthropic: 180000
  custom-llm: 64000
```

Resolution order: explicit per-provider `context_window` → `default_context_windows` map → built-in defaults → 0 (disabled).

---

## 3. Rate Limiting

### 3.1 Proactive Throttling

A `RateLimitedCompleter` wraps any `Completer` and enforces rate limits using a sliding 1-minute window:

- **TPM (Tokens Per Minute):** Tracks input and output tokens separately.
- **RPM (Requests Per Minute):** Counts API calls.
- Waits until capacity is available before issuing calls.
- Calls are serialized for accurate token diff computation.

### 3.2 Reactive Retry

On HTTP 429 responses:

- Exponential backoff: `2^attempt × baseDelay` with ±25% jitter.
- Respects `Retry-After` header when provided.
- Adapts from server rate-limit info headers (both Anthropic and OpenAI header formats supported).
- Configurable `max_retries` (default 3).

### 3.3 Adaptive Throttling

When capacity nears zero, the completer sleeps until the server-reported reset time rather than polling.

---

## 4. Chat Data Model

### 4.1 Roles

Four conversation roles: `System`, `User`, `Assistant`, `Tool`.

### 4.2 Content Parts

Messages contain one or more typed content parts:

| Part | Purpose |
|------|---------|
| `Text` | Plain text content |
| `Image` | URL or embedded bytes with media type |
| `ToolCall` | Assistant's tool invocation (ID, name, JSON arguments, optional metadata) |
| `ToolResult` | Tool output with error flag |

### 4.3 Messages

Each message combines:
- **Sender**: Agent identity string (enables multi-agent tracking via `BySender`)
- **Role**: One of the four conversation roles
- **Parts**: Ordered list of content parts
- **Metadata**: Arbitrary key-value map

### 4.4 Chat Container

Thread-safe mutable message container (`sync.RWMutex`) supporting:
- Append and Replace operations
- Signal-based `Wait()` / `Since()` for streaming observers
- `BySender()` filtering for multi-agent tracking
- `SystemPrompt()` extraction, `Last()`, `Each()` iteration
- Deep-copy semantics on read operations to ensure immutable read patterns

---

## 5. Agent System

### 5.1 ReAct Loop

Each agent runs a ReAct (Reason + Act) loop:

1. Evaluate effects at `PhaseBeforeComplete` (context management)
2. Call `completer.Complete()` with current chat and tools
3. Append reply to chat; emit `message_added` event
4. Evaluate effects at `PhaseAfterComplete` (lightweight cleanup)
5. If no tool calls in reply → return as final answer
6. Execute all tool calls **concurrently** (via `sync.WaitGroup`)
7. Append tool results to chat
8. If `task_complete` was called → return immediately
9. Loop until final answer or iteration limit

### 5.2 Agent Configuration

```yaml
agents:
  - name: coder
    description: A coding expert
    instructions: You are a coding expert...
    provider: default
    prefix: "🦾"
    toolboxes:
      - filesystem
      - exec
      - search
      - name: git
        tools: [git_status, git_diff]  # tool whitelist
      - http
      - browser
      - state
      - tasks
      - notes
    skills: [coder-workflow]
    effects:
      - kind: trim_tool_results
        params: { max_result_length: 500, preserve_recent: 4 }
      - kind: compact
        params: { threshold: 0.8 }
      - kind: loop_detect
        params: { threshold: 3, window_size: 6 }
      - kind: reflection
        params: { failure_threshold: 2 }
      - kind: progress
        params: { interval: 5 }
    options:
      max_iterations: 100
      max_delegation_depth: 5
```

### 5.3 System Prompt Structure

Ordered for prompt-cache friendliness (static sections first):

1. `<identity>` — Agent name and description
2. `<completion_protocol>` — Sub-agent `task_complete` instructions (depth > 0 only)
3. `<notes_protocol>` — Notes awareness (only if notes tools present)
4. `<instructions>` — Agent-specific behavioral instructions
5. `<behavioral_constraints>` — Heuristic hints (can be disabled)
6. `<project_context>` — External + curated + generated context
7. `<skills>` — Inline skill content
8. `<available_skills>` — On-demand skill descriptions
9. `<available_agents>` — Registry directory (excluding self)

### 5.4 Middleware

Wraps the `Run()` method for cross-cutting concerns. Applied in order (first = outermost):

| Middleware | Purpose |
|-----------|---------|
| `Timeout(duration)` | Cancels agent after deadline |
| `Recovery()` | Catches panics and returns error |
| `Logger(log, name)` | Logs agent lifecycle |
| `OutputGuardrail(check)` | Post-run validation of output |

### 5.5 Entry Agent

One agent is designated `entry_agent` in config. This is the agent that handles user messages in a session.

---

## 6. Multi-Agent Delegation

### 6.1 Registry

A thread-safe directory of agent factories:
- `Register(name, description, factory)` — adds agent blueprint
- `Spawn(name, depth)` — creates fresh instance with incremented delegation depth
- `NextID(configName)` — monotonically increasing counter for unique instance names

Instance names follow the format `<configName>-<taskSlug>-<counter>` for traceability.

### 6.2 Delegation Tool

The `delegate` tool enables agent-to-agent task assignment:

1. Validates: no self-delegation, depth within `MaxDelegationDepth`
2. Spawns child agents from registry
3. Propagates parent's registry, EventNotifier, EventFunc, ReflectionDir, TaskBoard
4. Inherits parent's toolboxes (deduplication by pointer equality; child's own tools take precedence)
5. Searches for prior reflection notes and prepends as `<prior_reflections>`
6. Prepends `<delegation_context>` message with parent context
7. Appends task description as user message
8. Runs all child agents **concurrently**

### 6.3 Completion Protocol

Sub-agents (depth > 0) receive a `task_complete` tool and must call it to signal completion:

```go
type CompletionResult struct {
    Status        string       // "completed", "failed", etc.
    Summary       string
    FilesModified []string
    TestsRun      []string
    Caveats       string
}
```

If a sub-agent exhausts its iteration limit without calling `task_complete`, a synthetic failed result is generated and a reflection note is written.

### 6.4 Task Board Integration

When the `tasks` toolbox is enabled:
- `delegate` auto-claims tasks before running child agents
- Task status is auto-updated when child completes
- Other agents can watch for task completion via blocking `watch` tool

---

## 7. Effects System

Per-iteration hooks that run at defined phases of the ReAct loop. Effects optionally implement `Resetter` for per-run state cleanup.

### 7.1 Available Effects

| Effect | Kind | Phase | Purpose |
|--------|------|-------|---------|
| **CompactEffect** | `compact` | Before | Full conversation summarization when token usage exceeds threshold |
| **TrimToolResultsEffect** | `trim_tool_results` | After | Truncate old tool results to save tokens |
| **SlidingWindowEffect** | `sliding_window` | Before | Three-zone context management (recent full, medium trimmed, old summarized) |
| **ObservationMaskEffect** | `observation_mask` | Before | Replace old observations with brief placeholders |
| **LoopDetectEffect** | `loop_detect` | Before | Detect repeated identical tool calls and inject intervention |
| **ReflectionEffect** | `reflection` | Before | Detect consecutive failures and inject reflection prompt |
| **ProgressEffect** | `progress` | Before | Periodic progress note prompts |

### 7.2 CompactEffect

- **Trigger:** When `input_tokens >= context_window × threshold` (default 0.8)
- **Action:** Renders conversation transcript, sends to LLM for summarization
- **Summary format:** Goal, Completed Work, Files Touched, Key Decisions, Errors, Current State, Next Steps
- **Result:** Replaces chat with system prompt + single compacted user message

### 7.3 TrimToolResultsEffect

- Truncates content of tool results in older messages
- Preserves most recent N tool messages untrimmed (default 4)
- Error results are never trimmed
- Uses metadata to avoid re-trimming already-trimmed results

### 7.4 LoopDetectEffect

- Scans a sliding window for consecutive identical tool calls
- Injects intervention message at threshold (default 3 identical calls)
- Re-injection guard prevents spam at the same count

### 7.5 ReflectionEffect

- Counts consecutive error-only tool results
- Injects reflection prompt at threshold (default 2 failures)
- Prompts agent to reconsider its approach

### 7.6 ProgressEffect

- Every N iterations (default 5), injects a prompt to write a progress note
- Only activates if `write_note` tool is available

### 7.7 Auto-Generated Effects

When the effective context window is non-zero and no explicit effects are configured, the engine auto-generates:
1. `trim_tool_results` (lightweight, runs after completion)
2. `compact` with the agent's context threshold (default 0.8)

Effect priority sorting ensures compaction-class effects run before injection effects.

---

## 8. Token Estimation

### 8.1 Character-to-Token Heuristic

- ~1 token per 4 characters (English text)
- Per-message overhead: 4 tokens
- Per-tool overhead: 10 tokens

### 8.2 Methods

- `EstimateChat(chat)` — Estimate tokens for all messages
- `EstimateTools(tools)` — Estimate tokens for tool definitions
- `EstimateTotal(chat, tools)` — Combined estimate

### 8.3 Usage Tracking

Thread-safe `usage.Tracker` accumulates token counts across multiple API calls within a session. Tracks input and output tokens separately.

---

## 9. Built-in Tools

### 9.1 Tool Architecture

All tools are registered in a flat `ToolBox` collection. Each tool has:
- **Name**: Unique identifier
- **Description**: For LLM consumption
- **InputSchema**: JSON Schema for arguments
- **Handler**: `func(ctx, input) (string, error)`

Tool calls never return Go errors; failures are reported as `ToolResult` with `IsError: true`.

### 9.2 Permission Model

Tools that interact with the local environment use a shared `permissions.Store`:
- **Filesystem:** Directory-level approval (parent approval covers children); symlink resolution checks both logical and real paths
- **Exec:** Command-level trust (trusting `git` allows all `git` invocations)
- **HTTP:** Domain-level trust
- **Browser:** Domain-level trust for navigation

Permission choices: "yes" (single use), "trust" (permanent for session), "no" (deny).

Permissions are persisted atomically to `.shelly/local/permissions.json` via temp-file-then-rename.

### 9.3 Filesystem Tools

| Tool | Description |
|------|-------------|
| `fs_read` | Read file contents (10MB cap; supports offset+limit for large files) |
| `fs_write` | Write file (shows unified diff for confirmation) |
| `fs_edit` | Edit file (old_text must appear exactly once) |
| `fs_list` | List directory contents |
| `fs_delete` | Delete file or directory |
| `fs_move` | Move/rename file |
| `fs_copy` | Copy file |
| `fs_stat` | File metadata |
| `fs_diff` | Unified diff between files |
| `fs_patch` | Apply multiple hunks atomically |
| `fs_mkdir` | Create directory |

Concurrent writes are serialized via per-path mutex. Two-path operations (move, copy) lock in sorted order to avoid deadlocks.

### 9.4 Exec Tool

| Tool | Description |
|------|-------------|
| `exec_run` | Run CLI commands |

- Direct subprocess execution (no shell interpretation)
- 1MB output cap
- Concurrent prompt coalescing for same command
- Three user options: "yes" (single), "trust" (permanent), "no" (deny)

### 9.5 Search Tools

| Tool | Description |
|------|-------------|
| `search_content` | Regex search in files (skips binaries, 1MB total cap) |
| `search_files` | Glob pattern matching (supports `**` for recursive) |

100 results max (configurable). Optional `context_lines` for search_content.

### 9.6 Git Tools

| Tool | Description |
|------|-------------|
| `git_status` | Working tree status |
| `git_diff` | Show diff for path |
| `git_log` | Show commit history (restricted to built-in formats to prevent metadata exfiltration) |
| `git_commit` | Create commit (path traversal protection, no `.` args) |

1MB stdout/stderr cap.

### 9.7 HTTP Tool

| Tool | Description |
|------|-------------|
| `http_fetch` | HTTP requests (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS) |

- SSRF protection: private IP ranges blocked at DNS + connection time
- Redirect validation rejects untrusted domains and private IPs
- 1MB response body cap, 60-second timeout

### 9.8 Browser Tools

| Tool | Description |
|------|-------------|
| `browser_search` | DuckDuckGo search (no domain trust required) |
| `browser_navigate` | Navigate to URL (domain trust check) |
| `browser_click` | Click element on page |
| `browser_type` | Type text into element |
| `browser_extract` | Extract text content (strips scripts/styles, 100KB cap) |
| `browser_screenshot` | Take screenshot (viewport, full_page, or element; base64 PNG) |

Headless Chrome via chromedp. Incognito mode, GPU disabled. Lazy startup with auto-restart on context cancel. 30-second timeout per operation.

### 9.9 Notes Tools

| Tool | Description |
|------|-------------|
| `write_note` | Write a markdown note |
| `read_note` | Read a note by name |
| `list_notes` | List all notes with first-line preview (80 chars) |

Notes are stored in `.shelly/local/notes/`. Name validation (`^[a-zA-Z0-9_-]+$`) prevents path traversal. Notes survive context compaction for persistent cross-agent communication.

### 9.10 Ask Tool

| Tool | Description |
|------|-------------|
| `ask_user` | Ask the user a question (free-form or multiple-choice) |

Questions are auto-incremented (`q-1`, `q-2`, ...). `OnAskFunc` callback notifies the frontend. Supports blocking programmatic questions via `Ask(ctx, text, options[])`.

### 9.11 State Tools

| Tool | Description |
|------|-------------|
| `{ns}_state_get` | Read a key from shared state |
| `{ns}_state_set` | Write a key to shared state |
| `{ns}_state_list` | List all keys in shared state |

Thread-safe key-value store (blackboard pattern). Values stored as `json.RawMessage`. Supports blocking `Watch(ctx, key)` for inter-agent coordination.

### 9.12 Task Tools

| Tool | Description |
|------|-------------|
| `{ns}_tasks_create` | Create a task with title, description, blocked_by, metadata |
| `{ns}_tasks_list` | List tasks (filter by status, assignee, blocked state) |
| `{ns}_tasks_get` | Get task details |
| `{ns}_tasks_claim` | Atomically claim and start a task |
| `{ns}_tasks_update` | Update task fields |
| `{ns}_tasks_watch` | Block until task completes or fails |

Status flow: `pending` → `in_progress` → `completed` / `failed`. Blocking dependencies prevent claiming/reassigning until all deps complete. Task changes broadcast via notification channel.

### 9.13 Toolbox Assignment

- Each agent declares its `toolboxes` list in YAML config
- Toolbox entries can be plain strings (all tools) or objects with `tools` whitelist
- `ask` toolbox is always implicitly included
- At delegation, parent's toolboxes are merged into child (child's own tools take precedence)

**Built-in toolbox names:** `ask`, `filesystem`, `exec`, `search`, `git`, `http`, `browser`, `state`, `tasks`, `notes`

---

## 10. Skills System

### 10.1 Skill Structure

Skills are folder-based definitions stored in `.shelly/skills/`:

```
skills/
  code-review/
    SKILL.md          # Required entry point (YAML frontmatter + body)
    checklist.md      # Supplementary files
```

YAML frontmatter is optional and supports `name` and `description` fields.

### 10.2 Two Modes

| Mode | Condition | Behavior |
|------|-----------|----------|
| **Inline** | No description | Full content embedded in system prompt |
| **On-demand** | Has description | Description listed in prompt; content loaded via `load_skill` tool |

### 10.3 Per-Agent Filtering

Agents can specify which skills they receive:

```yaml
agents:
  - name: coder
    skills: [coder-workflow, testing]  # only these skills
```

Empty/omitted means the agent receives all skills loaded by the engine.

---

## 11. MCP (Model Context Protocol) Integration

### 11.1 MCP Client

Connects to external MCP servers via two transports:

| Transport | Config | Notes |
|-----------|--------|-------|
| **Stdio** | `command: mcp-search` | Subprocess lifecycle managed (SIGTERM/SIGKILL cleanup) |
| **HTTP Streamable** | `url: https://...` | HTTP-based transport |

`ListTools()` discovers available tools and creates handler closures that dispatch calls through the MCP client.

### 11.2 MCP Server

Exposes Shelly's `toolbox.Tool` instances over MCP protocol:

```go
srv := mcpserver.New("shelly", "1.0")
srv.Register(tools...)
srv.Serve(ctx, os.Stdin, os.Stdout)
```

### 11.3 Configuration

```yaml
mcp_servers:
  - name: web-search
    command: mcp-search
    args: [--verbose]
  - name: bright-data
    url: https://mcp.example.com/mcp?token=abc
```

MCP tools are registered as regular toolbox tools and available for assignment to agents.

---

## 12. Project Context

### 12.1 Context Sources (loaded in order)

1. **External:** `CLAUDE.md`, `.cursorrules`, `.cursor/rules/*.mdc` (YAML frontmatter stripped)
2. **Curated:** `*.md` files in `.shelly/` root
3. **Generated:** Auto-generated structural index cached at `.shelly/local/context-cache.json`

### 12.2 Generated Index

Includes:
- Go module path (from `go.mod`)
- Entry points (`cmd/*/main.go` patterns)
- Package listing (`pkg/` subdirs with `.go` files, depth limit 4)

Cache is invalidated when `go.mod` is newer than the cache file.

---

## 13. Shelly Directory

### 13.1 Layout

```
.shelly/
  .gitignore              # Ignores local/
  config.yaml             # Main config (committed)
  context.md              # Curated context (committed)
  *.md                    # Additional context files (committed)
  skills/                 # Skill folders (committed)
    code-review/
      SKILL.md
  local/                  # Gitignored runtime state
    permissions.json      # File/command/domain ACL
    context-cache.json    # Auto-generated project index
    notes/                # Cross-agent notes
    reflections/          # Sub-agent failure reflection notes
```

### 13.2 Bootstrapping

- `Bootstrap(dir)` — Creates full structure from scratch
- `BootstrapWithConfig(dir, config)` — Bootstrap with custom config
- `EnsureStructure(dir)` — Creates `local/` and `.gitignore` (idempotent)
- `MigratePermissions(dir)` — Moves legacy `permissions.json` to `local/` (idempotent)

---

## 14. Engine & Session

### 14.1 Engine

The composition root that wires all components from YAML config:

- Creates provider adapters (completers)
- Connects MCP clients (parallel initialization)
- Loads skills from `.shelly/skills/`
- Loads project context
- Registers agent factories in registry
- Creates shared state/task stores (when referenced by agents)
- Manages session lifecycle

### 14.2 Session

Represents one interactive conversation:

- `Send(ctx, text)` — Appends user message, runs agent ReAct loop, returns reply
- `SendParts(ctx, ...parts)` — Like Send but with explicit content parts
- `Chat()` — Returns underlying chat container
- `Respond(questionID, response)` — Answers pending `ask_user` question

Thread-safe: only one `Send` active per session at a time (mutual exclusion lock).

### 14.3 EventBus

Channel-based pub/sub for observing engine activity:

| Event Kind | Data | Description |
|------------|------|-------------|
| `message_added` | `{role, message}` | Message appended to chat |
| `tool_call_start` | `{tool_name, call_id}` | Tool execution begins |
| `tool_call_end` | `{tool_name, call_id}` | Tool execution completes |
| `agent_start` | `{prefix, parent}` | Sub-agent spawned |
| `agent_end` | `{prefix, parent}` | Sub-agent finished |
| `ask_user` | `ask.Question` | Question for user |
| `file_change` | `string` | File modified |
| `compaction` | `string` | Context compacted |
| `error` | `error` | Error occurred |

Subscribe with configurable buffer size. Non-blocking publish (drops events to full subscribers to prevent loop stalls).

---

## 15. Terminal User Interface (TUI)

### 15.1 Architecture

Built on Bubbletea v2 with a state-machine model:

| State | Description |
|-------|-------------|
| `Idle` | Waiting for user input |
| `Processing` | Agent is thinking/running |
| `AskUser` | User interaction needed |

### 15.2 Layout

Two vertically stacked regions:

1. **Messages region** (top) — Append-only, grows downward. Uses terminal's native scroll. Never re-rendered in place. Contains user messages, agent messages, tool calls, and sub-agent containers.
2. **Input region** (bottom) — Fixed-height, re-rendered in place. Contains task panel (when active), user input field, and token counter.

Minimum supported terminal width: 80 columns.

### 15.3 Theme

GitHub terminal light theme with dark-mode detection at startup (via OSC 11).

**Color palette:**

| Name | Hex | Usage |
|------|-----|-------|
| Foreground | #24292f | Primary text |
| Muted | #656d76 | Secondary text, completed tasks |
| Accent | #0969da | Interactive elements |
| Error | #cf222e | Error text |
| Success | #1a7f37 | Completed items |
| Warning | #9a6700 | Questions, ask UI |
| Magenta | #8250df | Spinners, sub-agents |

**Sub-agent color palette** (round-robin assignment):
Blue (#0969da), Green (#1a7f37), Orange (#bc4c00), Pink (#bf3989), Teal (#0a7480), Purple (#8250df)

### 15.4 User Input

- **Enter:** Submit message
- **Shift+Enter / Alt+Enter:** Insert newline (multi-line input)
- **@:** Open file picker
- **/:** Open command picker
- **Escape:** Dismiss picker → interrupt agent (innermost context first)
- **Ctrl+C:** Quit

Input remains enabled while the agent is working. Messages sent during processing are queued and delivered when the agent is ready.

Token counter displayed below input (formatted with k/M suffixes). Hidden when a picker is open.

### 15.5 File Picker

Triggered by `@` in input:
- Discovers files via `WalkDir()` (skips `.git`, `node_modules`, `vendor`, `.shelly`; max 1000 entries)
- Filters by basename prefix match (weighted) then full path contains match
- Max 4 visible items; scroll with ↑/↓
- Enter/Tab to select, Escape to dismiss
- Selected file path inserted into input and highlighted in sent message

### 15.6 Command Picker

Triggered by `/` in input:
- Static commands: `/help`, `/clear`, `/exit`
- Substring filter as user types
- Enter executes immediately (no second Enter needed)
- Escape dismisses without executing

### 15.7 Chat View

Renders only in-progress agent activity. Committed content is printed via `tea.Println` and scrolls natively.

**Agent Containers:**
- Each agent gets an `AgentContainer` with display items
- Sub-agents shown as nested containers with 4-line windowing
- Tool calls show name, args, elapsed time, and result
- Parallel tool calls grouped in `ToolGroupItem`
- Final answers stored on container, emitted as summary when agent ends

**Display Items:**
| Item | Description |
|------|-------------|
| `ThinkingItem` | Agent reasoning text |
| `PlanItem` | Agent planning with 📝 prefix |
| `ToolCallItem` | Single tool invocation (running spinner → done with result) |
| `ToolGroupItem` | Parallel tool execution group |
| `SubAgentItem` | Nested agent container |

**Spinner:** Braille progression frames (⣾→⣽→⣻→⢿→⡿→⣟→⣯→⣷) with 16 random thinking messages.

### 15.8 Task Panel

Displayed above input when tasks are active:
- Title line: "Tasks" + status counts (N pending, N in progress, N completed)
- Tasks sorted by status: pending → in progress → completed
- Icons: `○` pending, `⣾` in progress (magenta spinner), `✓` completed (light gray)
- Agent name in parentheses after task text
- Max 6 tasks shown (prioritizes pending/in-progress over completed)
- Disappears when all tasks complete

### 15.9 Ask User UI

When agents ask questions:
- Questions batch with 200ms debounce window
- Tabbed interface: `[Q1] [Q2] ... [Confirm]`
- Question types: free-form, single-select, multi-select, custom override
- Navigation: ←/→ for tabs, ↑/↓ for options, Space for toggle, Enter to confirm, Esc to reject
- Confirm tab shows all answers for review

### 15.10 Bridge

Two goroutines synchronize engine events to TUI messages:

1. **Event Watcher:** Subscribes to EventBus → translates to TUI messages
2. **Chat Watcher:** Polls chat via `Wait()`/`Since()` for new messages
3. **Task Watcher** (optional): Monitors task store mutations

All communication via `p.Send()` — never touches model state directly.

### 15.11 Markdown Rendering

Uses Glamour (charmbracelet) with cached renderer. Respects terminal light/dark theme detected at startup.

### 15.12 Stale Send Detection

Monotonic `sendGeneration` counter prevents processing results from cancelled sends after Escape.

---

## 16. Configuration Wizard

### 16.1 Interactive Setup

Stack-based screen navigation with main menu:

1. **Providers** — Add/edit LLM provider configurations
2. **Agents** — Define agent names, providers, toolboxes, effects, skills, prompts
3. **MCP Servers** — Configure MCP server connections
4. **Settings** — Global settings
5. **Review & Save** — Preview generated YAML and persist to `.shelly/config.yaml`

### 16.2 Templates

Pre-built configurations for quick setup:

```bash
./bin/shelly init --template list              # List available templates
./bin/shelly init --template simple-assistant   # Single agent setup
./bin/shelly init --template dev-team           # Orchestrator + planner + coder
```

Templates define provider slots (e.g., "primary", "fast") mapped to actual providers during setup. Agent structure and embedded skills come from the template.

### 16.3 CLI Flags

| Flag | Description |
|------|-------------|
| `--config` | Explicit config path |
| `--shelly-dir` | Override `.shelly/` directory |
| `--env` | Override `.env` path |
| `--agent` | Override entry agent |

Config resolution: explicit flag → `.shelly/config.yaml` → `shelly.yaml` (legacy).

---

## 17. Concurrency & Thread Safety

### 17.1 Core Guarantees

| Component | Mechanism |
|-----------|-----------|
| `chat.Chat` | `sync.RWMutex`; signal-based streaming |
| `state.Store` | `sync.RWMutex`; deep-copy on read; close+recreate signal for Watch |
| `tasks.Store` | `sync.RWMutex`; atomic Claim/Reassign; signal-based WatchCompleted |
| `permissions.Store` | `sync.RWMutex`; atomic persistence via temp-file-then-rename |
| Filesystem tools | Per-path `FileLocker`; sorted lock order for two-path ops |
| Exec tools | Concurrent prompt coalescing per command |
| Session | Mutual exclusion on Send |
| Registry | `sync.RWMutex` for factory access |
| EventBus | `sync.RWMutex` for subscription management |
| RateLimitedCompleter | Two mutexes: window tracking + call sequencing |

### 17.2 Tool Execution

Tool calls within a single ReAct iteration execute concurrently via `sync.WaitGroup`. Results are collected in order. Context cancellation is honored after all tools complete.

---

## 18. Security

### 18.1 Permission Gating

All tools that interact with the local environment require explicit user approval:
- Filesystem: directory-level (parent covers children)
- Exec: command-level
- HTTP/Browser: domain-level

### 18.2 SSRF Protection

HTTP tool blocks private IP ranges at both DNS resolution and connection time. Redirect validation rejects untrusted domains and private IPs.

### 18.3 Output Caps

| Resource | Limit |
|----------|-------|
| File read | 10MB |
| Command output | 1MB |
| HTTP response body | 1MB |
| Browser text extraction | 100KB |
| Search results | 100 entries |
| File picker entries | 1000 |

### 18.4 Path Traversal Protection

- Git commit rejects paths starting with `-` and `.` args
- Git log restricted to built-in formats to prevent metadata exfiltration
- Notes name validation: `^[a-zA-Z0-9_-]+$`
- Skill paths use relative paths to prevent machine-specific path leakage

---

## 19. Reflections

### 19.1 Failure Reflections

When sub-agents fail (exhaust iteration limit without calling `task_complete`):
- A reflection note is written to `.shelly/local/reflections/`
- Filename: `<agent-name>-<timestamp>.md`
- Content includes: task, approach taken, reason for failure

### 19.2 Reflection Retrieval

When delegating, the parent agent searches for prior reflections relevant to the task and prepends them as `<prior_reflections>` in the child's context. This enables learning from past failures.

---

## 20. Agent Context Propagation

### 20.1 Context Keys

The `agentctx` package provides zero-dependency context key helpers:
- `WithAgentName(ctx, name)` — Inject agent identity
- `AgentNameFromContext(ctx)` — Extract agent identity

Used by: agent (at `Run` start), engine (session start), tasks (task attribution).

---

## 21. Development Tooling

| Tool | Purpose | Config |
|------|---------|--------|
| [gofumpt](https://github.com/mvdan/gofumpt) | Code formatting (strict superset of gofmt) | — |
| [golangci-lint v2](https://golangci-lint.run/) | Linting (50+ linters) | `.golangci.yml` |
| [testify](https://github.com/stretchr/testify) | Test assertions (`assert`, `require`) | — |
| [gotestsum](https://github.com/gotestyourself/gotestsum) | Formatted test output | — |
| [go-task](https://taskfile.dev/) | Task runner | `Taskfile.yml` |

### 21.1 Build Tasks

| Task | Description |
|------|-------------|
| `task build` | Build to `./bin/shelly` |
| `task run` | Run the application |
| `task fmt` / `task fmt:check` | Format / check formatting |
| `task lint` / `task lint:fix` | Lint / auto-fix |
| `task test` | Run tests |
| `task test:coverage` | Tests with coverage |
| `task check` | All checks (fmt + lint + test) |

### 21.2 Linter Extras

gosec, gocritic, gocyclo (max 15), unconvert, misspell, modernize, testifylint.
