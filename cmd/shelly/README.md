# cmd/shelly

CLI entry point and Bubbletea v2 TUI for Shelly. This package parses command-line flags, loads configuration, creates the engine, and runs an interactive terminal interface where users converse with LLM agents.

## Purpose

`cmd/shelly/` is the top of the application stack. It is responsible for:

1. Parsing CLI flags (`--config`, `--shelly-dir`, `--env`, `--agent`).
2. Loading `.env` files and resolving the YAML configuration path.
3. Creating an `engine.Engine` and an initial `engine.Session`.
4. Running a Bubbletea v2 program that renders chat messages, tool calls, agent activity, and user input in a terminal UI.

## File Organization

```
cmd/shelly/
  main.go              CLI entry point: flag parsing, engine creation, program launch
  helpers.go           loadDotEnv(), resolveConfigPath() utilities
  internal/
    app/
      app.go           Root bubbletea model (AppModel), state machine, message routing
    msgs/
      msgs.go          All bubbletea message types shared across internal packages
    styles/
      styles.go        Centralized lipgloss color palette and style definitions
    format/
      format.go        Markdown rendering, token/duration formatting, spinner frames
      toolformat.go    Human-readable labels for tool calls (ToolFormatters registry)
      format_test.go   Tests for formatting utilities
    chatview/
      chatview.go      ChatViewModel: viewport, message routing, agent lifecycle
      container.go     AgentContainer: accumulates display items for one agent
      items.go         DisplayItem interface + concrete types (ThinkingItem, ToolCallItem, etc.)
      container_test.go
      chatview_test.go
    input/
      input.go         InputModel: textarea with auto-grow, picker integration
      filepicker.go    FilePickerModel: @-mention file autocomplete popup
      cmdpicker.go     CmdPickerModel: /-command autocomplete popup
    askprompt/
      askprompt.go     AskBatchModel: batched ask-user prompts with choice/text/confirm UI
    bridge/
      bridge.go        Goroutine bridge: converts engine events and chat messages to tea.Msg
    tty/
      drain.go         StaleEscapeFilter: suppresses stale terminal escape sequences
      flush_unix.go    FlushStdinBuffer (BSD/macOS): drains leftover stdin data
      flush_other.go   FlushStdinBuffer (no-op on non-BSD platforms)
```

## Architecture

### Startup Sequence

`main()` in `main.go` performs the following steps:

1. **Flag parsing** -- `--config`, `--shelly-dir`, `--env`, `--agent` are parsed via the standard `flag` package.
2. **Dotenv loading** -- `loadDotEnv()` loads the `.env` file (missing files are silently ignored).
3. **Config resolution** -- `resolveConfigPath()` checks, in order: the explicit `--config` flag, `<shellyDir>/config.yaml`, then `shelly.yaml` as a legacy fallback.
4. **Engine creation** -- `engine.LoadConfig()` parses the YAML file, then `engine.New()` initializes the full engine (providers, adapters, agents, MCP clients, skills, project context, `.shelly/` directory). A `StatusFunc` callback prints initialization progress to stderr.
5. **Session creation** -- `eng.NewSession(agentName)` creates a session bound to the entry agent (or the one specified by `--agent`).
6. **Background detection** -- `lipgloss.HasDarkBackground()` probes the terminal's background color before Bubbletea starts, storing the result in `format.IsDarkBG`. This prevents glamour from issuing its own OSC 11 query which would race with the input handling.
7. **Stdin flush** -- `tty.FlushStdinBuffer()` drains any leftover escape-sequence responses from stdin so they do not leak into the textarea as garbage characters.
8. **Stale escape filter** -- A `tea.WithFilter` callback is installed that suppresses all user-input messages while the input box is disabled (the first 200ms after startup), except for Ctrl+C.
9. **Program launch** -- `tea.NewProgram(model)` is created and run. A goroutine sends `ProgramReadyMsg` to the model so it can start the bridge goroutines.

### State Machine

`AppModel` in `internal/app/app.go` manages three states:

