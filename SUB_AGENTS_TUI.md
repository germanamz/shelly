# Sub-Agents TUI Navigation — Design Plan

## Problem

Sub-agent activity is currently rendered inline within parent agent containers. As delegation depth and parallelism increase, this becomes hard to follow. Users need the ability to navigate into individual sub-agent chats and view their full history independently.

## Solution Overview

Introduce an **interactive menu bar** between the chat viewport and user input that lets users browse and select running sub-agents. Selecting a sub-agent switches the chat view to display that agent's full message history. `Esc` navigates back to the parent/root view.

## Architecture

### 1. Reusable Interactive Menu Component

**Package:** `cmd/shelly/internal/menubar/`

A generic horizontal menu bar component following bubbletea's Model/Update/View pattern. Designed for reuse — not coupled to sub-agents.

```go
type Item struct {
    ID    string
    Label string   // display text, e.g. "Subagents (3)"
    Badge int      // optional count badge (0 = hidden)
}

type Model struct {
    items    []Item
    cursor   int       // which item is focused (-1 = none/inactive)
    active   bool      // whether the menu bar has focus
    width    int
}
```

**Behavior:**
- Renders as a single horizontal line of items separated by dividers
- Left/Right arrows move the cursor between items
- Enter/Space activates the focused item (emits `MenuItemSelectedMsg{ID}`)
- Esc deactivates the menu (returns focus upward)
- Visually highlights the focused item using accent color
- Items can be updated dynamically (add/remove/update labels and badges)

**Messages emitted:**
- `MenuItemSelectedMsg{ID string}` — user selected an item
- `MenuDeactivatedMsg{}` — user pressed Esc, menu released focus

**Messages consumed:**
- `tea.KeyPressMsg` — arrow keys, enter, escape
- `MenuSetItemsMsg{Items []Item}` — update item list
- `MenuSetWidthMsg{Width int}` — resize

### 2a. Panel Component (chrome layer)

**Package:** `cmd/shelly/internal/panel/`

A generic container component that provides the visual chrome — border, title, sizing, open/close lifecycle. It wraps any inner content passed as a string to `View(content string)`. Not coupled to lists or any specific content type.

```go
type Model struct {
    title      string          // panel header, e.g. "Subagents", "Tasks"
    active     bool            // whether the panel is open/visible
    width      int
    height     int             // max height (content may be shorter)
    panelID    string          // identifier for distinguishing panels in messages
}
```

**Behavior:**
- Renders a bordered box with a title header via `View(content string)` — the consumer passes pre-rendered content as a string, and the panel wraps it with border and title. The panel only handles chrome, not content.
- The chat viewport height shrinks by the panel's height when active, restored when closed
- **Does not handle Esc itself.** The panel is a passive chrome wrapper — Esc handling is the responsibility of the consumer or the inner content component. This avoids conflicts when inner components (e.g., list in selectable mode) also respond to Esc. The innermost focused component always wins the Esc race.
- Manages open/close state via explicit messages from the consumer; consumers embed `panel.Model` and delegate chrome rendering to it
- **Error display:** When the consumer encounters an error (e.g., data API failure), it passes an error string to `View()` instead of normal content. The panel renders this as a centered, styled error message within the bordered box.

**Messages emitted:**
- `PanelClosedMsg{PanelID string}` — emitted by consumer when it decides to close (not by panel itself)

**Messages consumed:**
- `PanelSetSizeMsg{Width, Height int}` — resize

### 2b. List Component (item rendering + optional selection)

**Package:** `cmd/shelly/internal/list/`

A generic vertical list that renders items with status icons, scrolling, and optional indentation. Supports two modes: **read-only** (scroll only, no cursor — used by task browser) and **selectable** (cursor navigation + item selection — used by sub-agent browser). The mode is configured at creation time.

```go
type Item struct {
    ID     string
    Label  string   // primary text
    Detail string   // secondary text (right-aligned or below, e.g. provider label)
    Status Status   // determines icon: Running (spinner), Done (checkmark), Pending (circle)
    Color  string   // optional hex color for the label (matches AgentContainer.Color convention)
    Indent int      // nesting depth (0 = root), rendered as tree indentation
}

type Status int
const (
    StatusNone       Status = iota
    StatusPending           // renders: circle
    StatusRunning           // renders: spinner (animated)
    StatusDone              // renders: checkmark (dimmed)
    StatusFailed            // renders: X (error color)
)

type Model struct {
    items      []Item
    width      int
    height     int        // max visible rows before scrolling
    scrollTop  int        // scroll offset
    spinnerIdx int
    selectable bool       // false = read-only (scroll only), true = cursor + selection
    cursor     int        // which item is focused (only used when selectable)
}
```

**Behavior (common):**
- Renders a vertical list of items (no border — that's the panel's job)
- Each item renders: status icon + indented label + detail text
- Spinner icon animates on `TickMsg` for `StatusRunning` items
- Items can be updated dynamically
- Empty state: renders centered "No items" message
- Exposes `View()` returning the rendered item lines (consumers wrap this in a `panel.Model` or use directly)
- **Per-line rendering:** Exposes `RenderLine(index int) string` returning a single rendered item line. In selectable mode, the focused line is wrapped with cursor highlight styling. `View()` is implemented in terms of `RenderLine()`.

**Behavior (read-only mode, `selectable=false`):**
- Up/Down arrows scroll the viewport when items exceed visible height (no item highlight)

**Behavior (selectable mode, `selectable=true`):**
- Up/Down arrows move the cursor between items (scrolls to keep cursor visible)
- Enter selects the focused item (emits `ListItemSelectedMsg{ItemID}`)
- Esc emits `ListDeactivatedMsg{}` — consumer decides whether to close the panel
- Items can be updated dynamically without losing cursor position (match by ID; clamp if list shrinks)
- Cursor highlight uses accent color background or indicator on the focused line

**Messages emitted (selectable mode only):**
- `ListItemSelectedMsg{PanelID, ItemID string}` — user selected an item
- `ListDeactivatedMsg{}` — user pressed Esc

