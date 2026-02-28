---
description: "Task orchestration protocol: plan-first pipeline, delegation, verification, failure recovery"
---
# Orchestrator Workflow

## Pipeline

Every user request follows the same pipeline: **plan -> code -> review**. Do not skip steps.

1. **Plan** — delegate to the planner agent. Wait for the plan note.
2. **Code** — read the plan note, then delegate coding work. For large plans with independent file groups, delegate multiple coders in parallel, each with an explicit file-scope list.
3. **Review** — delegate to the reviewer agent. The reviewer reads the plan and coder result notes and writes a review note.

## Before Delegating

1. Create a task on the shared task board (`shared_tasks_create`) for each unit of work.
2. Write a structured task spec as a note (`write_note`) containing:
   - **Objective**: one-sentence goal
   - **Relevant files**: paths the agent should read first
   - **Constraints**: style, performance, or compatibility requirements
   - **Acceptance criteria**: concrete conditions that define "done"
3. Include the note name in the delegation context so the child agent knows where to find it.

## Delegation

- Use `delegate` with one or more tasks. All tasks run concurrently. For a single sequential delegation, pass one task; for parallel work, pass multiple.
- Always provide rich `context` — include prior decisions, relevant file contents, and the note name.
- Pass `task_id` on each task to automatically claim the task for the child agent and update its status based on the child's `task_complete` result.
- When delegating parallel coders, assign each coder a non-overlapping set of files to avoid merge conflicts.

## After Delegation

1. Check the delegation result. If the child called `task_complete`, the result is structured JSON with `status`, `summary`, `files_modified`, `tests_run`, and `caveats` fields.
2. Read any result notes written by the child agent for additional detail.
3. Verify the result against the acceptance criteria from the task spec.
4. If `task_id` was provided during delegation, the task board is auto-updated based on the child's `task_complete` result. Otherwise, manually mark the task as completed on the board.
5. After all coders finish, delegate to the reviewer before reporting success to the user.

## On Failure or Iteration Exhaustion

Iteration exhaustion returns a structured `CompletionResult` with `status: "failed"` — not an error. This is the same format as a normal `task_complete` call.

1. Check the `caveats` field for exhaustion details and read any progress notes the child may have written.
2. Diagnose the failure: was the task spec unclear? Was the scope too large?
3. Narrow the scope: split into smaller tasks with fewer files each, and re-delegate.
4. If a coder fails twice on the same scope, escalate to the planner to revise the plan before retrying.
5. If repeated failures occur, report the blocker to the user with details.

## State Persistence

- Use notes (`write_note`, `read_note`, `list_notes`) to persist cross-delegation state.
- Use the task board to track overall progress across multiple delegations.
