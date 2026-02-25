# Task Spec: Plan /clear Command

## Objective
Create a detailed, actionable implementation plan for adding a \"/clear\" slash command to the Shelly TUI. The command shall close the current session (stop agents, clear ephemeral state like chats, tasks) and start a fresh new session loaded from the YAML config, preserving .shelly/ directory structure but resetting runtime state.

## Relevant Files
Start exploration by reading these core files, then search for more:
- `cmd/shelly/shelly.go` (TUI entry point, likely main model)
- `pkg/engine/README.md` and key files in `pkg/engine/` (composition root, Session management)
- `pkg/agent/README.md` (agent lifecycle)
- `pkg/tasks/README.md` (task board)
- `pkg/state/README.md` (state store)

Use coding tools:
- `search` codebase for \"slash command\", \"cmd/\", \"session\", \"engine.New\", \"chat.Clear\"
- `read_file` on promising matches
- `list_dir` for TUI models/views

## Constraints
- Follow project conventions: targeted edits, testify tests with `assert`, linter compliance.
- Leverage existing Engine/Session APIs for reset/restart; avoid raw state mutation.
- Bubbletea: handle as `tea.Msg` or input parser for \"/clear\".
- Fresh session: new `engine.Session`, clear shared state/tasks/notes? Clarify scope.
- No changes to dependency structure or composition root.
- Ensure command is user-friendly: confirmation? Immediate?

## Acceptance Criteria
1. Write plan to note `clear-command-plan` as structured Markdown:
   - **Current State Analysis**: Key code paths for sessions, commands, TUI input.
   - **Proposed Changes**: Files to edit/create, pseudocode/diffs for each.
   - **New Tests**: Specific test cases for /clear behavior.
   - **Verification Steps**: How to test manually + `task run`.
   - **Risks/Edge Cases**: Incomplete resets, concurrent agents, config reload.
2. Plan must be self-contained for coder implementation.
3. Call `task_complete` with:
   - status: \"completed\"
   - summary: 1-2 sentence overview
   - files_analyzed: list of key files read
