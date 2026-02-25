# Task Spec: Plan /clean Command

## Objective
Analyze the codebase, particularly the Bubble Tea TUI in `cmd/shelly/shelly.go` and `pkg/engine`, to understand how slash commands are handled and how sessions are managed. Create a detailed, step-by-step implementation plan to add a new `/clean` slash command that closes the current session (stops engine, clears state, chats, tasks) and starts a fresh new one (re-initializes from config).

## Relevant Files (read these first)
- `cmd/shelly/shelly.go` (TUI Model, Update, View, command parsing)
- `pkg/engine/engine.go`, `pkg/engine/session.go` (Engine, Session lifecycle)
- `pkg/state/` (state management)
- `pkg/tasks/` (task board)
- `pkg/agent/` (agents)
- `Taskfile.yml`, `.golangci.yml` (build/test/lint)

Use `search` or `read_file` to find command handling logic (look for \"/[a-z]+\" patterns, msg.Command or similar).

## Constraints
- Go 1.25, module github.com/germanamz/shelly
- Follow conventions: testify assert/require, no full rewrites, targeted edits
- Linter: golangci-lint v2 (gocyclo <=15)
- Tests: add unit tests for new logic
- TUI: bubbletea, lipgloss styles

## Acceptance Criteria
1. Note `clean-command-plan.md` written with sections: **Current Architecture Analysis**, **Proposed Changes**, **Step-by-Step Implementation Plan** (numbered steps with file:line hints), **Files to Modify**, **New Tests**, **Risks & Edge Cases**.
2. Plan enables clean session reset without restarting the app (reuse config, .shelly dir).
3. Call `task_complete` with summary of plan, files analyzed, caveats.
4. Run `task check` in plan verification steps if possible.