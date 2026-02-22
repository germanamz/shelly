# skill

Package `skill` provides loading of markdown-based skill files that teach agents step-by-step procedures, with optional YAML frontmatter for metadata.

## Overview

A **Skill** is a named block of markdown content derived from a `.md` file. Skills are not identity — they are procedures an agent follows. For example, an orchestration skill might instruct the agent to break tasks into subtasks and delegate them.

Skills can optionally include YAML frontmatter with a `description` field. When a description is present, the agent's system prompt shows only the description instead of the full content, and the agent can load the full content on demand via the `load_skill` tool.

## Types

- **`Skill`** — holds a `Name` (derived from filename or frontmatter), `Description` (from frontmatter, may be empty), and `Content` (body after frontmatter, or full content if none).
- **`Store`** — holds loaded skills and exposes a `load_skill` tool for on-demand retrieval.

## Functions

- **`Load(path)`** — loads a single `.md` file as a Skill, parsing YAML frontmatter if present.
- **`LoadDir(dir)`** — loads all `.md` files from a directory (non-recursive), sorted by filename.
- **`NewStore(skills)`** — creates a Store from the given skills.

## Store Methods

- **`Get(name)`** — returns the skill with the given name and whether it was found.
- **`Skills()`** — returns all skills sorted by name.
- **`Tools()`** — returns a `*toolbox.ToolBox` containing the `load_skill` tool.

## YAML Frontmatter

Skills can include YAML frontmatter delimited by `---`:

```markdown
---
name: code-review
description: Teaches code review best practices
---
Full body here...
```

- `name` overrides the filename-derived name.
- `description` populates the `Description` field; when present, the system prompt shows only the description instead of inlining the full content.
- If the file has no frontmatter, the full content goes to `Content` (backward compatible).

## Usage

```go
// Load a single skill.
s, err := skill.Load("skills/code-review.md")

// Load all skills from a directory.
skills, err := skill.LoadDir("skills/")

// Create a store for on-demand loading.
store := skill.NewStore(skills)
tb := store.Tools() // registers "load_skill" tool
```

## Example Skill File (`skills/orchestration.md`)

```markdown
---
description: Orchestration procedures for complex tasks
---
When you receive a complex task:
1. Break it into subtasks
2. Check available agents with list_agents
3. Delegate subtasks to appropriate agents using delegate_to_agent
4. Synthesize the results into a final answer
```

## Dependencies

- `pkg/tools/toolbox` — for the `Store.Tools()` method (same pattern as `state.Store` and `tasks.Store`).
