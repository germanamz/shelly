# CLI & TUI Interface (`cmd/shelly/`)

## Overview

The CLI entry point and interactive TUI for Shelly. Built with **bubbletea v2** for the terminal UI, **lipgloss v2** for styling, and **glamour** for markdown rendering. Supports three execution modes: interactive TUI (default), batch processing, and project indexing.

## CLI Entry Point (`main.go`)

### Subcommands & Flag Parsing

`main()` dispatches on `os.Args[1]`:

| Subcommand | Handler | Purpose |
|------------|---------|---------|
| *(default)* | `main()` inline | Interactive TUI session |
| `config` | `runConfig()` | Launch config wizard TUI |
| `init` | `runInit()` | Initialize `.shelly/` from templates |
| `batch` | `runBatch()` | Headless batch execution |
| `index` | `runIndex()` | Project indexing (TUI or headless) |

### Common Flags (default mode)

| Flag | Default | Purpose |
|------|---------|---------|
| `--config` | auto-resolved | Path to config YAML |
| `--shelly-dir` | `.shelly` | Path to `.shelly/` directory |
| `--env` | `.env` | Path to `.env` file |
| `--agent` | `""` | Agent override (skip agent picker) |
| `--session` | `""` | Resume a specific session |

### Startup Flow (Interactive Mode)

1. Parse flags, load `.env` via `loadDotEnv()`
2. Resolve config path: explicit `--config` → `.shelly/config.yaml` → `shelly.yaml`
3. Detect terminal background (dark/light) via `lipgloss.HasDarkBackground()` — stored in `format.IsDarkBG` *before* bubbletea starts to avoid OSC 11 race conditions
4. Create `engine.Engine` from config YAML
5. Initialize `format.InitMarkdownRenderer()` at detected terminal width
6. Create `app.Model` with engine, initial agent, session ID
7. Run `tea.NewProgram(model)` with full-screen alt-screen, mouse cell motion, filter for stale escape sequences

## Configuration & Initialization

### `config.go` — `runConfig()`
Launches a `configwizard.Model` TUI that walks through provider, agent, and MCP server setup. If config already exists, offers to edit it.

### `init.go` — `runInit()`
Bootstraps `.shelly/` directory from built-in templates. Flags: `--shelly-dir`, `--template` (default `"default"`). Can run standalone (applies template) or launch config wizard if needed.

### `helpers.go`
- `loadDotEnv(path)` — Loads `.env` file, ignores missing files
- `resolveConfigPath(explicit, shellyDir)` — Config path priority: explicit flag → `<shellyDir>/config.yaml` → `shelly.yaml`

## Batch Mode (`batch.go`)

Headless execution without TUI. Reads prompt from stdin or `--prompt` flag.

```
shelly batch --config ... --agent myagent --prompt "do something"
```

Creates engine, starts session via `engine.NewSession()`, subscribes to events, prints streamed text/tool-use to stdout. Uses signal context (`SIGINT`/`SIGTERM`) for cancellation. Outputs token usage summary on completion.

## Index Mode (`index.go`)

Runs project indexing. Two sub-modes:
- **Headless** (`--headless`): runs indexing agent non-interactively, prints to stdout
- **Interactive** (default): launches full TUI with the `"project-indexer"` agent pre-selected

Same flag set as default mode plus `--headless` and `--prompt`.

## TUI Architecture (`internal/`)

### App Model (`internal/app/app.go`)

The root bubbletea model. **State machine** with modes:

```
type mode int
const (
    modeChat    mode = iota  // Main conversation view
    modeAsk                   // Agent asking user a question
    modeConfig                // Configuration wizard
)
```

**Key fields:**
- `eng *engine.Engine` — composition root
- `sess *engine.Session` — current active session
- `bridge bridge.Bridge` — event forwarder (agent→TUI)
- `chat chatview.Model` — conversation display
- `input input.Model` — user input area
- `askPrompt askprompt.Model` — agent question handler
- `configWizard configwizard.Model` — setup wizard
- `mode mode` — current UI state
- `spinFrame int` — spinner animation frame