**Messages consumed:**
- `tea.KeyPressMsg` — Up/Down (scroll or cursor), Enter (selection, selectable only), Esc (deactivate, selectable only)
- `ListSetItemsMsg{Items []Item}` — replace item list
- `ListSetSizeMsg{Width, Height int}` — resize
- `msgs.TickMsg` — advance spinner

### 3. Sub-Agent Data API

**Modified package:** `cmd/shelly/internal/chatview/`

Before the sub-agent browser can populate its list, `ChatViewModel` must expose sub-agent data through a public API. This avoids coupling the adapter to private fields.

**New public types and methods on `ChatViewModel`:**
```go
type SubAgentInfo struct {
    ID         string
    Label      string   // agent instance name
    Provider   string   // provider label (e.g. "anthropic/claude-sonnet-4")
    Status     string   // "running" or "done" or "failed"
    Color      string   // hex color from colorRegistry
    ParentID   string   // parent agent ID ("" for top-level sub-agents)
    Depth      int      // nesting depth (0 = direct child of root)
}

func (m ChatViewModel) SubAgents() []SubAgentInfo { ... }
func (m ChatViewModel) FindContainer(agentID string) *AgentContainer { ... }
```

`SubAgents()` returns only currently running sub-agents (completed agents are disposed — see Section 6). `FindContainer()` resolves an agent ID to its container pointer (used by Phase 6 for view stack navigation).