| State | Meaning |
|-------|---------|
| `StateIdle` | Waiting for user input. |
| `StateProcessing` | An `sess.Send()` call is in flight. |
| `StateAskUser` | The ask-user prompt (AskBatchModel) is active and awaiting user answers. |

Transitions:

- **Idle -> Processing**: User submits a message via `InputSubmitMsg`.
- **Processing -> Idle**: `SendCompleteMsg` arrives (the agent finished its ReAct loop).
- **Processing -> AskUser**: An `AskUserMsg` arrives from the bridge while processing.
- **AskUser -> Processing**: The user answers all batched questions (`AskBatchAnsweredMsg`).

The user can interrupt a running `Send()` call by pressing Escape, which cancels the per-send context (`cancelSend`).

### Bridge (Event Forwarding)

`internal/bridge/bridge.go` spawns two goroutines when `ProgramReadyMsg` arrives:

1. **Event watcher** -- Subscribes to the `engine.EventBus` and converts engine events to bubbletea messages:
   - `EventAskUser` -> `msgs.AskUserMsg` (carries an `ask.Question` from `pkg/codingtoolbox/ask`)
   - `EventAgentStart` -> `msgs.AgentStartMsg` (carries agent name, display prefix, parent agent name)
   - `EventAgentEnd` -> `msgs.AgentEndMsg`

2. **Chat watcher** -- Calls `chat.Wait()` in a loop, forwarding new `message.Message` values as `msgs.ChatMessageMsg`. It uses a cursor-based `chat.Since()` pattern so it catches all messages, even when the context is cancelled.

Both goroutines only call `p.Send()` -- they never mutate model state directly. The returned cancel function stops both goroutines and waits for them to exit before returning, ensuring no stale messages arrive after cancellation.

## Integration with `pkg/` Packages

### `pkg/engine/`

The TUI is the primary consumer of the engine. In `main.go`:

- `engine.LoadConfig(path)` parses the YAML configuration file into an `engine.Config`.
- `engine.New(ctx, cfg)` creates the `Engine`, which internally wires providers, model adapters, agents, MCP clients, skills, the `.shelly/` directory, project context, and the event bus.
- `eng.NewSession(agentName)` creates a `Session` bound to a specific agent. The session provides `Send(ctx, text)` for sending user messages and `Respond(questionID, answer)` for answering ask-user prompts.
- `eng.Events()` returns the `*EventBus` used by the bridge to subscribe to real-time events.
- `eng.RemoveSession(id)` is called on `/clear` to tear down the old session before creating a fresh one.

### `pkg/chats/`

The chat data model flows through the TUI in several ways:

- `chat.Chat` (from `pkg/chats/chat`) -- The bridge's chat watcher calls `c.Wait()` and `c.Since()` to detect new messages and forward them as `msgs.ChatMessageMsg`.
- `message.Message` (from `pkg/chats/message`) -- Each `ChatMessageMsg` wraps a `message.Message`. The `ChatViewModel.AddMessage()` method dispatches on `msg.Role` (from `pkg/chats/role`): system and user messages are ignored (user messages are rendered separately), assistant messages are processed for tool calls or final answers, and tool messages complete pending tool call items.
- `content.ToolCall` and `content.ToolResult` (from `pkg/chats/content`) -- Assistant messages are inspected via `msg.ToolCalls()` to detect tool invocations. Tool result messages are iterated with `msg.Parts` to find `content.ToolResult` values that match pending calls by `ToolCallID`.

### `pkg/agent/`

The TUI does not import `pkg/agent/` directly in most files. However:

- `bridge.go` imports `pkg/agent` to access `agent.AgentEventData`, a struct carried in `EventAgentStart` and `EventAgentEnd` events that includes the display `Prefix` and `Parent` agent name. This data drives the nested sub-agent display in the chat view.
- Agent lifecycle (the ReAct loop, delegation, middleware, effects) is managed entirely by the engine and session; the TUI only observes it through the event bus and chat messages.

### `pkg/codingtoolbox/` (specifically `pkg/codingtoolbox/ask`)

