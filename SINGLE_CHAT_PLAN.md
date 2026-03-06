# Single Chat View Plan

## Problem

The current TUI renders sub-agent messages **nested inside** the parent agent's container using tree-pipe indentation (`┃`). When the user "selects" a sub-agent, the sub-agent's container renders within the parent's viewport, growing the parent's display. The correct behavior is a **full viewport switch**: selecting a sub-agent replaces the entire rendered content with that agent's messages, and the parent stops rendering until the user navigates back.

Sub-agent spawning on the parent should render like a tool execution (a compact inline item), not an expanding nested container.

## Current Architecture

### Key structures (`cmd/shelly/internal/chatview/chatview.go`)
- `ChatViewModel.agents` — top-level agent containers
- `ChatViewModel.subAgents` — sub-agent containers (also inserted into parent's `Items`)
- `ChatViewModel.viewedAgent` — currently selected agent (`""` = root)
- `ChatViewModel.viewStack` — navigation history

### Current flow
1. **Sub-agent starts** (`startAgent`, line 663): creates `AgentContainer` with color, `MaxShow=4`, and **appends it to `parentAC.Items`** (line 676)
2. **Rendering** (`container.go:View`, line 188): sub-agent containers render inline with tree-pipe indentation when `isSubAgent` (Color != "")
3. **Sub-agent ends** (`endAgent`, line 695): replaces `AgentContainer` in parent's Items with `SummaryLineItem`
4. **Focused view** (`liveContent`, line 446): when `viewedAgent != ""`, renders only that agent's container — but it's still structurally nested in the parent

### Problem details
- Sub-agents grow inside the parent container, pushing parent content down
- The "focused" view shows the sub-agent's container in isolation but doesn't show its committed history (user messages, prior turns)
- There is no per-agent committed history — only one global `committed []string`

---

## Design: Single Active Chat View

### Core Concept

Only one agent's messages render at a time. The viewport shows the **selected agent's full chat history** (committed messages + live activity). Switching agents is a full content swap, not a nested embed.

Sub-agent spawning appears in the parent as a **compact inline item** (similar to a tool call), showing status, name, and a one-line summary when done.

### Step 1: Per-Agent Committed History ✅

**Goal:** Each agent maintains its own committed message buffer so switching between agents shows the correct chat history.

**Changes in `chatview.go`:**

- Add `Committed []string` field to `AgentContainer` (`container.go`)
- When committing user messages (`commitUserMessage`), append to the **currently viewed agent's** container instead of the global `committed` slice
- When committing assistant text responses, route to the appropriate agent's `Committed` buffer
- Keep the global `committed` for session-level messages (welcome text, system notices) that appear on the root view
- On `endAgent` for top-level agents, collapse their `Committed` into the global buffer (existing behavior for summaries)

**Migration:** Move the initial welcome/system messages to remain in global `committed`. Agent-specific messages go to `AgentContainer.Committed`.

### Step 2: Remove Nested Sub-Agent Rendering ✅

**Goal:** Sub-agents no longer render as expanding nested containers inside the parent.

**Changes in `chatview.go` `startAgent`:**

- Stop appending `AgentContainer` to `parentAC.Items` (remove line 676)
- Instead, append a new **`SubAgentRefItem`** to the parent's Items — a lightweight, non-expanding display item

**New item: `SubAgentRefItem`** (in `items.go`):

```go
type SubAgentRefItem struct {
    Agent         string
    Prefix        string
    ProviderLabel string
    Color         string
    Task          string    // delegation task description
    Status        string    // "running", "done", "failed"
    FinalAnswer   string    // set on completion
    Elapsed       string    // set on completion
    FrameIdx      int       // for spinner animation
}
```

- **View when running:** renders like a tool call — colored agent name with spinner, plus the task description on one line
  ```
  ⚡ coder-refactor (anthropic/claude-sonnet-4) ◐
    Task: Refactor the auth module
  ```
- **View when done:** renders like current `SummaryLineItem` — checkmark, name, elapsed, answer excerpt
  ```
  ✓ coder-refactor (anthropic/claude-sonnet-4) — Refactored 3 files  12s
  ```
- `IsLive()` returns `true` while running (so spinners advance)

This replaces both the live nested container AND the `SummaryLineItem` replacement logic.

### Step 3: Modify `liveContent` for Full View Switch

**Goal:** `liveContent()` renders the selected agent's full history, not just its container.

**Changes in `chatview.go` `rebuildContent`:**

Current logic (line 422-434):
```go
// Renders global committed + liveContent()
```

New logic:
```
if viewedAgent == "":
    render global committed + root agent containers (as today)
else:
    render viewedAgent's Committed history + viewedAgent's live container items
```

- When viewing a sub-agent, the viewport shows that agent's `Committed` buffer followed by its live `Items` — identical to how root agents render
- The global `committed` buffer is NOT shown when viewing a sub-agent

### Step 4: Update `endAgent` for Sub-Agents

**Goal:** When a sub-agent ends, update the `SubAgentRefItem` in the parent instead of replacing an `AgentContainer`.

**Changes in `chatview.go` `endAgent`:**

- Remove the `replaceWithSummaryLine` call for sub-agents
- Instead, find the `SubAgentRefItem` in the parent's Items and update its `Status`, `FinalAnswer`, and `Elapsed` fields
- Keep the sub-agent's `AgentContainer` in `subAgents` map (marked `Done`) so the user can still navigate to view its history after completion
- Only clean up the container from `subAgents` when the parent top-level agent ends (or on `flushAll`)

**Deferred cleanup:** Sub-agent containers stay in `subAgents` until the session ends, allowing post-completion browsing. The `SubAgentRefItem` in the parent acts as a clickable/selectable reference.

### Step 5: Route Messages to Correct Agent

**Goal:** Incoming messages (tool calls, thinking, text) are routed to the correct agent container regardless of which agent is currently viewed.

This already works — `handleAgentMsg` in `chatview.go` resolves the container by agent name. No changes needed here.

**User input routing** (`app.go` `handleSubAgentSubmit`): Already routes to the viewed agent via `m.eng.SendToAgent`. The committed user message needs to go to the viewed agent's `Committed` buffer (Step 1).

### Step 6: Update Sub-Agent Panel & Navigation

**Changes in `subagentpanel.go`:**

- `SubAgents()` method: include completed sub-agents (since containers are now retained)
- Add the root/entry agent to the list as the first item so users can navigate back to root from the panel
- Selecting an agent triggers `ChatViewFocusAgentMsg` (unchanged)

**Changes in `focusAgent` / `navigateBack`:**

- `focusAgent`: instead of a deep view stack, simplify to a flat selection — set `viewedAgent` and save previous scroll position. The view stack can remain for back-button support but depth is effectively 1 (root or one agent)
- `navigateBack`: returns to root view

**Breadcrumb simplification:**
- Since nesting is flattened visually, breadcrumb shows: `<- root > agent-name` (max 1 level deep in the breadcrumb, since the viewport always shows one agent's full chat)

### Step 7: Spinner Animation for SubAgentRefItem

**Changes in `container.go` `advanceFrame`:**

- When advancing frames, also advance `FrameIdx` on any `SubAgentRefItem` in the container's Items that has `Status == "running"`
- This keeps the spinner animated in the parent's view

### Step 8: Update `flushAll`

**Changes in `chatview.go` `flushAll`:**

- For sub-agents: mark `SubAgentRefItem` in parent as done (same as endAgent)
- Keep existing logic for top-level agents (collapse committed + summary into global buffer)
- Clean up `subAgents` map

### Step 9: Update Tests

**Changes in `chatview_test.go`:**

- Update tests that assert `AgentContainer` in parent's Items to assert `SubAgentRefItem` instead
- Update tests that assert `SummaryLineItem` replacement to assert `SubAgentRefItem` status update
- Add test: switching `viewedAgent` renders the correct agent's committed history
- Add test: sub-agent messages don't appear in parent's container
- Add test: `SubAgentRefItem` shows running spinner, then completion summary

---

## Summary of File Changes

| File | Changes |
|------|---------|
| `chatview/container.go` | Add `Committed []string` to `AgentContainer`. Remove sub-agent tree-pipe rendering (lines 229-236). Update `advanceFrame` for `SubAgentRefItem`. |
| `chatview/items.go` | Add `SubAgentRefItem` struct + `View`/`IsLive`/`Kind` methods. `SummaryLineItem` can be kept for top-level agent summaries or removed if unified. |
| `chatview/chatview.go` | `startAgent`: create `SubAgentRefItem` in parent instead of nesting container. `endAgent`: update `SubAgentRefItem` status instead of `replaceWithSummaryLine`. `rebuildContent`: render per-agent committed history when `viewedAgent != ""`. `commitUserMessage`: route to viewed agent's Committed. `flushAll`: update for new structure. Retain sub-agent containers after completion. |
| `chatview/chatview_test.go` | Update all sub-agent tests for new item types and rendering behavior. |
| `subagentpanel/subagentpanel.go` | Include completed agents and root agent in the list. |
| `app/app.go` | Minor: ensure user message commits route to correct agent buffer. |

## Non-Goals

- No changes to `pkg/agent/`, `pkg/engine/`, or the event bridge — the event flow stays the same
- No changes to the sub-agent panel's visual design (just data changes)
- No changes to how agent inboxes or message routing works in the engine
- No changes to status bar or usage tracking

## Migration Risk

- **Low risk:** All changes are in `cmd/shelly/internal/chatview/` and `cmd/shelly/internal/app/`
- **No protocol changes:** Engine events remain identical
- **Backwards compatible:** The sub-agent panel, breadcrumb, and keyboard shortcuts work the same way — only the rendering model changes
