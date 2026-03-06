# CLI & TUI Interface

The Shelly command-line interface and terminal UI live in `cmd/shelly/`. The entry point is `main.go` which dispatches to subcommands. The interactive TUI uses **bubbletea v2** and **lipgloss v2** with a modular component architecture in `cmd/shelly/internal/`.

---

## Entry Point (`main.go`)

**Subcommand dispatch** via `flag.NewFlagSet`:
- `(default)` — Interactive TUI session (`runInteractive`)
- `index` — Project indexing mode (`runIndex`)
- `batch` — Batch processing mode (`runBatch`)
- `init` — Initialize `.shelly/` directory (`runInit`)
- `config` — Edit existing configuration (`runConfig`)

**Top-level flags** (for interactive mode): `--config`, `--shelly-dir`, `--session`, `--agent`, `--prompt`, `--attachment`.

**Startup flow** (`runInteractive`):
1. Load `.env` via `loadDotEnv()`
2. Resolve config path via `resolveConfigPath()`
3. Load and validate `engine.Config`
4. Create `engine.Engine` with `engine.New()`
5. Subscribe to engine EventBus
6. Create bubbletea program with `app.New()` model
7. Start the TUI with `tea.NewProgram().Run()`

If `--prompt` is provided with no TTY, falls back to non-interactive mode (sends prompt, streams output to stdout).

### Helpers (`helpers.go`)

- **`loadDotEnv(path)`** — Loads `.env` file, ignores missing files
- **`resolveConfigPath(flagValue, shellyDir)`** — Priority: explicit `--config` → `<shellyDir>/config.yaml` → `shelly.yaml` in CWD

---

## Subcommands

### Batch Mode (`batch.go`)

**`runBatch(args)`** — Runs tasks from JSONL input:
- Flags: `--config`, `--shelly-dir`, `--input` (file or stdin), `--output` (file or stdout), `--concurrency`
- Loads engine, calls `engine.RunBatch(ctx, input, output, concurrency)`
- Handles SIGINT/SIGTERM for graceful shutdown

### Index Mode (`index.go`)

**`runIndex(args)`** — Runs a project indexing agent session via the TUI:
- Flags: `--config`, `--shelly-dir`, `--agent` (defaults to `"index-explorer"`)
- Creates the engine, finds or defaults the indexer agent
- Opens a TUI session with a pre-set indexing prompt
- Uses the same TUI infrastructure as interactive mode, with `app.WithInitialPrompt()` and `app.WithTargetAgent()`
- Supports `--template` flag for loading index templates from `.shelly/skills/`

### Init Command (`init.go`)

**`runInit(args)`** — Initializes a new `.shelly/` directory:
- Flags: `--shelly-dir`, `--template`
- If `--template` specified, applies a project template directly
- Otherwise, launches a **config wizard** TUI (`configwizard` package) for interactive setup

### Config Command (`config.go`)

**`runConfig(args)`** — Edits existing configuration:
- Flags: `--shelly-dir`, `--config`
- Loads existing config, launches config wizard pre-populated with current values
- Validates and persists updated config

---

## TUI Architecture (`cmd/shelly/internal/`)

### Package Layout

| Package | Purpose |
|---------|---------|
| `app/` | Main bubbletea Model — state machine, Update/View, focus management |
| `bridge/` | Engine↔TUI bridge — subscribes to EventBus, translates to tea.Msg |
| `msgs/` | Message types shared between bridge and TUI components |
| `chatview/` | Chat display — viewport, message rendering, agent containers |
| `input/` | User input area — textarea, attachments, file picker, command picker, history |
| `menubar/` | Top menu bar with clickable items and keyboard shortcuts |
| `askprompt/` | Prompt overlay for agent-initiated questions (ask tool) |
| `configwizard/` | Multi-step configuration wizard (providers, agents, MCP, review) |
| `format/` | Markdown rendering, duration formatting, spinner frames, tool formatting |
| `styles/` | Shared lipgloss color palette and style definitions |
| `templates/` | Embedded project templates for `shelly init` |
| `basetextarea/` | Custom textarea component (wraps bubbles textarea) |
| `list/` | Reusable list selection component |
| `tty/` | TTY detection and output flushing utilities |

### App Model (`app/app.go`)

The central bubbletea `Model` with state machine:

```go
type Model struct {
    state          appState       // current state (loading/chatting/asking/wizard)
    engine         *engine.Engine
    session        *engine.Session
    chatView       chatview.Model
    inputArea      input.Model
    menuBar        menubar.Model
    askOverlay     askprompt.Model
    bridge         *bridge.Bridge
    // ...dimensions, focus, config
}
```