The `ask_user` tool is the mechanism by which agents request information from the user. The bridge converts `EventAskUser` events (carrying `ask.Question` from `pkg/codingtoolbox/ask`) into `msgs.AskUserMsg`, which triggers the `AskBatchModel` UI. After the user answers, `sess.Respond(questionID, answer)` sends the response back to the agent.

### `pkg/modeladapter/`

In `app.go`, `updateTokenCounter()` attempts to cast `sess.Completer()` to `modeladapter.UsageReporter` to access the `UsageTracker()` and display cumulative token counts (input + output) below the input box.

### `pkg/tools/`, `pkg/providers/`, `pkg/shellydir/`, `pkg/projectctx/`, `pkg/skill/`, `pkg/state/`, `pkg/tasks/`

These packages are not imported directly by `cmd/shelly/`. They are wired internally by `pkg/engine/` during `engine.New()`. The TUI interacts with their effects indirectly:

- **`pkg/tools/`** -- Tool calls and results appear as chat messages, rendered by the `ToolCallItem` and `ToolGroupItem` display items.
- **`pkg/shellydir/`** -- Directory initialization happens inside `engine.New()`.
- **`pkg/projectctx/`** -- Project context (`.md` files, structural index) is loaded by the engine and injected into agent system prompts.
- **`pkg/skill/`** -- Skills are loaded by the engine; `load_skill` tool calls appear in the TUI as tool call items with the label `Loading skill "name"`.
- **`pkg/state/` and `pkg/tasks/`** -- State and task tool calls (e.g., `_state_get`, `_tasks_create`) are displayed via the suffix-based formatters in `toolformat.go`.

### Tool Call Formatting

`internal/format/toolformat.go` contains two registries that produce human-readable labels for tool invocations:

- `ToolFormatters` -- Maps exact tool names (e.g., `fs_read`, `exec_run`, `git_commit`, `delegate`, `load_skill`) to formatter functions.
- `SuffixFormatters` -- Matches dynamically-namespaced tools by suffix (e.g., `_state_get`, `_tasks_create`) for state and task board tools.

Unknown or MCP tools fall through to a generic format showing the tool name and truncated arguments.

## TUI Structure

### Display Items

All visual elements in the chat view implement the `DisplayItem` interface:

```go
type DisplayItem interface {
    View(width int) string
    IsLive() bool
    Kind() string
}
```

Concrete types:

| Type | Kind | Description |
|------|------|-------------|
| `ThinkingItem` | `"thinking"` | Agent reasoning text, rendered as markdown with a tree connector. |
| `ToolCallItem` | `"tool_call"` | A single tool invocation with spinner (live) or result (completed). |
| `ToolGroupItem` | `"tool_group"` | Groups parallel calls of the same tool into a collapsible tree. |
| `SubAgentItem` | `"sub_agent"` | Wraps a nested `AgentContainer` for sub-agent display with windowing. |
| `PlanItem` | `"plan"` | Agent plan text (used when the agent prefix is the plan emoji). |

### AgentContainer

`AgentContainer` accumulates `DisplayItem` values for a single agent during its ReAct loop. It maintains:

- A `CallIndex` map for O(1) lookup of pending `ToolCallItem` values by call ID.
- A `MaxShow` window (0 = show all for top-level, >0 = windowed for sub-agents).
- Spinner frame state that advances every 100ms tick.

When an agent finishes (`EndAgent`), its container is collapsed into a one-line summary (agent name + elapsed time) and committed to the scrollback buffer.

### ChatViewModel

The root display component. It wraps a `viewport.Model` (from `charm.land/bubbles/v2/viewport`) and manages:

- `Committed` -- A `strings.Builder` holding all finalized content (user messages, agent answers, collapsed agent summaries, errors).
- `agents` / `subAgents` -- Maps of active `AgentContainer` instances. Sub-agents are nested inside their parent's items list.
- `agentOrder` -- Preserves the arrival order of top-level agents for consistent rendering.

The `View()` method combines committed content with live agent views, sets the viewport content, and scrolls to the bottom.

### InputModel

Wraps a `textarea.Model` with:

