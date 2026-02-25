# Task Spec: Implement /clean Command

## Objective
Implement the '/clean' slash command in the TUI according to the plan in note 'clean-command-plan'. Close current session, clear state/tasks/chats, start fresh session from config, update UI.

## Relevant Files (read first)
- Note: 'clean-command-plan' for detailed plan
- `cmd/shelly/shelly.go`
- `cmd/shelly/shelly_test.go`
- `pkg/engine/*.go` (session lifecycle)
- `pkg/state/*`
- `pkg/tasks/*`
- `pkg/agentctx/*` (context keys)

Search codebase for existing commands: grep -r \"/help\" or /clear.

## Constraints
- Targeted edits: read file before edit, prefer append case in switch over rewrite.
- Tests: use testify/assert, table-driven if multiple.
- Linter: gocyclo <=15, no unused.
- No breaking changes to existing commands.
- Graceful: timeout on session close.

## Acceptance Criteria
1. `/clean` handled: run app, type /clean, see \"New session started\", old chats gone, new agents ready.
2. State cleared: shared_state_list empty post-clean, tasks all completed.
3. Code changes pass: `task fmt:check`, `task lint`, `task test` (100% coverage on new code).
4. New tests: at least 2-3 covering happy path, no-session case.
5. Write note 'implementation-complete' with summary: files changed, test results, manual test outcome.
6. Call `shared_tasks_update` id=\"task-2\" status=completed when done.
7. If issues, block on new task or note risks.