**States (`appState`):**
- `stateLoading` — Engine initializing
- `stateChatting` — Normal chat interaction
- `stateAsking` — Agent is asking the user a question (overlay mode)
- `stateWizard` — Config wizard active

**Focus system** — Tracks which component receives keyboard input. Focus cycles between `focusChatView` and `focusInput` via Tab key. The menu bar is always accessible.

**Key Update flow:**
1. Receives `tea.Msg` (key press, mouse, window resize, or bridge message)
2. Routes bridge messages (`msgs.*`) to update chat view, handle ask requests, track usage
3. Routes key events based on current state and focus
4. Returns updated model + optional commands

**Commands (`commands.go`):**
- `cmdInitEngine()` — Async engine creation
- `cmdNewSession()` — Creates engine session with options
- `cmdSendMessage()` — Sends user input to session
- `cmdRespondAsk()` — Responds to agent's ask prompt
- `cmdSaveSession()` — Persists session state

**Constructor options:**
- `WithInitialPrompt(text)` — Auto-send prompt on session start
- `WithTargetAgent(name)` — Target specific agent
- `WithSessionID(id)` — Resume existing session
- `WithAttachments(...)` — Pre-attach files

### Bridge (`bridge/bridge.go`)

Connects the async engine event system to bubbletea's synchronous message loop:

```go
type Bridge struct {
    eventCh    chan engine.Event
    program    *tea.Program
    askCh      chan ask.Request  // from session
    cancelTick context.CancelFunc
}
```

**`New(eventCh, program)`** — Creates bridge, starts goroutine that:
1. Listens on `eventCh` for engine events
2. Translates each `engine.Event` into appropriate `msgs.*` type
3. Calls `program.Send()` to inject into bubbletea loop

**Event translation mapping:**
| Engine Event | TUI Message |
|-------------|-------------|
| `EventMessageAdded` | `msgs.ChatMessage` |
| `EventToolCallStart` | `msgs.ToolCallStart` |
| `EventToolCallEnd` | `msgs.ToolCallEnd` |
| `EventAgentStart` | `msgs.AgentStart` |
| `EventAgentEnd` | `msgs.AgentEnd` |
| `EventUsageUpdate` | `msgs.UsageUpdate` |
| `EventStreamDelta` | `msgs.StreamDelta` |
| `EventThinking` | `msgs.ThinkingUpdate` |
| `EventPlan` | `msgs.PlanUpdate` |
| `EventSummaryLine` | `msgs.SummaryLine` |
| `EventError` | `msgs.ErrorOccurred` |

**Ask handling:** Starts a separate goroutine watching `session.Ask()` channel. When agent sends an ask request, bridge converts it to `msgs.AskRequest` and injects it into the TUI.

**Spinner ticking:** Starts a tick goroutine (100ms interval) sending `msgs.SpinnerTick` to animate spinners for in-progress operations.

### Messages (`msgs/msgs.go`)

Typed message structs flowing from bridge to TUI:

- **Chat:** `ChatMessage` (full message), `StreamDelta` (streaming text chunk), `StreamEnd`
- **Tool calls:** `ToolCallStart` (name, args, callID), `ToolCallEnd` (callID, result, isError)
- **Agent lifecycle:** `AgentStart` (name, icon, color, provider), `AgentEnd` (name, final answer)
- **Thinking/Plan:** `ThinkingUpdate`, `PlanUpdate`, `SummaryLine`
- **State:** `UsageUpdate`, `ErrorOccurred`, `SessionLoaded`
- **Interaction:** `AskRequest` (question, options), `SpinnerTick`

### Chat View (`chatview/`)

**`Model`** — Manages the scrollable chat display:
- Contains a `viewport.Model` (bubbles viewport) for scrolling
- Maintains ordered list of `DisplayItem` interfaces
- Tracks `AgentContainer` stack for nested agent rendering
- Handles message grouping and tool call correlation

**Display Items (`items.go`):**
- `ThinkingItem` — Agent thinking/reasoning display
- `PlanItem` — Agent plan display
- `ToolCallItem` — Individual tool call with spinner → result
- `ToolGroupItem` — Groups parallel calls of the same tool (windowed display)
- `SummaryLineItem` — Collapsed summary for finished sub-agents
- `UserMessageItem` — User input display
- `AssistantMessageItem` — Assistant text response

