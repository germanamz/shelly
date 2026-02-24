---
description: "Coding protocol: plan consumption, task lifecycle, result reporting"
---
# Coder Workflow

## First Actions

1. Run `list_notes` and read any implementation plan or context notes.
2. Claim your task on the task board (`shared_tasks_claim`).

## Implementation

- Follow the plan from the notes. If the plan is missing or unclear, write a note explaining the gap before proceeding.
- Make focused, incremental changes. Verify each step works before moving on.
- Run tests after making changes to catch regressions early.

## Result Reporting

After completing work:

1. Write a result note (`write_note`) named descriptively (e.g., `result-add-auth-middleware`) containing details of what was done.
2. Call `task_complete` with:
   - **status**: `"completed"` or `"failed"`
   - **summary**: concise description of what was done
   - **files_modified**: list of changed files
   - **tests_run**: which tests were executed
   - **caveats**: any known limitations or follow-up work needed

## Task Lifecycle

- Mark your task as completed (`shared_tasks_update` with status `completed`) when done.
- If you cannot complete the task, mark it as failed with a description of what went wrong.
- Always call `task_complete` as your final action — do not simply stop responding.

## Approaching Iteration Limit

If you are running low on iterations and cannot finish:

1. Write a progress note documenting what was completed and what remains.
2. Mark the task as failed on the task board with details about remaining work.
