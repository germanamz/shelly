# NEXT_UI.md — TUI vs UI_SPEC.md Gap Analysis

Comparison of the current `cmd/shelly/` implementation against `UI_SPEC.md`.
Items are categorized as **Missing** (not implemented), **Misaligned** (implemented but differs from spec), or **Extra** (implemented but not in spec).

---

## Bugs

### 0. Managed Region Overflow — Stale Lines Merge With Scrollback

After a certain number of messages or elapsed time, the live managed region (everything returned by `View()`) never gets fully cleared, causing its remnants to merge with newly committed content in scrollback.

**Root cause:** Bubbletea's managed region is re-rendered at the bottom of the terminal each frame. When `tea.Println` commits content, the runtime moves the cursor up by the last-known view height, clears those lines, prints the committed text, then re-renders the managed region. However, if the managed region exceeds the terminal viewport height, the top portion scrolls into immutable scrollback. The runtime can only clear visible lines — it cannot erase content that has already scrolled off the top of the viewport. When the view subsequently shrinks (e.g., after `EndAgent` removes a container), those stale lines remain in scrollback and visually merge with the committed content above them.

**Reproduction conditions:**
- Long-running sessions with multiple concurrent sub-agents (each with 4+ windowed items)
- Task panel active (adds height)
- Multi-line input (up to 5 lines)
- Small terminal height relative to the live content

**Contributing factors in the code:**

1. **Unbounded live view height** — `ChatViewModel.View()` renders all active agent containers with no height cap. With 2-3 sub-agents (each showing 4 items + header + windowing indicator) plus the standalone spinner, the chatview alone can be 20-30+ lines. Add the task panel (up to 7 lines) and input box (up to 8 lines) and the managed region easily exceeds a typical 40-50 row terminal.

   `cmd/shelly/internal/chatview/chatview.go:55-80` — no height limit on concatenated container views.

2. **Batch Println + view shrink** — `FlushAll()` returns a `tea.Batch` of multiple `tea.Println` calls, one per agent. Each Println triggers a hide-print-rerender cycle. Between these cycles, the managed region shrinks as agents are removed from the map, but the runtime's height tracking may lag behind the actual view changes within the same batch.

   `cmd/shelly/internal/chatview/chatview.go:309-344` — `FlushAll` batches N Println commands.
   `cmd/shelly/internal/app/app.go:143-152` — `SendCompleteMsg` handler calls `FlushAll`.

3. **ThinkingItem committed as live view item** — `ThinkingItem.IsLive()` returns `false`, yet it remains in the agent container's `Items` list and is rendered by `AgentContainer.View()` for the duration of the agent's lifetime. These items add to the managed region height despite being semantically "static." They could be committed to scrollback immediately via `tea.Println` to reduce the live view height.

   `cmd/shelly/internal/chatview/items.go:62` — `IsLive() = false` but never committed.
   `cmd/shelly/internal/chatview/container.go:178-206` — `View()` renders all items including non-live ones.

**Potential fixes (not mutually exclusive):**

- **Cap the managed region height** — Limit `View()` output to `terminalHeight - N` lines, windowing the chatview content and committing overflow to scrollback proactively.
- **Eagerly commit non-live items** — When a `ThinkingItem`, completed `ToolCallItem`, or completed `ToolGroupItem` is added/completed, immediately emit it via `tea.Println` and remove it from the container's Items list. This keeps the managed region minimal (only truly live items).
- **Single-Println flush** — Instead of batching N `tea.Println` calls in `FlushAll`, concatenate all summaries into a single string and emit one `tea.Println`. This avoids multiple hide-print-rerender cycles within a single frame.

---

## Missing

### 1. Header Region

The spec defines a persistent, re-rendering header at the top of the screen with two lines:

