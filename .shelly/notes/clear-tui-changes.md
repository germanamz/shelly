# Clear TUI Implementation Progress (Part 1)

## Status
✅ **Complete**: The minimal v1 /clear TUI implementation is already present in the codebase and matches the spec exactly.

## Key Changes Verified
- `cmd/shelly/model.go`:
  - `appModel` includes `eng *engine.Engine`.
  - `newAppModel` accepts and sets `eng`.
  - `handleSubmit` has `/clear` case:
    - Cancels bridge.
    - `m.eng.RemoveSession(m.sess.ID())`.
    - `newSess := m.eng.NewSession("")`.
    - Updates `m.sess`, `m.chatView.Clear()`, `m.inputBox.Reset()`, status message.
    - Restarts bridge.
    - TODOs for v2 state/task clears.
  - `/help` includes `/clear`.

- `cmd/shelly/main.go`:
  - Passes `eng` to `newAppModel(ctx, sess, eng, verbose)`.

- `cmd/shelly/chatview.go`:
  - `Clear()` method resets agents, sub-agents, processing state.

## Verification
- `task check`: ✅ Passed (fmt:check, lint, tests: 864 passed).
- No edits needed.

## Next Steps
Proceed to Engine changes (Part 2) or v2 features per overall plan.
