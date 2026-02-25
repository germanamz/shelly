# Task Spec: Implement /clear Command

## Objective
Implement the `/clear` slash command in the Shelly TUI exactly as detailed in the plan note `clear-command-plan`. Close current session, clear UI views, start fresh session. Skip optional state/tasks clear for v1 (add methods but comment out calls).

## Relevant Files (Read First)
- `clear-command-plan` (full plan)
- `cmd/shelly/model.go` (TUI model, handleSubmit)
- `cmd/shelly/shelly.go` or `cmd/shelly/main.go` (app init, pass eng)
- `cmd/shelly/chatview.go` (add Clear())
- `pkg/engine/engine.go` (add ClearState/ClearTasks)
- `cmd/shelly/model_test.go` (add tests)
- `cmd/shelly/README.md` (update commands)

Search for `handleSubmit`, `newAppModel`, `chatViewModel`.

## Constraints
- **Targeted Edits**: Use `edit_file` with precise snippets; verify with `read_file` before/after.
- **No Rewrites**: Preserve existing logic/structure.
- **Tests**: Add 2-3 unit tests for handleSubmit(\"/clear\"), verify session swap, view clear.
- **Quality**: After changes: `task fmt lint test check`. Fix issues before complete.
- **Defaults**: `NewSession(\"\")` uses entry agent.
- **Feedback**: Use Thinking item for \"Starting fresh...\", status msg.
- **Bubbletea**: Proper msg batch (e.g., tea.ClearStatusBar{} if needed).

## Steps
1. Add `eng *engine.Engine` to appModel; update constructors.
2. Implement `chatViewModel.Clear()`.
3. Add `Engine.ClearState()` / `ClearTasks()` (empty impl for v1, comment calls).
4. Implement `/clear` case in `handleSubmit`: stop bridge, remove old sess, clear views, new sess, restart bridge.
5. Update `/help` text.
6. Add tests.
7. Update README.md.
8. Run `task check` → all green.

## Acceptance Criteria
1. `task run` → `/clear` works: UI clears, new session starts, no panics.
2. `task test` → 100% pass, new tests green.
3. Write summary note `clear-command-changes`:
   - List edited files + diff summaries.
   - Manual test results.
   - `task check` output.
4. Call `task_complete` with summary: \"Implemented /clear per plan. All checks pass.\"
