# skill

Package `skill` provides loading of folder-based skill definitions that teach agents step-by-step procedures, with optional YAML frontmatter for metadata.

## Overview

A **Skill** is a named block of markdown content loaded from a dedicated folder. Each skill folder must contain a `SKILL.md` entry point and may include any number of supplementary files (documentation, scripts, templates) that the agent can access via filesystem tools.

Skills can optionally include YAML frontmatter in `SKILL.md` with `name` and `description` fields. When a description is present, the agent's system prompt shows only the description instead of the full content, and the agent can load the full content on demand via the `load_skill` tool.

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

- **`SKILL.md`** -- mandatory entry point; may contain optional YAML frontmatter
- **Folder name** -- used as the skill name (frontmatter `name:` can override)
- **Supplementary files** -- any additional files/folders; agents access them via filesystem tools using the `Dir` path returned by `load_skill`

## Exported Types

### Skill

```go
type Skill struct {
    Name        string // Derived from folder name; overridden by frontmatter "name" if present.
    Description string // From YAML frontmatter; empty if no frontmatter.
    Content     string // Body after frontmatter (or full content if no frontmatter).
    Dir         string // Absolute path to the skill folder.
}
```

- **`HasDescription()`** -- reports whether the skill has a non-empty description from frontmatter.

### Store

```go
type Store struct { /* unexported fields */ }
```

Holds loaded skills indexed by name and exposes a `load_skill` tool for on-demand retrieval by agents.

## Exported Functions

### Load

```go
func Load(path string) (Skill, error)
```

Loads a skill from a folder containing `SKILL.md`. Parses YAML frontmatter if present. The `Dir` field is always set to the absolute path of the folder. Returns an error if `SKILL.md` is missing or frontmatter YAML is invalid.

### LoadDir

```go
func LoadDir(dir string) ([]Skill, error)
```

Loads skills from all subdirectories that contain a `SKILL.md` file. Subdirectories without `SKILL.md` are silently skipped. Non-directory entries are ignored. Returns an error if the directory cannot be read or any valid skill folder fails to load.

### NewStore

```go
func NewStore(skills []Skill, workDir string) *Store
```

Creates a `Store` from the given skills. The `workDir` parameter is used to convert absolute skill directory paths to relative paths before exposing them to LLM providers, avoiding machine-specific path leakage. Pass an empty string to use absolute paths.

## Store Methods

- **`Get(name string) (Skill, bool)`** -- returns the skill with the given name and whether it was found.
- **`Skills() []Skill`** -- returns all skills sorted by name.
- **`Tools() *toolbox.ToolBox`** -- returns a `ToolBox` containing the `load_skill` tool. The tool response includes the skill content plus a footer with the skill directory path (relative to `workDir` when available) and a hint to use filesystem tools for supplementary files.

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
- If the file has no frontmatter (or no closing `---`), the full content goes to `Content`.
- Windows-style `\r\n` line endings are normalized to `\n` before parsing.

## Usage

```go
// Load a single skill from a folder.
s, err := skill.Load("skills/code-review")

// Load all skills from a directory of skill folders.
skills, err := skill.LoadDir("skills/")

// Create a store for on-demand loading.
store := skill.NewStore(skills, "/path/to/project")
tb := store.Tools() // registers "load_skill" tool
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

- `pkg/tools/toolbox` -- for the `Store.Tools()` method (same pattern as `state.Store` and `tasks.Store`).
- `gopkg.in/yaml.v3` -- for parsing YAML frontmatter.