**Thread safety:** `SubAgents()` and `FindContainer()` are called from the bubbletea `Update` goroutine only (single-threaded by bubbletea's design). Agent start/end events arrive as `tea.Msg` values processed sequentially in `Update`. The underlying sub-agent tracking maps are only mutated inside `Update` handlers. No mutex is needed — bubbletea's message loop serializes all access. The event batching (Section 12) further guarantees that multiple start/end events within a tick are applied as a single atomic list update before any subsequent `SubAgents()` call.

**Note:** Per-agent usage data is delivered via push-based `AgentUsageUpdateMsg` from the bridge (see Section 9), not via callbacks — so `SubAgents()` does not expose usage data and no cross-goroutine reads occur.

### 4. Sub-Agent Browser (consumer of panel + list)

**Package:** `cmd/shelly/internal/subagentpanel/`

Thin adapter that composes `panel.Model` + `list.Model` (selectable mode) using the `ChatViewModel.SubAgents()` API.

**Responsibilities:**
- On activation, calls `ChatViewModel.SubAgents()` and builds `list.Item` entries
- Maps `SubAgentInfo.Depth` to `list.Item.Indent`
- Maps agent status (running/done) to `list.Status`
- Assigns `Color` from `SubAgentInfo.Color`
- Sets `Detail` to the provider label
- On `ListItemSelectedMsg` with its panel ID: emits `SubAgentSelectedMsg{AgentID}`
- Refreshes item list on `AgentStartMsg`/`AgentEndMsg` while panel is open
- On error from `SubAgents()` API, passes error string to `panel.View()` for styled error display

### 5. Task Browser (consumer of panel + list)

**Package:** `cmd/shelly/internal/taskpanel/` (refactored)

Refactors the existing `TaskPanelModel` to compose `panel.Model` + `list.Model` internally for a consistent visual language.

**Responsibilities:**
- On activation from menu bar, populates `list.Item` entries from task board
- Maps `tasks.StatusPending` to `list.StatusPending`, `StatusInProgress` to `list.StatusRunning`, etc.
- Sets `Detail` to assignee name (if any)
- Sorts items: pending first, in-progress second, completed last (same as current)
- **Read-only** — no selection, no cursor, scroll-only. Users navigate to agents via the sub-agents panel
- The non-interactive summary line (current behavior: "Tasks 3 pending 2 in-progress") moves to the menu bar badge; the panel itself becomes the detailed view
- Refreshes on `TasksChangedMsg` while panel is open

### 6. Chat View: Agent-Scoped Rendering & Agent Lifecycle

**Modified package:** `cmd/shelly/internal/chatview/`

The chat view gains the concept of a "viewed agent" — which agent's history to display.

#### Agent Disposal on Completion

When a child agent finishes its work and signals the parent:
- The engine disposes the child agent (including its owned completer and provider — see Section 9)
- The parent's inline sub-agent container for this child is removed entirely — sub-agent containers are no longer rendered inline in the parent view
- Instead, the parent retains only a **single summary line** for the child: agent name, status (done/failed), and a one-line result summary
- Users who want to see the full child history must navigate to it via the sub-agent browser (if still running) or the view stack (if pinned)
- The child's detailed work is only accessible through explicit navigation, not by scrolling the parent

This means the sub-agent list only contains **currently running** agents. Completed agents leave no container overhead. The sub-agent browser replaces the previous inline container rendering entirely.

#### View Stack

**New fields on `ChatViewModel`:**
```go
type viewStackEntry struct {
    AgentID      string
    Container    *AgentContainer // pinned reference — survives map deletion on agent end
    ScrollOffset int             // preserved viewport scroll position for this level
}

type ChatViewModel struct {
    // ... existing fields ...
    viewedAgent   string           // "" = root view (default), or agent instance name
    viewStack     []viewStackEntry // navigation stack for back functionality (capped at 32, cleaned up on agent end)
}
```

**Why pinned container pointers:** When an agent ends, the engine disposes it. If the user is viewing that agent at that moment, string-based lookups would fail. By storing the `*AgentContainer` pointer alongside the ID, the view stack remains valid — the container's data is still in memory until the user navigates away.

**Scroll position preservation:** Each view stack entry stores the viewport's `ScrollOffset` at the time the user navigates away. When navigating back (`Esc`), the viewport restores the saved scroll position — but if new content has arrived in the parent view since the user left, the viewport auto-scrolls to the bottom instead, so the user sees the latest state. When pushing a new entry (navigating into a sub-agent), the current scroll offset is captured on the top-of-stack entry before the new entry is pushed. New entries always start scrolled to the bottom.

**Memory management:** The view stack is capped at a maximum depth of 32 entries. This is well above any practical nesting depth. If a push would exceed the cap, the navigation is rejected with a visual indication ("Maximum navigation depth reached"). Entries are cleaned up proactively: when an agent finishes (disposed), its entry is removed from the view stack if the user is not currently viewing it. If the user *is* viewing a completed agent, the pinned pointer keeps the data alive until the user navigates away, at which point the entry is removed. This prevents stale entries from accumulating.

**Behavior:**
- When `viewedAgent == ""` (default): render exactly as today — committed lines + top-level agent containers with inline sub-agents (showing only spawn/init messages for completed children)
- When `viewedAgent == "<agent-id>"`: render only that agent's container with `MaxShow=0` (show ALL items, no windowing), plus a breadcrumb header showing the navigation path
- Breadcrumb example: `← root › orchestrator-plan-42 › coder-implement-auth-7`
- **Breadcrumb truncation:** When the breadcrumb exceeds terminal width, middle segments are collapsed to `…`. The first segment (`← root`) and last segment (current agent) are always shown in full. Intermediate segments are collapsed right-to-left until the breadcrumb fits. If even root + current exceeds width, the current agent name is truncated with `…` suffix.
- `clear()` resets viewStack and viewedAgent to root (handles `/compact` while viewing a sub-agent)

**New messages:**
- `ChatViewFocusAgentMsg{AgentID string}` — switch to viewing a specific agent
- `ChatViewNavigateBackMsg{}` — pop viewStack, return to previous view

**Rendering in agent-scoped mode:**
- The targeted `AgentContainer` renders with `MaxShow=0` (full history)
- Sub-agent items within it still render collapsed/windowed (user can drill into those too)
- Breadcrumb renders **above the input area, below the panel** (not inside the viewport, so it doesn't scroll away). It sits in the same visual zone as the menu bar.
- Viewport height is reduced by 1 when breadcrumb is visible; `ChatViewModel` exposes `HeaderHeight() int` for `recalcViewportHeight`
- Each breadcrumb segment uses the agent's color from `colorRegistry`, with `›` separators in dim style; the last segment is bold
- **Completed agent segments** are rendered with strikethrough styling to indicate the agent is no longer running — the user can still view the pinned history but cannot send input to it
- The leading `←` serves as a visual back-navigation hint (clickable for mouse users; keyboard users press `Esc`)

```go
// ChatViewModel.View() renders only the viewport content (agent-scoped or root).
// Breadcrumb is rendered separately via RenderBreadcrumb() and composed by AppModel.
func (m ChatViewModel) View() string {
    return m.viewport.View()
}

func (m ChatViewModel) RenderBreadcrumb() string { ... } // segments with agent colors, bold last, strikethrough completed
func (m ChatViewModel) ViewedAgent() string { return m.viewedAgent }
```

### 7. Focus Management in AppModel

**Modified package:** `cmd/shelly/internal/app/`

The `AppModel` currently manages focus between input, ask-prompt, and pickers via nil checks and `Active` flags. This extends to include the menu bar and list panels.

**New fields on `AppModel`:**
```go
type ActivePanel int
const (
    PanelNone ActivePanel = iota
    PanelSubAgents
    PanelTasks
)

type AppModel struct {
    // ... existing fields ...
    menuBar       menubar.Model
    subAgentPanel subagentpanel.Model
    activePanel   ActivePanel       // which list panel is open (PanelNone = none)
    menuFocused   bool              // whether the menu bar has keyboard focus
}
```

**Why `activePanel` instead of a single `FocusListPanel`:** Two list panels (subagents, tasks) share the focus state. `activePanel` tracks which one is currently shown, so key routing in `handleKey` dispatches to the right panel. Opening one panel auto-closes the other.

**Navigation flow:**
```
                    Ctrl+B (toggle)
    [Input] ─────────────────────────────> [MenuBar]
       ^                                      │
       │ Ctrl+B/Esc                  Enter on "Subagents" or "Tasks"
       │                                      │
       │                                      v
       └──────────────── Esc ──────── [ListPanel (shared)]
                                              │
                                        Enter on item
                                              │
                                              v
                                     [ChatView shows
                                      agent history]
                                              │
                                        Esc
                                              │
                                              v
                                     [Previous view
                                      (pop viewStack)]
```

**Key routing priority in `handleKey` (extends existing cascade):**
1. Ctrl+C → quit (unchanged)
2. PageUp/Down → viewport scroll (unchanged)
3. Session picker active → forward (unchanged)
4. Ask prompt active → forward (unchanged)
5. **List panel active (`activePanel != PanelNone`)** → forward to active panel (NEW)
6. **Menu bar focused (`menuFocused`)** → forward to menu bar (NEW)
7. `Esc` → navigate back in sub-agent view stack (NEW)
8. Escape priority chain (unchanged)
9. Default → input box (unchanged)

**Key bindings (when input is focused):**
- `Ctrl+B`: toggle focus to/from menu bar (mnemonic: **b**rowse). Does not conflict with common terminal keybindings. When the menu bar is focused, `Ctrl+B` returns focus to input.
- The menu bar items ("Subagents", "Tasks") are also accessible as `/subagents` and `/tasks` slash commands, providing a keyboard-only alternative for users whose terminal intercepts `Ctrl+B`.

**Key bindings (when viewing a sub-agent):**
- `Esc`: navigate back one level in viewStack (only when input is empty and no panel/menu is focused — fits the existing Esc priority chain). If input has text, Esc clears it as today.
- The breadcrumb's `←` segment is also clickable (mouse support)
- The input sends messages to the **currently viewed agent** — if viewing a sub-agent, the message is routed to that agent

**Terminal compatibility:** `Alt+` keybindings are avoided because many terminals (iTerm2, tmux, etc.) intercept them for word movement or other functions. `Ctrl+B` is free in most terminals (tmux users who haven't rebound it are the exception — the slash commands serve as their fallback).

**Back navigation priority within Esc chain:** When the user presses Esc with empty input: (1) if a list panel is open, close it; (2) else if menu bar is focused, unfocus it; (3) else if viewing a sub-agent, navigate back one level; (4) else, existing Esc behavior. This means Esc always does "the most local close" first.

**Interaction with existing overlays:** When ask-prompt or session-picker is active, they take priority (steps 3-4 above). The menu bar and list panel are unreachable until those dismiss — no new edge cases.

### 8. Menu Bar & List Panel Integration Points

**Layout in `AppModel.View()`:**

```
┌─────────────────────────────────────────┐
│  Chat messages / Agent containers       │  ← chatView (viewport)
│                                         │
│                                         │
├─────────────────────────────────────────┤
│  ┌ Subagents ─────────────────────────┐ │  ← listPanel (conditional, inserted when activePanel != PanelNone)
│  │  ◉ orchestrator-plan-42            │ │     Viewport shrinks by panel height; restored when closed.
│  │  ◉ coder-auth-7                    │ │     Only shows running agents (completed ones are disposed).
│  └────────────────────────────────────┘ │
├─────────────────────────────────────────┤
│  ← root › orchestrator-42 › coder-7    │  ← breadcrumb (conditional, only when viewing sub-agent)
├─────────────────────────────────────────┤
│  [Subagents (3)]  [Tasks (2)]           │  ← menuBar (between panel/breadcrumb and input)
├─────────────────────────────────────────┤
│  > Type a message...                    │  ← inputBox (or askPrompt / sessionPicker)
├─────────────────────────────────────────┤
│  model: claude-sonnet-4 │ cost: $0.12   │  ← statusBar
└─────────────────────────────────────────┘
```

**View composition:**
```go
parts := []string{m.chatView.View()}
if m.activePanel != PanelNone {
    parts = append(parts, m.activeListPanelView())
}
if m.chatView.ViewedAgent() != "" {
    parts = append(parts, m.chatView.RenderBreadcrumb())  // above menu bar, below panel
}
parts = append(parts, m.menuBar.View())  // between breadcrumb and input
// ... inputBox / askPrompt / sessionPicker (existing switch)
parts = append(parts, m.statusBar())
```

**Menu bar visibility:** The menu bar is **hidden by default** and only renders once the first sub-agent spawns or the first task appears. This avoids wasting a line in solo-agent sessions where the menu bar is never useful. The one-time height jitter when it first appears is acceptable. Once visible, the menu bar persists for the rest of the session.

**Lazy item creation:** Menu bar items are added individually when their category first becomes relevant — the "Subagents" item appears on the first `AgentStartMsg` with a parent, and the "Tasks" item appears on the first `TasksChangedMsg`. If a user only creates tasks but never spawns sub-agents, only the "Tasks" item is shown (and vice versa). Once an item is added, it persists for the session — dimmed when its badge is 0, but never removed. This prevents a useless "Subagents (0)" item from appearing in task-only sessions.

**Panel height policy:** When a list panel is open, its height is `min(len(items) + 2, 12)` rows — the `+2` accounts for the top and bottom border lines, and the cap of 12 prevents the panel from dominating the screen. When the panel has 0 items (empty state), it renders at a fixed height of 3 (borders + "No items" message). The panel's `height` field stores this computed value; it is recalculated on every `ListSetItemsMsg`.

**Impact on `recalcViewportHeight`:** The existing calculation already accounts for variable status bar height (task panel lines). The same pattern applies — subtract menu bar height (0 when hidden, 1 when visible) and list panel height (0 when closed, or computed height when open) from available space.

### 9. Agent-Level Usage Tracking

**Modified packages:** `pkg/engine/`, `pkg/agent/`, `pkg/modeladapter/`

Each agent owns its own usage tracker while sharing provider instances. Providers are reusable resources (connection pooling, auth, rate limiting) — duplicating them per agent would waste connections and complicate rate limit handling. Instead, a thin **usage-tracking layer** wraps the shared completer to attribute token usage to individual agents.

**Design:**
- Providers and completers remain shared/pooled — the engine creates them once per provider config
- When an agent is spawned, the engine wraps the shared completer in an **`AgentUsageCompleter`** that intercepts completion responses and attributes usage to the agent's own `UsageTracker`
- The `AgentUsageCompleter` is a thin decorator: delegates completion to the shared completer, captures usage from the response, records it in the agent-scoped tracker
- Each agent's `UsageTracker` is independent — tracks only that agent's token usage and cost
- When the agent finishes, the engine captures the final usage snapshot and the `AgentUsageCompleter` wrapper is GC'd (the underlying shared completer lives on)

**Lifecycle:**
```
Agent spawned → Engine wraps shared completer in AgentUsageCompleter
             → Engine registers wrapper in bridge's active-agent tracker
             → Agent runs
             → Agent finishes → Engine captures final usage snapshot
             → Engine deregisters wrapper from bridge's active-agent tracker
             → AgentUsageCompleter wrapper becomes unreferenced → GC'd
    (shared provider + completer remain alive for other agents)
```

**Bridge deregistration:** The bridge maintains a `map[string]*AgentUsageCompleter` of active agent wrappers for tick-based usage reads. On `agent_end`, the engine removes the entry from this map *before* emitting `AgentEndMsg` (which carries the final usage snapshot). This ensures the bridge holds no reference to the wrapper after the agent finishes, allowing GC. The deregistration and final snapshot capture happen atomically within the engine event loop — no race.

**Aggregated session usage:** The session/engine level maintains a running total by summing usage reports from each agent as they complete. This is independent of individual agent trackers — the engine captures the agent's final usage on `agent_end` and adds it to the session total.

**Push-based usage delivery to TUI:** Per-agent usage is delivered to the TUI via push-based messages from the bridge — not via callbacks or closures. The bridge emits `AgentUsageUpdateMsg{AgentID, Usage}` on each tick for running agents. This keeps the TUI completely decoupled from engine internals — no cross-goroutine reads, no retained references to completers. The bridge goroutine reads the `AgentUsageCompleter`'s tracker (same goroutine that owns it) and sends a snapshot as a `tea.Msg`. On `AgentEndMsg`, a final usage snapshot is included directly in the message.

**Thread safety:** Each `AgentUsageCompleter` writes to its own `UsageTracker`, so there are no concurrent access concerns on per-agent state. The bridge reads usage from the same goroutine that owns the tracker (engine event loop), then sends a snapshot to the TUI via `tea.Msg`. The shared completer handles its own thread safety internally. The session-level usage aggregation is protected by the engine's existing synchronization. No cross-goroutine reads occur between the TUI and engine.

### 10. Context-Aware Status Bar

**Modified:** `AppModel.statusBar()` in `cmd/shelly/internal/app/`

The status bar becomes context-aware — it shows information about the currently viewed agent, plus keyboard hints when the menu bar or list panel is focused.

**Behavior:**
- **Root view** (`viewedAgent == ""`): shows session-level aggregated usage as today — provider label, total tokens, cumulative cost, cache ratio.
- **Sub-agent view** (`viewedAgent != ""`): shows the viewed agent's individual stats — its provider label, its token usage, its cost. A prefix identifies which agent: e.g. `coder-auth-7 | anthropic/claude-sonnet-4 | 12.4k tokens | $0.03`
- **Keyboard hints** (appended when relevant):
  - Menu bar focused: `←→ navigate  ⏎ select  esc back`
  - List panel focused: `↑↓ navigate  ⏎ select  esc close`
  - Viewing sub-agent: `esc back to parent`

**Keyboard discoverability:**
- When the menu bar first appears (first sub-agent spawn), a transient hint is shown in the status bar: `ctrl+b to browse sub-agents`  — dismissed after 5 seconds or on any keypress
- `/help` output includes the sub-agent navigation keybindings (Ctrl+B, etc.)

**Data flow for per-agent usage (push-based):**
- The bridge emits `AgentUsageUpdateMsg{AgentID string, Usage usage.Total}` on each tick for running agents. The bridge reads usage from the `AgentUsageCompleter`'s tracker within the engine event loop goroutine (no cross-goroutine access), then sends a snapshot via `tea.Msg`.
- `AgentEndMsg` includes a final usage snapshot: `Usage *usage.Total` — the bridge captures this from the agent's completer at the moment it emits `agent_end`. This freezes the stats for completed agents.
- `AppModel` maintains `agentUsage map[string]AgentUsageInfo` — updated from `AgentUsageUpdateMsg` (for live agents) and from `AgentEndMsg.Usage` (for completed agents). Completed agent entries are cleaned up when no longer referenced by the view stack.

**Implementation approach:**
- Add `AgentUsageInfo` struct: `{ProviderLabel, ProviderKind, Model, Tokens, Cost, CacheRatio}`
- Add `AgentUsageUpdateMsg{AgentID string, Usage usage.Total}` — emitted by bridge on tick
- `statusBar()` checks `viewedAgent`: if set, render from `agentUsage[viewedAgent]`; if empty, render aggregated session usage as today
- The aggregated session view remains unchanged — it always shows the full session totals regardless of which sub-agents contributed

### 11. Input Routing to Viewed Agent

**Modified packages:** `cmd/shelly/internal/app/`, `pkg/engine/`

When the user submits a message while viewing a sub-agent, the message is routed to that agent — not the root session agent.

**Behavior:**
- `viewedAgent == ""`: input goes to root session agent (current behavior, unchanged)
- `viewedAgent == "<agent-id>"`: input is routed to the viewed agent
- The engine must support sending user messages to a specific agent by ID
- Messages are **queued** — if the agent is mid-completion (waiting for LLM response), the message is enqueued and delivered as the next user turn when the current completion finishes
- **Tool-call-pending state:** If the agent is waiting for tool results (i.e., the last assistant message contains tool_use blocks without corresponding tool_result blocks), the user message is held until all pending tool results are collected. Only then is the user message appended as the next turn. This preserves the tool_use → tool_result pairing that LLMs expect.
- **Compaction interaction:** A queued user message does not interfere with compaction. If compaction triggers while a message is queued, the compaction completes first (summarizing existing history), then the queued message is appended to the post-compaction history as a fresh user turn. The agent's ReAct loop sees a clean state.
- If the viewed agent has already completed, the input is rejected with a visual indication (e.g. "Agent has completed — navigate back to send messages")

**Why this makes sense:** The user is looking at a specific agent's conversation. Sending messages to a different agent would be confusing. The view context and input target should always match.

**Engine-level design for agent-targeted messaging:**

The current ReAct loop in `pkg/agent/agent.go` runs as `agent.Run(ctx, initialMessages)` — a blocking loop that alternates between LLM completion and tool execution. To support injected user messages, the following changes are needed:

1. **Per-agent message inbox:** Each agent gets a `chan chats.Message` inbox (buffered, capacity 1). The engine exposes `SendToAgent(agentID string, msg chats.Message) error` which resolves the agent and sends to its inbox.

2. **ReAct loop integration point:** After each tool execution cycle completes (all tool_results collected and appended), and before calling the next LLM completion, the loop checks the inbox:
   ```go
   // Inside the ReAct loop, after tool results are appended:
   select {
   case userMsg := <-a.inbox:
       a.messages = append(a.messages, userMsg)
       // Continue to next LLM completion with the user message included
   default:
       // No queued message — proceed normally
   }
   ```

3. **Mid-completion queueing:** If the agent is blocked on an LLM completion call, the message sits in the channel until the completion returns. The next loop iteration picks it up after processing any tool calls from the current completion.

4. **Agent registry:** The engine maintains a `map[string]*agent.Agent` of currently running agents (already partially exists for delegation). `SendToAgent` does a lookup, returns error if agent not found or completed.

5. **Ordering guarantees:** The channel is buffered at capacity 1. If the user sends a second message before the first is consumed, the `SendToAgent` call returns an error ("agent has a pending message") and the TUI shows feedback. This prevents message pile-up and keeps the conversation coherent.

6. **Cleanup:** When an agent finishes, the engine closes its inbox channel and removes it from the registry. Subsequent `SendToAgent` calls return a "completed" error.

### 12. Data Flow

```
Engine EventBus
    │
    ├── agent_start (Parent != "") ──> Bridge ──> AgentStartMsg ──> ChatView creates sub-container
    │                                                              ──> AppModel updates menuBar badge
    │
    ├── tick (periodic) ───────────> Bridge ──> AgentUsageUpdateMsg ──> AppModel updates agentUsage[agentID]
    │                                  (bridge reads tracker in      (per running agent, push-based)
    │                                   engine goroutine, sends
    │                                   snapshot via tea.Msg)
    │
    ├── agent_end (Parent != "") ───> Bridge ──> AgentEndMsg ─────> ChatView replaces container with summary line
    │                                  (includes final Usage        ──> AppModel updates menuBar badge
    │                                   snapshot in message)        ──> AppModel freezes agentUsage entry
    │                                                                ──> Engine disposes agent (completer + provider)
    │                                                                ──> View stack cleanup: remove entry if not currently viewed
    │
    └── message_added ──────────────> Bridge ──> ChatMessageMsg ──> ChatView routes to correct container
                                                                    (renders if viewedAgent matches)
```

**Badge count:** `len(activeSubAgents)` — counts only currently running agents. Completed agents are disposed and no longer appear in the list or badge.

**Event batching:** `AgentStartMsg` and `AgentEndMsg` events are batched within a tick window before updating the panel list and badge count. This prevents flickering when agents spawn and complete in rapid succession (sub-second churn) and avoids race conditions from interleaved start/end events. The batch is flushed on each `TickMsg` — any accumulated agent events since the last tick are applied as a single list update.

## Implementation Order

**Vertical slice approach:** Phases are ordered to build and validate component APIs against real data early, rather than building all generic components upfront. Each phase produces a testable increment.

**MVP (Phases 1–8):** Core navigation — sub-agent data API, generic components, menu bar + sub-agent browser wiring, task browser refactor, agent-scoped chat view, agent disposal. This delivers the primary UX goal: users can navigate into sub-agent conversations.

**Enhancements (Phases 9–11):** Per-agent usage tracking, context-aware status bar, and input routing to sub-agents. These depend on the MVP being stable and extend the navigation UX with stats and interaction.

**Polish & Validation (Phases 12–13):** Integration tests and edge case handling. Run after enhancements are complete.

### Phase 1: Sub-Agent Data API ✅
Build the data layer first — everything else depends on being able to query sub-agent state.

1. Add `SubAgentInfo` struct and `SubAgents()` method to `ChatViewModel`
2. Add `FindContainer()` method for resolving agent ID to container pointer
3. Walk sub-agent tracking maps, compute depth from parent chain
4. Only return currently running agents (completed agents are disposed)
5. **Unit tests:**
   - `TestSubAgents_Empty` — no sub-agents returns empty slice
   - `TestSubAgents_FlatList` — direct children have depth 0
   - `TestSubAgents_NestedDepth` — nested agents have correct depth
   - `TestSubAgents_ExcludesCompleted` — completed (disposed) agents not in list
   - `TestFindContainer_Exists` — returns valid pointer
   - `TestFindContainer_NotFound` — returns nil

### Phase 2: Generic Components — Panel + List ✅
Build the reusable components in isolation. No wiring to AppModel yet.

1. Create `cmd/shelly/internal/panel/` package
   - `Model` with title, border, sizing, open/close lifecycle
   - `View(content string)` wraps content in bordered box with title
   - Panel does not handle Esc — inner content or consumer handles close
   - Error display: consumer passes error string to `View()`
   - `README.md` per project conventions
   - Unit tests for resize, error display
2. Create `cmd/shelly/internal/list/` package
   - `Model` with items, scroll offset, spinner, status icons, `selectable` flag
   - Read-only mode (`selectable=false`): scroll only, no cursor
   - Selectable mode (`selectable=true`): cursor navigation, Enter selection, Esc deactivation
   - `View()` renders item lines; `RenderLine(index int) string` for per-line access (cursor highlight in selectable mode)
   - `README.md` per project conventions
   - Unit tests for scroll, dynamic item updates, spinner animation, empty state, RenderLine, cursor navigation, selection, cursor preservation on item update

### Phase 3: Menu Bar Component ✅
Build the menu bar in isolation.

1. Create `cmd/shelly/internal/menubar/` package
   - `Model` with items, cursor, focus, key handling
   - `View()` with lipgloss styling (accent highlight, dimmed inactive, disabled state)
   - `README.md` per project conventions
   - Unit tests for navigation, selection, resize, disabled items

### Phase 4: Sub-Agent Browser + AppModel Wiring ✅
Wire the generic components into the sub-agent browser and integrate into AppModel.

1. Create `cmd/shelly/internal/subagentpanel/` package composing `panel.Model` + `list.Model` (selectable)
   - Build item list from `ChatViewModel.SubAgents()` API (depth for indent, color, status)
   - Handle `ListItemSelectedMsg` → emit `SubAgentSelectedMsg`
   - On error from `SubAgents()`, display error in panel
   - Batch `AgentStartMsg`/`AgentEndMsg` events within tick windows — flush as single list update on `TickMsg`
2. Wire menu bar + sub-agent panel into `AppModel`:
   - Add `menuBar`, `activePanel`, `menuFocused` fields to `AppModel`
   - Wire menu bar into `View()` between viewport/panel area and input
   - Menu bar hidden by default — only rendered after first sub-agent spawn or first task appears; persists for session once visible
   - Implement focus transitions (`Ctrl+B` toggles menu focus, Esc from menu → input)
   - "Subagents" menu item appears on first `AgentStartMsg` with parent; persists for session (dimmed at badge 0)
   - Update badges on `AgentStartMsg`/`AgentEndMsg`
   - Wire activation from `MenuItemSelectedMsg{ID: "subagents"}` → set `activePanel = PanelSubAgents`
   - Wire list panel rendering between chatview and menu bar (viewport height adjustment)
3. **Unit tests for focus state machine** — verify key routing isolation between states:
   - `TestFocusMenuBar_CtrlBFromInput` — Ctrl+B → menu focused
   - `TestFocusMenuBar_CtrlBReturnsToInput` — Ctrl+B in menu → input focused
   - `TestFocusListPanel_EnterFromMenu` — Enter on item → panel opens
   - `TestFocusListPanel_EscClosesPanel` — Esc in panel → returns to menu or input
   - `TestMenuBarKeys_DontReachInput` — Left/Right in menu don't type in textarea
   - `TestListPanelKeys_DontReachInput` — Up/Down in panel don't scroll viewport
   - `TestFocusBlockedByAskPrompt` — Ask prompt active blocks menu activation
   - `TestFocusBlockedByPicker` — File picker active blocks menu activation
   - `TestMenuBarHidden_NoInteraction` — Hidden menu bar ignores key events and takes no layout space
   - `TestMenuBarHint_ShownOnFirstAppearance` — Transient status bar hint shown when menu bar first appears
   - `TestMenuBarHint_DismissedOnKeypress` — Hint dismissed on any keypress or after timeout

### Phase 5: Task Browser (refactor taskpanel) ✅
Now that panel + list components are validated via the sub-agent browser, apply them to the task browser.

1. **Golden tests first:** Snapshot the current task panel rendered output as golden files before touching any code. These capture exact sort order, summary format, and visual layout. The refactor must not change behavior — any visual difference is an explicit, reviewable diff in the golden files.
2. Refactor `cmd/shelly/internal/taskpanel/` to compose `panel.Model` + `list.Model` (read-only) internally
   - Current taskpanel is 133 lines, self-contained
   - Golden tests verify no regressions in rendered output
3. Map task statuses to `list.Status`, assignee to `Detail`
4. Wire activation from `MenuItemSelectedMsg{ID: "tasks"}` → set `activePanel = PanelTasks`
5. Lazy item creation: add "Tasks" item on first `TasksChangedMsg`. Persists once created (dimmed at badge 0, never removed).
6. Update badges on `TasksChangedMsg`
7. Preserve the current sort order (pending > in-progress > completed)
8. Move summary counts to menu bar badge; detailed view in the read-only list panel

### Phase 6: Agent-Scoped Chat View ✅
**Golden tests first:** Snapshot the current chat view rendered output (root view with sub-agent containers) as golden files before modifying rendering logic. These serve as the baseline for verifying that agent-scoped rendering doesn't break root view output.

1. Add `viewedAgent` and `viewStack []viewStackEntry` to `ChatViewModel`
   - `viewStackEntry` holds both `AgentID string` and `Container *AgentContainer` (pinned pointer)
   - View stack capped at 32 entries — entries cleaned up when agents finish (removed if not currently viewed; kept via pinned pointer if viewed, removed on navigate-away)
2. Implement `ChatViewFocusAgentMsg` handler — resolve agent via `FindContainer()`, push to stack (reject if at depth cap), set viewedAgent
3. Implement `ChatViewNavigateBackMsg` handler — pop stack, release pinned container if agent is completed
4. Modify `View()` to render agent-scoped view when `viewedAgent != ""`
5. Render breadcrumb above input area, below panel (reduce viewport height by 1); `RenderBreadcrumb()` composed by AppModel
   - Breadcrumb truncation: middle segments collapsed to `…` when exceeding terminal width; first and last segments always shown
6. Set `MaxShow=0` on the viewed container for full history
7. Reset `viewStack` and `viewedAgent` to root in `clear()` (handles `/compact` during sub-agent view)
8. Wire `Esc` key binding in `AppModel.handleKey` to emit `ChatViewNavigateBackMsg`
9. **Unit tests:**
   - `TestFocusAgent_PushesStack` — focusing agent adds to view stack
   - `TestFocusAgent_DepthCapReached` — push rejected at depth 32 with visual feedback
   - `TestNavigateBack_PopsStack` — Esc pops one level
   - `TestNavigateBack_AtRoot_Noop` — Esc at root view does nothing
   - `TestViewStack_CleanupOnAgentEnd` — finished agent entry removed from stack when not viewed
   - `TestViewStack_AgentEndsWhileViewing` — pinned pointer keeps view valid, entry stays until navigate-away
   - `TestBreadcrumb_Rendering` — correct segments, colors, bold last segment
   - `TestBreadcrumb_Truncation` — middle segments collapsed to `…` when exceeding width
   - `TestBreadcrumb_CompletedSegment_Strikethrough` — completed agent segments rendered with strikethrough
   - `TestBreadcrumb_ReducesViewportHeight` — viewport shrinks by 1
   - `TestClear_ResetsViewStack` — `/compact` returns to root view
   - `TestAgentScoped_MaxShowZero` — viewed container renders full history
   - `TestScrollPosition_PreservedOnBack` — navigating back restores previous scroll offset (or auto-scrolls to bottom if new content arrived)
   - `TestScrollPosition_NewEntryStartsAtBottom` — navigating into sub-agent starts scrolled to bottom
   - `TestScrollPosition_AutoScrollOnNewContent` — returning to parent auto-scrolls if new messages arrived while away
10. **Golden tests:** Verify root view rendering is unchanged after modifications

### Phase 7: Agent Disposal ✅
Separated from agent-scoped view to isolate the data lifecycle change. Depends on Phase 6 (view stack must exist for pinned pointer handling).

1. Implement agent disposal on completion:
   - On `AgentEndMsg`: remove inline sub-agent container, replace with single summary line in parent's view
   - Engine disposes agent (completer + provider are GC'd with agent)
   - Remove agent from active sub-agent tracking
   - Clean up view stack entry if agent is not currently viewed
2. **Unit tests:**
   - `TestAgentDisposal_ReplacedWithSummaryLine` — completed agent shows only summary in parent
   - `TestAgentDisposal_PinnedPointerSurvives` — viewing completed agent still works via pinned pointer
   - `TestAgentDisposal_ViewStackCleanup` — disposed agent removed from stack when not currently viewed
   - `TestAgentDisposal_BadgeUpdates` — menu bar badge decrements on agent end
3. **Golden tests:** Verify parent view with summary lines matches expected output

### Phase 8: Agent-Level Usage Tracking (engine change) ✅
**High-risk phase** — this touches the core engine (`pkg/engine/`) and agent (`pkg/agent/`) packages.

1. Create `AgentUsageCompleter` decorator in `pkg/modeladapter/` — wraps a shared completer, intercepts responses, records usage in an agent-scoped `UsageTracker`
2. Engine change: each agent spawned via delegation gets an `AgentUsageCompleter` wrapping the shared completer
3. Providers and completers remain shared/pooled — not duplicated per agent
4. Agent disposal on completion captures final usage snapshot; engine deregisters wrapper from bridge's active-agent tracker; `AgentUsageCompleter` wrapper becomes unreferenced → GC'd (shared completer lives on)
5. Bridge emits `AgentUsageUpdateMsg{AgentID, Usage}` on each tick for running agents — reads tracker in engine goroutine, sends snapshot via `tea.Msg` (no cross-goroutine access)
6. Session-level usage aggregation: engine captures each agent's final usage on `agent_end` and adds to session total
7. Verify aggregated session usage still works correctly
8. **Unit tests:**
   - `TestAgentUsageCompleter_TracksUsage` — wrapper records usage from completion responses
   - `TestAgentUsageCompleter_DelegatesToShared` — completion calls pass through to shared completer
   - `TestDelegatedAgent_IndependentTracker` — spawned agent gets independent UsageTracker
   - `TestAgentDisposal_WrapperGCd` — wrapper is not referenced after agent disposal (shared completer unaffected)
   - `TestBridgeDeregistration_ReleasesReference` — bridge removes wrapper from active-agent tracker on agent_end
   - `TestSessionUsage_AggregatesAllAgents` — session totals sum across all agent trackers
   - `TestSessionUsage_IncludesDisposedAgents` — disposed agent usage is preserved in session total
   - `TestAgentUsageUpdateMsg_PushBased` — bridge emits usage snapshots on tick, TUI receives them

### Phase 9: Context-Aware Status Bar ✅
1. Extend `AgentEndMsg` with `Usage *usage.Total` — final snapshot captured by bridge at agent_end
2. Add `agentUsage map[string]AgentUsageInfo` to `AppModel`
3. Update usage map from `AgentUsageUpdateMsg` (for live agents, push-based) and from `AgentEndMsg.Usage` (for completed agents)
4. Clean up completed agent usage entries when no longer referenced by view stack
5. Modify `statusBar()` to read from `agentUsage[viewedAgent]` when viewing a sub-agent
6. Add context-sensitive keyboard hints to status bar (menu focused / panel focused / viewing sub-agent with `esc`)
7. Root view continues showing aggregated session usage (unchanged behavior)
8. **Unit tests:**
   - `TestStatusBar_RootView_AggregatedUsage` — default shows session totals
   - `TestStatusBar_SubAgentView_IndividualUsage` — shows viewed agent's stats
   - `TestStatusBar_KeyboardHints_MenuFocused` — shows `←→ navigate` hints
   - `TestStatusBar_KeyboardHints_PanelFocused` — shows `↑↓ navigate` hints
   - `TestStatusBar_KeyboardHints_ViewingSubAgent` — shows `esc back` hint
   - `TestAgentUsage_LiveUpdate` — AgentUsageUpdateMsg updates display for running agents
   - `TestAgentUsage_FreezeOnEnd` — AgentEndMsg.Usage freezes stats
   - `TestAgentUsage_CleanupOnStackPop` — disposed agent usage removed when leaving stack

### Phase 10: Input Routing ✅
1. Modify `AppModel` input submission to route messages to `viewedAgent` when set
2. Engine API: add `SendToAgent(agentID string, msg chats.Message) error` — resolves agent from registry, sends to per-agent inbox channel
3. Per-agent `chan chats.Message` inbox (buffered, capacity 1) — checked by ReAct loop after each tool execution cycle, before next LLM completion
4. Mid-completion queueing: message sits in channel until current completion + tool cycle finishes
5. Capacity overflow: if inbox already has a pending message, `SendToAgent` returns error, TUI shows "agent has a pending message" feedback
6. Handle edge case: viewed agent has completed — reject input with visual feedback
7. Cleanup: engine closes inbox channel and removes agent from registry on finish
8. **Unit tests:**
   - `TestInputRouting_RootView_GoesToRoot` — default routing unchanged
   - `TestInputRouting_SubAgentView_GoesToAgent` — message sent to viewed agent
   - `TestInputRouting_QueuedDuringCompletion` — message queued when agent is mid-completion, delivered after
   - `TestInputRouting_CompletedAgent_Rejected` — input rejected with feedback
   - `TestInputRouting_ToolCallPending_HeldUntilResults` — user message held while tool results are pending, delivered after
   - `TestInputRouting_CompactionThenQueued` — compaction completes before queued message is appended
   - `TestInputRouting_InboxFull_Rejected` — second message rejected when first is still pending

### Phase 11: Integration Tests ✅
End-to-end tests that verify the full flow across components. These use a test harness that simulates the `AppModel` with mock engine events.

1. **`TestIntegration_SpawnAndBrowse`** — spawn agent → open panel → verify agent appears in list → select agent → verify chat view switches → agent completes → verify disposal
2. **`TestIntegration_NestedNavigation`** — spawn parent agent → parent spawns child → navigate to parent → navigate to child → Esc back to parent → Esc back to root
3. **`TestIntegration_InputRoutingFlow`** — navigate to sub-agent → send message → verify routed to correct agent → agent completes → send message → verify rejection
4. **`TestIntegration_RapidAgentChurn`** — spawn and complete 10 agents within a single tick window → verify panel updates once with correct final state (no intermediate flicker)
5. **`TestIntegration_StatusBarContext`** — root view shows session usage → navigate to sub-agent → status bar shows agent usage → navigate back → session usage restored
6. **`TestIntegration_PanelAndMenuLifecycle`** — fresh session has no menu bar → first agent spawns → menu bar appears with hint → open panel → agent completes → badge updates → close panel → menu bar persists with dimmed badge
7. **`TestIntegration_CompactWhileViewingSubAgent`** — navigate to sub-agent → `/compact` → verify view resets to root, view stack cleared
8. **`TestIntegration_ViewStackDepthCap`** — navigate 32 levels deep → verify 33rd push is rejected with feedback

### Phase 12: Polish
1. Auto-scroll to bottom when switching agent views
2. Handle edge case: viewed agent ends while user is viewing it (show "completed" state, allow staying — pinned pointer keeps data alive; input to this agent is rejected)
3. Handle edge case: list panel open while items change (batched updates prevent flicker; cursor position preserved by ID matching)
4. Add sub-agent navigation keybindings to `/help` output
5. Add `/subagents` and `/tasks` slash commands as alternatives to menu bar navigation

## Non-Goals (Explicit)

- **No real-time streaming split view** — only one agent's view at a time
- **No persistent sub-agent view preference** — always starts at root view on new session
- **No filtering/search within sub-agent list** — simple cursor navigation is sufficient for typical agent counts