**Agent Container (`container.go`):**
- `AgentContainer` — Accumulates display items for one agent
- Tracks `CallIndex` (map[string]*ToolCallItem) for O(1) tool call lookup by ID
- Supports windowed display (`MaxShow`) for sub-agents — shows only recent N items
- Renders live spinner or collapsed summary when done
- `CollapsedSummary()` shows agent name, nested sub-agent summaries, final answer, and elapsed time
- `AdvanceSpinners()` recursively increments frame indices for animation

**Key behaviors:**
- Auto-scrolls to bottom on new content (unless user has scrolled up)
- Groups consecutive tool calls of the same tool into `ToolGroupItem`
- Sub-agent containers are nested within parent containers with tree-pipe indentation
- Live items show animated spinners; completed items show elapsed time

### Input Area (`input/input.go`)

**`Model`** — Multi-mode input component:

**Modes:**
- `ModeNormal` — Standard text input with textarea
- `ModeAttach` — File attachment picker
- `ModeCommand` — Slash command picker (`/session`, `/new`, `/clear`, `/exit`, etc.)

**Features:**
- **File attachments** — File picker with glob filtering, attached files shown as badges
- **Command picker** — `/`-prefix triggers command selection overlay
- **Input history** — Up/Down arrows navigate previous inputs
- **Multi-line** — Shift+Enter for newlines, Enter to send
- **Attachment badge display** — Shows attached file names with remove capability

**Submit flow:** On Enter, collects text + attachments into `msgs.SubmitMsg`, resets input state.

### Config Wizard (`configwizard/`)

Multi-step TUI wizard for creating/editing `.shelly/config.yaml`:

**Steps (separate files):**
- `templatepicker.go` — Choose project template
- `provider.go` — Configure LLM providers (type, model, API key)
- `agent.go` — Configure agents (name, provider, instructions)
- `mcpserver.go` — Configure MCP tool servers
- `settings.go` — Global settings (max turns, timeouts)
- `review.go` — Review and confirm configuration
- `persist.go` — Write config to disk

Uses `form.go` for a shared form component with field navigation.

### Templates (`templates/templates.go`)

**Embedded project templates** via `//go:embed settings/*.yaml skills/*.md`:

- `ListTemplates()` — Returns available template metadata
- `ApplyTemplate(name, shellyDir)` — Copies template files to `.shelly/`
- Templates include pre-configured settings YAML and skill markdown files

### Styles (`styles/styles.go`)

GitHub-inspired light terminal color palette:
- `ColorFg` (#24292f), `ColorMuted` (#656d76), `ColorAccent` (#0969da)
- `ColorError` (#cf222e), `ColorSuccess` (#1a7f37), `ColorWarning` (#9a6700)
- `ColorBorder` (#d0d7de), `ColorBg` (#ffffff), `ColorBgMuted` (#f6f8fa)

Pre-built styles: `DimStyle`, `AccentStyle`, `ErrorStyle`, `SuccessStyle`, `WarningStyle`, `TreePipe` (`│ `), `TreeCorner` (`└ `), `SpinnerStyle`.

### Format (`format/`)

- **`format.go`** — `RenderMarkdown(text)` using glamour, `FmtDuration()`, `FmtTokens()`, `RandomThinkingMessage()`, `SpinnerFrames` (braille animation)
- **`toolformat.go`** — Tool-call-specific formatting: `FormatToolCall(name, args)` produces compact summaries (e.g., file paths for fs tools, search queries, git commands)

### TTY Utilities (`tty/`)

- `drain.go` — `DrainAndRestore()` for cleaning up terminal state
- `flush_unix.go` — Platform-specific stdout flushing

---

## Key Patterns

1. **Bridge pattern** — Async engine events are translated to synchronous bubbletea messages through a dedicated bridge goroutine, keeping the TUI's Elm-architecture clean
2. **Component composition** — Each TUI section (chat view, input, menu bar, ask overlay) is an independent bubbletea model composed by the app model
3. **Focus management** — Tab-cycling between components; current focus determines which component receives key events
4. **State machine** — App transitions between loading→chatting→asking states; each state has distinct Update/View behavior
5. **Display item hierarchy** — AgentContainer → DisplayItem (ThinkingItem, ToolCallItem, ToolGroupItem) supports nested sub-agent rendering with windowing
6. **Non-blocking event publishing** — Bridge drops events when TUI channel is full rather than blocking the engine
7. **Session continuity** — Sessions are persisted to JSON files and can be resumed by ID across process restarts