- Auto-growing height (1-5 lines) based on visual line count (accounting for soft wraps).
- A `FilePickerModel` that activates on `@` input, walks the working directory, and provides filtered file path autocomplete.
- A `CmdPickerModel` that activates on `/` at the start of input and offers command autocomplete (`/help`, `/clear`, `/exit`).
- A token counter displayed below the input box when no picker is active.

### AskBatchModel

Handles agent-initiated `ask_user` prompts. Questions are batched within a 200ms window and presented as a tabbed interface:

- Each tab corresponds to one question, supporting free-text input, single-select choice lists, or multi-select with checkboxes.
- A final "Confirm" tab summarizes all answers and offers Yes/No/custom options.
- Escape dismisses the entire prompt (sending a rejection to the agent).

### Styles

`internal/styles/styles.go` defines a centralized color palette inspired by the GitHub terminal light theme:

- `ColorFg`, `ColorMuted`, `ColorAccent` (blue), `ColorError` (red), `ColorSuccess` (green), `ColorWarning` (amber), `ColorMagenta` (purple).
- Pre-built styles for user messages, tool calls, sub-agents, spinners, borders, pickers, and ask prompts.
- Tree-drawing characters (`TreeCorner`, `TreePipe`, `TreeTee`) used throughout the display hierarchy.

## Key Bindings

| Key | Context | Action |
|-----|---------|--------|
| `Enter` | Input idle | Submit message |
| `Shift+Enter` / `Alt+Enter` | Input | Insert newline |
| `Escape` | Processing | Cancel current agent run |
| `Escape` | Picker open | Close the picker |
| `Escape` | Ask prompt | Dismiss questions (reject) |
| `Ctrl+C` | Always | Quit the application |
| `@` | Input | Open file picker |
| `/` | Input (empty) | Open command picker |
| `Up` / `Down` | Picker or ask prompt | Navigate options |
| `Tab` / `Enter` | Picker | Select highlighted entry |
| `Left` / `Right` | Ask prompt | Switch between question tabs |
| `Space` | Ask prompt (multi-select) | Toggle checkbox |

## Slash Commands

| Command | Action |
|---------|--------|
| `/help` | Display available commands and keyboard shortcuts. |
| `/clear` | Tear down the current session and start a fresh one. |
| `/quit` or `/exit` | Exit the application. |

## Configuration Flow

1. The user provides a YAML config file (via `--config`, `.shelly/config.yaml`, or `shelly.yaml`).
2. `engine.LoadConfig()` parses the file into an `engine.Config` struct.
3. `main.go` sets `cfg.ShellyDir` from the `--shelly-dir` flag and attaches a `StatusFunc` for progress output.
4. `engine.New(ctx, cfg)` uses the config to initialize all subsystems (providers, agents, tools, MCP, skills, project context).
5. `eng.NewSession(agentName)` binds a session to the configured (or flag-overridden) entry agent.

## Event Handling

The `engine.EventBus` uses a pub/sub model. The bridge subscribes with a buffer of 64 events and processes three event kinds:

| Event | Bridge Action | TUI Effect |
|-------|---------------|------------|
| `EventAgentStart` | Sends `AgentStartMsg` | Creates an `AgentContainer` (or nested `SubAgentItem`) with the agent's prefix. |
| `EventAgentEnd` | Sends `AgentEndMsg` | Collapses the agent container into a summary and commits it to scrollback. |
| `EventAskUser` | Sends `AskUserMsg` | Queues the question; after a 200ms batching window, opens the `AskBatchModel`. |

Chat messages are forwarded separately by the chat watcher goroutine as `ChatMessageMsg`, which the `ChatViewModel` routes by role (assistant messages create display items; tool messages complete pending calls).

## Terminal Compatibility

The `tty` package handles platform-specific terminal quirks:

- **BSD/macOS** (`flush_unix.go`): Temporarily puts stdin into raw non-blocking mode to drain leftover escape-sequence responses from prior terminal queries.
- **Other platforms** (`flush_other.go`): No-op.
- **StaleEscapeFilter** (`drain.go`): A `tea.WithFilter` callback that suppresses all input events during the post-startup drain window (first 200ms), except Ctrl+C. This prevents late-arriving OSC 11 responses from appearing as garbage in the textarea.
