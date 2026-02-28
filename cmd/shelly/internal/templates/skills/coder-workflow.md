---
description: "Coding protocol: plan consumption, file-scope adherence, incremental testing, structured reporting"
---
# Coder Workflow

## First Actions

1. Run `list_notes` and **read the implementation plan note first**. This is mandatory — do not start coding without reading the plan.
2. If not already auto-claimed via `task_id` delegation, claim your task on the task board (`shared_tasks_claim`).
3. Check if your delegation context specifies a file scope. If it does, only modify files within that scope.

## Implementation

- Follow the plan from the notes. If the plan is missing or unclear, write a note explaining the gap before proceeding.
- Make focused, incremental changes. Verify each step works before moving on.
- Run tests after each logical change to catch regressions early, not just at the end.
- Stay within your assigned file scope. If you discover changes needed outside your scope, document them in your result note rather than making them.
- **Reading files efficiently**: use `search_content` with `context_lines` to find and inspect code in one step. Use `fs_read_lines` to read a specific section (e.g., a function) when you already know the line range. Use `fs_read` for a full read when you need the complete file text to construct an `fs_edit` match.

## Result Reporting

The lead evaluates your work based on your result note and completion result. Be thorough and specific.

After completing work:

1. Write a result note (`write_note`) named descriptively (e.g., `result-add-auth-middleware`) containing:
   - **What was done**: summary of changes made
   - **Files modified**: list of changed files with a brief description of each change
   - **Tests run**: which tests were executed and their results
   - **Deviations from plan**: any places where you deviated from the plan and why
   - **Remaining work**: anything that could not be completed within your scope
   - **Caveats**: known limitations or follow-up work needed
2. Call `task_complete` with:
   - **status**: `"completed"` or `"failed"`
   - **summary**: concise description of what was done
   - **files_modified**: list of changed files
   - **tests_run**: which tests were executed
   - **caveats**: any known limitations or follow-up work needed

## Task Lifecycle

- When delegated with a `task_id`, the task board is managed automatically — claiming and status updates happen based on your `task_complete` call. You do not need to call `shared_tasks_claim` or `shared_tasks_update` manually.
- If not delegated with a `task_id`, mark your task as completed (`shared_tasks_update` with status `completed`) when done.
- Always call `task_complete` as your final action — do not simply stop responding.

## Approaching Iteration Limit

If you are running low on iterations and cannot finish:

1. Write a progress note documenting what was completed and what remains.
2. Call `task_complete` with `status: "failed"`, include a summary of what's done, and describe remaining work in `caveats`. This gives the lead structured data to decide how to proceed.
