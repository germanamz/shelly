# /clean Command Implementation Plan

## Current Architecture Analysis
- **TUI (cmd/shelly/shelly.go)**: Bubble Tea application with `Model` struct containing UI state (`chatHistory []DisplayItem`, `input` bubbletea.Model, `status`, styles). `Model.Update(tea.Msg)` handles `tea.KeyMsg` for input, parses messages on enter. Slash commands (`/help`, `/clear`, etc.) are handled by checking if input starts with `/` and dispatching to handlers (e.g., `/clear` clears `chatHistory`).
- **Engine (pkg/engine/)**: Top-level composition root. Loads YAML config, initializes `.shelly/`, loads skills/project context. Provides `Engine` with `NewSession()` -> `Session`. `Session` manages agent orchestration, event bus, state, tasks.
- **State (pkg/state/)**: Key-value store, likely per-session or global.
- **Tasks (pkg/tasks/)**: Shared task board for multi-agent coordination.
- **Agent (pkg/agent/)**: ReAct loop, delegation, effects.
- Session lifecycle: Start -> Run agents -> Stop/Cleanup.

Slash commands are processed in `Model.handleCommand(string)` or inline in `updateChatInput()`.

## Proposed Changes
- Add `/clean` handler in TUI Model:
  1. Gracefully shutdown current `Session` (if active): `session.Close()` or `engine.Stop()`.
  2. Clear volatile state: `chatHistory = []`, `stateStore.Clear()`, complete/clear tasks.
  3. Optionally reset `.shelly/` state files if persisted (use `pkg/shellydir/` bootstrap).
  4. Start new `Session`: `engine.NewSession(config)` (reuse original YAML config path).
  5. Update UI: Add message \"New session started.\", focus input.
- Reuse CLI args/config for new session (store `configPath` in Model).
- No app restart needed.

## Step-by-Step Implementation Plan
1. **Confirm structure**: Read `cmd/shelly/shelly.go` to locate command parsing (search for `/help`, `/clear`). Likely in `Model.updateChatInput(tea.KeyMsg)` or separate `handleCommand(string)`.
2. **Store config path**: Add `configPath string` to Model if not present (CLI arg passed to Model).
3. **Implement handler**:
   - In command switch/if:
     ```go
     case \"clean\", \"/clean\":
       if m.currentSession != nil { // assume field name
         ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
         m.currentSession.Close(ctx)
         cancel()
         m.currentSession = nil
       }
       m.chatHistory = m.chatHistory[:0]
       if m.stateStore != nil {
         m.stateStore.Clear()
       }
       // Clear tasks
       tasks, _ := m.taskBoard.List(shared_tasks_list logic)
       for _, t := range tasks {
         if t.Status != \"completed\" {
           m.taskBoard.Update(t.ID, \"completed\", \"Cleaned by /clean\")
         }
       }
       m.currentSession = m.engine.NewSession(m.configPath) // start new
       m.chatHistory = append([]DisplayItem{NewDisplayItem(\"🧹 New session started clean!\", userRole? )}, m.chatHistory...)
       m.input.Reset()
       m.Status = \"New session active\"
       return m, nil
     ```
4. **Add necessary imports**: context, time if needed.
5. **Handle missing methods**: If Session.Close not exist, implement or use engine.ResetSession().
6. **Tests**: Add to `shelly_test.go`:
   - `TestHandleCleanCommand`: mock Model with session, assert cleared, new session.
7. **Verify**: `task build`, `task run`, test /clean, `task check`.

## Files to Modify
- `cmd/shelly/shelly.go`
- `cmd/shelly/shelly_test.go`
- Possibly `pkg/engine/session.go` if Close needed.

## Potential Risks
- Exact field names/methods may differ (coder to discover).
- Blocking close: timeout.
- Task clear: ensure no lock.
- Persistent state not cleared: note for future /reset.
- Config path: find how passed (os.Args[1]?).

Plan assumes typical structure; adjust based on code read.