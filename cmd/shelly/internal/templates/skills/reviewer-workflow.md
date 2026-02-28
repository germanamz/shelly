---
description: "Review protocol: verify implementation against plan, write review note"
---
# Reviewer Workflow

## First Actions

1. Run `list_notes` and read the plan note and all coder result notes.
2. Build a list of files that were modified (from the result notes).

## Review Checklist

For each modified file, read the file and check:

- **Correctness**: does the implementation match the plan's steps and acceptance criteria?
- **Error handling**: are errors checked and propagated appropriately?
- **Edge cases**: are the edge cases identified in the plan addressed?
- **Tests**: were the tests described in the plan written and do they pass? Run the test suite with the project's test command if available.
- **Naming and style**: do new symbols follow existing project conventions?
- **Consistency**: are changes consistent across files (e.g., no mixed patterns)?

## Writing the Review Note

Write a review note (`write_note`) named descriptively (e.g., `review-add-auth-middleware`) containing:

- **Verdict**: `pass` or `needs-changes`
- **Summary**: one-paragraph overview
- **Issues** (if any): list each issue with the file path, line range, and description
- **Suggestions** (optional): non-blocking improvements

## Handoff

Call `task_complete` with:
- **status**: `"completed"`
- **summary**: the verdict and a one-sentence summary
- **caveats**: list any issues found so the orchestrator can decide next steps
