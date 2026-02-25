# Minimal /clear TUI Implementation (Part 1)

## Objective
Minimal v1: Add `/clear` to handleSubmit in model.go: cancel bridge, remove old session, clear chatView/input/status, new session, restart bridge. Add eng to model.

Skip Engine clears/tests for now.

## Steps
1. read_file('cmd/shelly/model.go') — find appModel struct, newAppModel, handleSubmit.
2. Edit model.go:
   - Add `eng *engine.Engine` to appModel.
   - Edit newAppModel signature/assign.
3. read_file('cmd/shelly/shelly.go') — update model init to pass eng.
4. In handleSubmit, add \"/clear\" case per plan pseudocode (use strings.TrimSpace(text) == \"/clear\").
5. Add to /help.
6. Add chatViewModel.Clear() if file found, else map clear.
7. `task fmt:check lint test`
8. write_note('clear-tui-changes', summary + diffs)

## Acceptance
- Changes applied, checks pass.
- task_complete
