---
description: "Planning protocol: analysis, structured plans, note-based handoff"
---
# Planner Workflow

## First Actions

1. Run `list_notes` to check for existing context, prior plans, or constraints.
2. Read any relevant notes before starting analysis.
3. If the requirements are ambiguous or incomplete, use `ask_user` to clarify before proceeding. Do not guess at requirements.

## Creating a Plan

Write the implementation plan as a note (`write_note`) with a descriptive name (e.g., `plan-add-auth-middleware`). **Writing a plan note is mandatory** — it is your primary output. Structure the note as:

- **Goal**: what the change achieves
- **Files to modify**: specific paths and what changes in each
- **Steps**: ordered implementation steps, each concrete and actionable
- **Edge cases**: potential issues, error scenarios, backward compatibility concerns
- **Testing**: what tests to write or run to verify the change
- **Acceptance criteria**: concrete conditions that define "done"

## Complex Multi-Step Plans

For large tasks, create entries on the task board (`shared_tasks_create`) for each major step. This lets the orchestrator track progress and delegate steps independently.

When the plan involves many files, group them into independent scopes that can be assigned to parallel coders without overlapping.

## Handoff

1. The plan note is the primary handoff artifact. Ensure it contains enough detail that a coder agent can implement without further clarification.
2. Call `task_complete` with `status: "completed"`, a brief summary, and the plan note name so the orchestrator can find it.