1. `shelly` in **bold accent** (#8250df)
2. Status items separated by ` · `: Agent name, Model, Provider, Connection status (`● Connected` / `● Disconnected`), Config path

The implementation has **no header region**. The ASCII art logo is printed once at startup via `tea.Println` (committed to scrollback), but there is no live status bar showing session metadata. None of the following are displayed:

- Active agent name
- Model identifier
- Provider name
- Connection status indicator
- Config file path
- "No config" fallback state

**Files to create/modify:** `cmd/shelly/internal/app/app.go` (add header to `View()`), possibly a new `header/` package.

### 2. Message Queuing During Agent Processing

**Spec:** "The input remains enabled while the agent is working. Messages sent during agent processing are **queued** and delivered when the agent is ready."

**Implementation:** When the user submits during `StateProcessing`, the current `Send` is cancelled (`cancelSend()`) and a new `Send` is started with the new text. This is cancel-and-restart, not queue-and-deliver.

**File:** `cmd/shelly/internal/app/app.go:324-338` (`handleSubmit` processing branch).

### 3. Streaming Agent Text

**Spec:** "Agent text streams in chunk by chunk as it is generated."

**Implementation:** Assistant messages are processed as complete `message.Message` objects from the chat model. There is no incremental/character-by-character text display. The bridge forwards full messages from `c.Since()`, and `processAssistantMessage` adds the complete text as a single `ThinkingItem` or `FinalAnswer`. Text appears all at once rather than streaming.

**Files:** `cmd/shelly/internal/chatview/chatview.go:137-199`, `cmd/shelly/internal/bridge/bridge.go`.

---

## Misaligned

### 4. Logo Display vs Header "shelly" Text

**Spec header:** First line shows `shelly` as plain bold accent text.
**Spec empty state:** ASCII art seashell drawing shown in the messages area before the first message.

**Implementation:** The ASCII art (`LogoArt`) is printed in `DimStyle` at startup and serves double duty as both the "header branding" and the empty state. There is no separate `shelly` bold accent text line, and the ASCII art is not removed after the first message — it stays in scrollback permanently.

**File:** `cmd/shelly/internal/app/app.go:79`, `cmd/shelly/internal/chatview/chatview.go:19-25`.

### 5. Sub-Agent Active Header Color

**Spec:** "While active, the header shows the agent name in **magenta** (#8250df) with a braille spinner."

**Implementation:** Active sub-agent headers use the agent's round-robin palette color (assigned in `StartAgent`), not magenta. The spec's "Agent Name Colors" section (which assigns distinct palette colors) contradicts the sub-agent container section (which says magenta). The implementation follows the palette colors.

**File:** `cmd/shelly/internal/chatview/items.go:351-354` (`SubAgentItem.View`).

### 6. Input Border Style

**Spec:** Shows horizontal rule separators (`──────`) above and below the user input.

**Implementation:** Uses a lipgloss `RoundedBorder()` (`╭──╮` / `╰──╯`) with accent-blue foreground when focused, muted when disabled. Functionally equivalent but visually different from the spec's flat horizontal rules.

**File:** `cmd/shelly/internal/input/input.go:228-239`, `cmd/shelly/internal/styles/styles.go:60-61`.

### 7. Config Wizard "Save" Behavior

**Spec:** `Save` writes config, shows "Config saved!" status, and **stays on screen**. `Save & Quit` writes and exits.

**Implementation:** The `configSavedMsg` handler sets `m.saved = true` and calls `tea.Quit`, meaning **all saves exit the wizard**. There is no "save and stay" flow. The View shows "Config saved successfully!" only as an exit message.

**File:** `cmd/shelly/internal/configwizard/wizard.go:80-82`.

### 8. Config Wizard Summary on No Config

**Spec:** When no config is loaded, summary reads "No configuration loaded."

**Implementation:** Summary reads "No configuration loaded" (without the trailing period). Minor difference.

**File:** `cmd/shelly/internal/configwizard/wizard.go:213`.

### 9. Questions UI Border Color

**Spec:** Does not specify a border color for the questions UI.

**Implementation:** Uses `AskBorder` with `ColorWarning` (#9a6700, amber) border. The spec shows the questions UI without any explicit border mention — the spec wireframes use plain layout without a border box.

**File:** `cmd/shelly/internal/styles/styles.go:70`.

### 10. User Message Text Wrapping

**Spec:** "All multi-line text wraps using pre-word wrapping at the terminal width."

**Implementation:** User messages (`RenderUserMessage`) split on `\n` and indent continuation lines, but do not apply word-wrapping to long lines. A single long line without newlines will extend past the terminal width rather than wrapping at the boundary.

**File:** `cmd/shelly/internal/format/format.go:127-141`.

---

## Extra (Not in Spec)

### 11. `/quit` Command

Implementation supports both `/quit` and `/exit` for exiting. The spec only mentions `/exit`.

**File:** `cmd/shelly/internal/app/app.go:284`.

### 12. `shelly index` Subcommand

A standalone subcommand that auto-submits an indexing message. Not mentioned in the UI spec.

**File:** `cmd/shelly/index.go`.

### 13. Stale Escape Filter

A 200ms filter that suppresses all key input (except Ctrl+C) after startup to prevent terminal escape sequence responses from leaking into the textarea. Not described in the spec but addresses a real terminal compatibility issue.

**File:** `cmd/shelly/internal/tty/drain.go`.

### 14. OSC 11 Background Detection

Pre-detects dark/light terminal background before Bubbletea starts, storing the result in `format.IsDarkBG` so glamour uses a fixed style. Not in spec but prevents rendering artifacts.

**File:** `cmd/shelly/main.go:104`, `cmd/shelly/internal/format/format.go:18`.

### 15. BSD/macOS Stdin Flush

Platform-specific raw-mode stdin drain (`FlushStdinBuffer`) to clear leftover bytes. No-op on non-BSD. Not in spec.

**File:** `cmd/shelly/internal/tty/flush_unix.go`, `cmd/shelly/internal/tty/flush_other.go`.

---

## Summary Table

| # | Category | Area | Severity |
|---|----------|------|----------|
| 0 | Bug | Managed region overflow — stale lines merge with scrollback | High |
| 1 | Missing | Header region (session status bar) | High |
| 2 | Missing | Message queuing during processing | Medium |
| 3 | Missing | Streaming agent text chunks | Medium |
| 4 | Misaligned | Logo display vs header text | Medium |
| 5 | Misaligned | Sub-agent active header color (palette vs magenta) | Low |
| 6 | Misaligned | Input border style (rounded vs flat rules) | Low |
| 7 | Misaligned | Config wizard save-and-stay behavior | Medium |
| 8 | Misaligned | Config summary trailing period | Trivial |
| 9 | Misaligned | Questions UI border color | Low |
| 10 | Misaligned | User message text wrapping | Medium |
| 11 | Extra | `/quit` command alias | Trivial |
| 12 | Extra | `shelly index` subcommand | N/A |
| 13 | Extra | Stale escape filter | N/A |
| 14 | Extra | OSC 11 background detection | N/A |
| 15 | Extra | BSD/macOS stdin flush | N/A |
