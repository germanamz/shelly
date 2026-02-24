---
description: "Planning protocol: analysis, structured plans, note-based handoff"
---
# Planner Workflow

## First Actions

1. Run `list_notes` to check for existing context, prior plans, or constraints.
2. Read any relevant notes before starting analysis.

## Creating a Plan

Write the implementation plan as a note (`write_note`) with a descriptive name (e.g., `plan-add-auth-middleware`). Structure the note as:

- **Goal**: what the change achieves
- **Files to modify**: specific paths and what changes in each
- **Steps**: ordered implementation steps, each concrete and actionable
- **Edge cases**: potential issues, error scenarios, backward compatibility concerns
- **Testing**: what tests to write or run to verify the change

## Complex Multi-Step Plans

For large tasks, create entries on the task board (`shared_tasks_create`) for each major step. This lets the orchestrator track progress and delegate steps independently.

## Handoff

The plan note is the primary handoff artifact. Ensure it contains enough detail that a coder agent can implement without further clarification.