**Lifecycle:**
- `Init()` → starts bridge watcher, session picker (if no session specified), ticker for spinner
- `Update()` → dispatches on mode, handles bridge messages, window resize, key events
- `View()` → renders header bar + mode-specific content + status bar

### Commands (`internal/app/commands.go`)

Tea commands for async operations:
- `startSessionCmd` — creates/resumes session, starts bridge
- `sendMessageCmd` — sends user message with attachments to session
- Bridge message handlers convert engine events to TUI messages

### Event Bridge (`internal/bridge/bridge.go`)

Bridges engine's `EventBus` to bubbletea's message system. Runs in a goroutine, watches for:

| Engine Event | TUI Message |
|-------------|-------------|
| `ChatMessage` | `msgs.ChatMessageMsg` |
| `ChatDelta` | `msgs.ChatDeltaMsg` |
| `AgentDone` | `msgs.AgentDoneMsg` |
| `AskUser` | `msgs.AskUserMsg` |
| `TaskUpdate` | `msgs.TaskUpdateMsg` |

Uses `tea.Program.Send()` to forward events to the TUI. Has `Stop()` for cleanup with `sync.Once`.

### Messages (`internal/msgs/msgs.go`)

Typed bubbletea messages for inter-component communication:

- **Bridge→TUI**: `ChatMessageMsg`, `ChatDeltaMsg`, `AgentDoneMsg`, `AskUserMsg`, `TaskUpdateMsg`
- **Internal**: `SubmitMsg` (user sends message), `SessionSelectedMsg`, `ConfigDoneMsg`, `PickerVisibilityMsg`, `SpinnerTickMsg`, `SessionsLoadedMsg`

### Chat View (`internal/chatview/`)

Scrollable conversation display built on `viewport.Model`.

**`chatview.go` — `Model`:**
- Maintains `[]DisplayItem` list of renderable items
- Viewport-based scrolling with auto-scroll to bottom
- Handles streaming deltas — creates/updates `Container` for live content
- Processes full messages — converts to appropriate `DisplayItem` types
- Token usage display in status line

**`items.go` — Display Items** (all implement `DisplayItem` interface):

| Type | Renders |
|------|---------|
| `ThinkingItem` | Animated thinking indicator with spinner |
| `TextItem` | Markdown-rendered text block (user/assistant) |
| `ToolUseItem` | Tool invocation with name, collapsible input/output |
| `ErrorItem` | Error message display |
| `CostItem` | Token usage and cost summary |
| `SubAgentItem` | Delegated sub-agent activity |

**`container.go` — `Container`:**
Accumulates streamed content blocks for a single message. Tracks agent name, color, timing. Converts deltas into final display items. Supports content types: text, thinking, tool_use, tool_result.

### Input Component (`internal/input/`)

Multi-feature input area:

**`input.go` — `Model`:**
- Auto-growing `basetextarea` (2–10 lines)
- **Enter** sends, **Shift+Enter** / **Alt+Enter** for newline
- **Ctrl+C** to cancel running agent
- File attachments via `@` prefix (triggers file picker)
- Slash commands via `/` prefix (triggers command picker)
- Input history navigation (Up/Down arrows)
- Attachment bar display
- Session picker integration

**`filepicker.go`:** Autocomplete popup for `@path` mentions. Scans filesystem, fuzzy-filters, max 4 visible entries. Resolves to file attachments.

**`cmdpicker.go`:** Slash command autocomplete. Available commands:
- `/new` — New conversation
- `/compact` — Compact conversation history
- `/session` — Browse/resume sessions
- `/config` — Open configuration wizard

**`sessionpicker.go`:** Popup for browsing saved sessions. Shows agent name, model, timestamp, preview. Supports search filtering.

**`history.go`:** Shell-like input history. Persisted to disk (null-byte delimited). Max 500 entries. Up/Down navigation with draft saving.

**`attachment.go`:** File attachment handling. Reads files up to 20MB, detects MIME type, classifies as image/document/text. Converts to `content.ContentBlock` for the chat message.

