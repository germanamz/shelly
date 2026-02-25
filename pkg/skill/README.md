# skill

Package `skill` provides loading of folder-based skill definitions that teach agents step-by-step procedures, with optional YAML frontmatter for metadata.

## Overview

A **Skill** is a named block of markdown content loaded from a dedicated folder. Each skill folder must contain a `SKILL.md` entry point and may include any number of supplementary files (documentation, scripts, templates) that the agent can access via filesystem tools.

Skills can optionally include YAML frontmatter in `SKILL.md` with a `description` field. When a description is present, the agent's system prompt shows only the description instead of the full content, and the agent can load the full content on demand via the `load_skill` tool.

## Folder Structure

```
skills/
  code-review/
    SKILL.md           # Required entry point
    checklist.md       # Supplementary doc (agent reads via filesystem)
  deployment/
    SKILL.md
    scripts/
      deploy.sh        # Script the agent can execute
```

- **`SKILL.md`** — mandatory entry point, same frontmatter format as before
- **Folder name** — used as the skill name (frontmatter `name:` can override)
- **Supplementary files** — any additional files/folders; agents access them via filesystem tools using the `Dir` path returned by `load_skill`

## Types

- **`Skill`** — holds `Name` (from folder or frontmatter), `Description` (from frontmatter, may be empty), `Content` (SKILL.md body after frontmatter), and `Dir` (absolute path to skill folder).
- **`Store`** — holds loaded skills and exposes a `load_skill` tool for on-demand retrieval.

## Functions

- **`Load(path)`** — loads a skill from a folder containing `SKILL.md`, parsing YAML frontmatter if present.
- **`LoadDir(dir)`** — loads skills from all subdirectories that contain a `SKILL.md` file. Subdirectories without `SKILL.md` are silently skipped.
- **`NewStore(skills)`** — creates a Store from the given skills.

## Store Methods

- **`Get(name)`** — returns the skill with the given name and whether it was found.
- **`Skills()`** — returns all skills sorted by name.
- **`Tools()`** — returns a `*toolbox.ToolBox` containing the `load_skill` tool. The tool response includes the skill content plus a footer with the skill directory path when available.

## YAML Frontmatter

`SKILL.md` files can include YAML frontmatter delimited by `---`:

```markdown
---
name: code-review
description: Teaches code review best practices
---
Full body here...
```

- `name` overrides the folder-derived name.
- `description` populates the `Description` field; when present, the system prompt shows only the description instead of inlining the full content.
- If the file has no frontmatter, the full content goes to `Content`.

## Usage

```go
// Load a single skill from a folder.
s, err := skill.Load("skills/code-review")

// Load all skills from a directory of skill folders.
skills, err := skill.LoadDir("skills/")

// Create a store for on-demand loading.
store := skill.NewStore(skills)
tb := store.Tools() // registers "load_skill" tool
```

## Example Skill Folder (`skills/orchestration/SKILL.md`)

```markdown
---
description: Orchestration procedures for complex tasks
---
When you receive a complex task:
1. Break it into subtasks
2. Check available agents with list_agents
3. Delegate subtasks to appropriate agents using delegate
4. Synthesize the results into a final answer
```

## Per-Agent Skill Assignment

Agents can declare a `skills` list in their YAML config to receive only a subset of the engine-level skills:

```yaml
agents:
  - name: coder
    skills: [coder-workflow]
```

When `skills` is non-empty, the engine filters the loaded skills to only those matching the listed names. When empty or omitted, the agent receives all engine-level skills (backward compatible). This lets each agent see only the workflow skills relevant to its role, keeping system prompts focused.

## Dependencies

- `pkg/tools/toolbox` — for the `Store.Tools()` method (same pattern as `state.Store` and `tasks.Store`).
