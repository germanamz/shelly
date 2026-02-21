# skill

Package `skill` provides loading of markdown-based skill files that teach agents step-by-step procedures.

## Overview

A **Skill** is a named block of markdown content derived from a `.md` file. Skills are not identity — they are procedures an agent follows. For example, an orchestration skill might instruct the agent to break tasks into subtasks and delegate them.

## Types

- **`Skill`** — holds a `Name` (derived from filename) and `Content` (raw markdown).

## Functions

- **`Load(path)`** — loads a single `.md` file as a Skill.
- **`LoadDir(dir)`** — loads all `.md` files from a directory (non-recursive), sorted by filename.

## Usage

```go
// Load a single skill.
s, err := skill.Load("skills/code-review.md")

// Load all skills from a directory.
skills, err := skill.LoadDir("skills/")
```

## Example Skill File (`skills/orchestration.md`)

```markdown
When you receive a complex task:
1. Break it into subtasks
2. Check available agents with list_agents
3. Delegate subtasks to appropriate agents using delegate_to_agent
4. Synthesize the results into a final answer
```