### Ask Prompt (`internal/askprompt/askprompt.go`)

Handles agent-initiated questions (via `ask` tool). Supports:
- **Free-text** responses with auto-growing textarea
- **Single-select** from options list
- **Multi-select** with checkboxes (space to toggle, enter to submit)
- **Batched questions** with tab navigation between multiple pending asks
- Tab header shows question labels

### Config Wizard (`internal/configwizard/`)

Multi-step TUI wizard for creating/editing `config.yaml`:
- `form.go` — Main form controller, step navigation
- `provider.go` — Provider selection (Anthropic, OpenAI, Grok, Gemini)
- `agent.go` — Agent configuration (name, model, system prompt)
- `mcpserver.go` — MCP server configuration
- `settings.go` — General settings
- `review.go` — Review and confirm
- `persist.go` — Writes config to disk
- `templatepicker.go` — Built-in template selection

### Styling (`internal/styles/styles.go`)

GitHub-inspired light theme palette:

| Variable | Color | Use |
|----------|-------|-----|
| `ColorFg` | `#24292f` | Primary foreground |
| `ColorMuted` | `#656d76` | Dim/muted text |
| `ColorAccent` | `#0969da` | Accent blue |
| `ColorError` | `#cf222e` | Error red |
| `ColorSuccess` | `#1a7f37` | Success green |

Pre-built styles: `UserPrefixStyle`, `AgentPrefixStyle`, `ToolStyle`, `ErrorStyle`, `MutedStyle`, etc. Tree-drawing characters (`TreeCorner = "└ "`, `TreePipe = "│ "`) for structured output.

### Formatting (`internal/format/`)

- **`format.go`:** Markdown rendering via glamour (dark/light auto-detect), token/cost/byte/duration formatting, word wrapping, user message rendering, animated thinking messages with spinner frames
- **`toolformat.go`:** Tool-use specific formatting — renders tool name, input parameters, output/errors with truncation for large results

### Base Textarea (`internal/basetextarea/`)

Wrapper around `bubbles/v2/textarea` adding auto-grow behavior. Grows from `MinHeight` to `MaxHeight` based on visual line count. Handles word-wrap line counting to match textarea's internal rendering.

### TTY Utilities (`internal/tty/`)

- `InputEnabler` interface — models report whether user input is active
- `NewStaleEscapeFilter()` — Suppresses stale escape sequences that arrive after input is disabled (e.g., during agent execution)
- Platform-specific flush (`flush_unix.go` / `flush_other.go`)

## Component Relationships

```
main.go
  └─ app.Model (root bubbletea model)
       ├─ bridge.Bridge ─── engine.EventBus ─── agent goroutine
       │    └─ sends msgs.* to tea.Program
       ├─ chatview.Model
       │    ├─ []DisplayItem (TextItem, ToolUseItem, ThinkingItem, ...)
       │    └─ Container (accumulates streaming deltas)
       ├─ input.Model
       │    ├─ basetextarea.Model (auto-growing textarea)
       │    ├─ FilePickerModel (@-mentions)
       │    ├─ CmdPickerModel (/commands)
       │    ├─ SessionPickerModel (session browser)
       │    ├─ History (persistent input history)
       │    └─ []Attachment (pending file attachments)
       ├─ askprompt.Model (agent questions)
       └─ configwizard.Model (setup wizard)
```

## Key Patterns

1. **Background detection before TUI** — Terminal background color detected via OSC query *before* bubbletea starts to avoid escape sequence conflicts
2. **Bridge pattern** — Engine events flow through a goroutine bridge that converts them to bubbletea messages, decoupling agent execution from UI rendering
3. **State machine modes** — `modeChat`/`modeAsk`/`modeConfig` control which sub-model receives updates and renders
4. **Streaming display** — `Container` accumulates deltas into live `DisplayItem`s, finalized on message completion
5. **Stale input filter** — Custom `tea.WithFilter` suppresses escape sequences arriving when input is disabled (during agent processing)
