---
description: "Task orchestration protocol: decomposition, delegation, verification, failure recovery"
---
# Orchestrator Workflow

## Before Delegating

1. Create a task on the shared task board (`shared_tasks_create`) for each unit of work.
2. Write a structured task spec as a note (`write_note`) containing:
   - **Objective**: one-sentence goal
   - **Relevant files**: paths the agent should read first
   - **Constraints**: style, performance, or compatibility requirements
   - **Acceptance criteria**: concrete conditions that define "done"
3. Include the note name in the delegation context so the child agent knows where to find it.

## Delegation

- Use `delegate_to_agent` for sequential tasks or `spawn_agents` for independent parallel work.
- Always provide rich `context` — include prior decisions, relevant file contents, and the note name.
- Pass `task_id` to `delegate_to_agent` or each task in `spawn_agents` to automatically claim the task for the child agent and update its status based on the child's `task_complete` result.

## After Delegation

1. Check the delegation result. If the child called `task_complete`, the result is structured JSON with `status`, `summary`, `files_modified`, `tests_run`, and `caveats` fields.
2. Read any result notes written by the child agent for additional detail.
3. Verify the result against the acceptance criteria from the task spec.
4. If `task_id` was provided during delegation, the task board is auto-updated based on the child's `task_complete` result. Otherwise, manually mark the task as completed on the board.

## On Failure or Iteration Exhaustion

1. Read any progress notes the child may have written.
2. Diagnose the failure: was the task spec unclear? Was the scope too large?
3. Refine the task spec, split into smaller tasks if needed, and re-delegate.
4. If repeated failures occur, report the blocker to the user with details.

## State Persistence

- Use notes (`write_note`, `read_note`, `list_notes`) to persist cross-delegation state.
- Use the task board to track overall progress across multiple delegations.
