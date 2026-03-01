# Knowledge Graph Implementation Plan

## Goal

Replace the bespoke Go-centric context generation system (`Generate()`, `IsStale()`, `generateIndex()`) with the spec's knowledge graph vision: a filesystem-based markdown graph that agents build and maintain using their existing tools, driven by a dedicated **project-indexing team**.

The current generator is hardcoded for Go projects (reads `go.mod`, finds `cmd/*/main.go`, walks `pkg/`). The knowledge graph is domain-agnostic — a coding template produces code indexes, a legal template produces document indexes, all driven by agent reasoning.

---

## Current State

### What exists
- **External context** (`LoadExternal`): Reads `CLAUDE.md`, `.cursorrules`, `.cursor/rules/*.mdc` — **keep as-is**
- **Curated context** (`LoadCurated`): Reads `*.md` from `.shelly/` root — **keep as-is** (this IS the knowledge graph entry point layer)
- **Generated context** (`Generate`, `IsStale`, `generateIndex`): Go-specific structural index cached to `.shelly/local/context-cache.json` — **remove entirely**

### What's missing
- No `.shelly/context.md` file created by bootstrap or templates
- No `.shelly/knowledge/` directory
- No project-indexer skill or indexing team template
- `shellydir.Dir` has no `KnowledgeDir()` accessor
- No staleness detection mechanism for the knowledge graph

---

## Design

### Two-Layer Knowledge Graph (from spec §12.2)

```
.shelly/
  context.md              <- Entry point (auto-loaded into prompt via LoadCurated)
  *.md                    <- Additional indexes (auto-loaded)
  knowledge/              <- Deep nodes (read on-demand by agents)
    architecture.md
    api-contracts.md
    ...
```

**Entry points** (`.shelly/*.md`) are loaded into every agent's system prompt automatically by the existing `LoadCurated()` mechanism. They act as indexes that reference deeper files.

**Deep nodes** (`.shelly/knowledge/*.md`) are NOT loaded into prompts. Agents navigate to these on-demand using filesystem tools when the current task requires deeper knowledge.

### Strict Read/Write Separation

The knowledge graph is a **read-only resource** for task agents. Only the dedicated indexing team writes to it. This prevents context leaks — task agents never spend context window budget on indexing work.

- **Task sessions**: Read `.shelly/context.md` and `.shelly/knowledge/` — never write to them
- **Indexing sessions**: Dedicated team that explores the project and writes/updates the graph

### Bootstrap-Triggered Suggestion

When `shelly init` or template `Apply()` runs, it creates the directory structure and a starter `context.md`. The CLI then prints a suggestion: `Run 'shelly index' to build the project knowledge graph.` Indexing is not auto-triggered — it would block init, require a configured provider, and surprise users who just want the directory structure. The user runs `shelly index` explicitly when ready.

### Project-Indexing Team

For larger projects, a single agent would exhaust its context window trying to index everything. A dedicated **indexing team template** solves this:

- **index-lead**: Coordinates the indexing effort, explores top-level structure, writes `context.md`, assigns deep-dive tasks
- **index-explorer**: Explores assigned areas (directories, modules, domains) and writes topic files to `.shelly/knowledge/`

The team uses the existing task board for coordination. The lead creates tasks like "Index pkg/agent/", the explorer claims and completes them.

### System-Level Staleness Detection

Instead of agents checking staleness (which leaks indexing concerns into task context), a lightweight Go-side check in `parallelInit()` compares `context.md` mtime against the latest git commit timestamp (`git log -1 --format=%ct`). If the latest commit is newer than `context.md`, the graph is considered stale and the engine surfaces a hint to the TUI: "Knowledge graph may be outdated. Run `shelly index` to refresh."

Git is a requirement for staleness detection. This filters out uncommitted scratch work and only flags staleness when the project has meaningfully progressed (committed changes). It also handles branch switches naturally. If git is not available or the project is not a git repository, staleness detection is skipped (fail open).

---

## Implementation Steps

### Step 1 — Add `KnowledgeDir()` to `shellydir.Dir`

**File:** `pkg/shellydir/shellydir.go`

Add a path accessor for the knowledge directory:

```go
// KnowledgeDir returns the path to the knowledge graph directory.
func (d Dir) KnowledgeDir() string { return filepath.Join(d.root, "knowledge") }
```

No filesystem I/O — just a path accessor like the others.

### Step 2 — Create `knowledge/` directory in bootstrap

**File:** `pkg/shellydir/init.go`

Add `knowledge/` directory creation to `BootstrapWithConfig()`, alongside the existing `skills/` creation:

```go
func BootstrapWithConfig(d Dir, config []byte) error {
    // ... existing root, skills, ensureStructure ...
    if err := os.MkdirAll(d.KnowledgeDir(), 0o750); err != nil {
        return fmt.Errorf("shellydir: create knowledge dir: %w", err)
    }
    // ... existing config write ...
}
```

Also create a starter `context.md` via `ensureFile()`:

```go
if err := ensureFile(d.ContextPath(), []byte(starterContext)); err != nil {
    return fmt.Errorf("shellydir: context: %w", err)
}
```

Where `starterContext` is a minimal template:

```markdown
# Project Context

<!-- This file is auto-loaded into agent context. -->
<!-- Run `shelly index` to populate it, or edit manually. -->
<!-- Reference deeper docs in .shelly/knowledge/ for on-demand access. -->
```

### Step 3 — Create `knowledge/` and starter `context.md` in template `Apply()`

**File:** `cmd/shelly/internal/templates/templates.go`

When applying a template, also create the `knowledge/` directory and starter `context.md`:

```go
func Apply(t Template, shellyDirPath string, force bool) error {
    // ... existing root, skills, ensureStructure ...
    if err := os.MkdirAll(dir.KnowledgeDir(), 0o750); err != nil {
        return fmt.Errorf("templates: create knowledge dir: %w", err)
    }
    if err := ensureFile(dir.ContextPath(), []byte(starterContext)); err != nil {
        return fmt.Errorf("templates: context: %w", err)
    }
    // ... rest unchanged ...
}
```

### Step 4 — Create the `project-indexer` skill

**File:** `cmd/shelly/internal/templates/skills/project-indexer.md`

This is an on-demand skill (has a `description` in frontmatter) used by the indexing team agents:

```markdown
---
description: "Explore the project and build/update the knowledge graph in .shelly/"
---

# Project Indexer

You are indexing this project to build a knowledge graph that helps all agents
understand the codebase. Follow these steps:

## 1. Check Existing State

Read `.shelly/context.md` first. If it already has real content (not just the
starter template), this is an **incremental update**:
- Compare the existing index against the current project structure
- Focus on areas that have changed (new directories, renamed packages, etc.)
- Update only the sections that are stale rather than rewriting everything

If `context.md` is empty or contains only the starter template, this is a
**full index** — proceed with a complete exploration.

## 2. Explore Project Structure

Use filesystem and search tools to understand the project:
- Read the project root for key files (README, config files, entry points)
- List top-level directories to understand layout
- Identify the language/framework (go.mod, package.json, Cargo.toml, etc.)
- Find entry points and main packages

## 3. Write the Entry Point Index

Write `.shelly/context.md` — this is auto-loaded into every agent's prompt.
Keep it concise (under 200 lines) since it consumes context window space.

Include:
- **Project overview**: What the project does (1-2 sentences)
- **Tech stack**: Language, framework, key dependencies
- **Directory layout**: Top-level structure with one-line descriptions
- **Package/module map**: Key modules with their roles
- **Entry points**: Main executables or endpoints
- **Key conventions**: Naming, testing, build commands
- **Knowledge references**: Links to deeper docs in `.shelly/knowledge/`

Example format:
```
# Project Context

## Overview
<project-name> is a <brief description>.

## Tech Stack
- Language: Go 1.25
- Key deps: bubbletea, yaml.v3, testify

## Structure
- `cmd/` — CLI entry points
- `pkg/` — Library packages
- `internal/` — Private implementation

## Package Map
| Package | Role |
|---------|------|
| pkg/engine | Composition root, wires config |
| pkg/agent | ReAct loop, delegation |

## Entry Points
- `cmd/myapp/main.go` — Main CLI

## Conventions
- Tests: `go test` with testify assertions
- Format: gofumpt
- Build: `task build`

## Deep Docs
- [Architecture](.shelly/knowledge/architecture.md)
- [API Contracts](.shelly/knowledge/api-contracts.md)
```

## 4. Write Deep Knowledge Files

Write detailed topic files to `.shelly/knowledge/`. These are NOT loaded
into prompts — agents read them on-demand when a task requires it.

Suggested topics (create only what's relevant):
- `architecture.md` — Component relationships, data flow, design patterns
- `api-contracts.md` — API surface, request/response formats
- `dependencies.md` — Key dependency graph, version constraints
- `testing.md` — Test patterns, fixtures, how to run tests

## 5. Verify

After writing, read back `.shelly/context.md` to confirm it's well-formed
and provides a useful project overview.
```

### Step 5 — Create the project-indexing team template

**File:** `cmd/shelly/internal/templates/settings/project-indexer.yaml`

A dedicated team template for indexing, triggered by `shelly index` or after bootstrap:

```yaml
meta:
  name: project-indexer
  description: "Dedicated team for building and updating the project knowledge graph"

config:
  providers:
    - name: default

  entry_agent: index-lead

  agents:
    - name: index-lead
      instructions: |
        You are the lead of a project-indexing team. Your job is to coordinate
        the exploration and indexing of this project into a knowledge graph.

        1. Read `.shelly/context.md` — if it has real content (not the starter
           template), this is an incremental update. Compare existing index
           against current project state and only re-index changed areas.
        2. Explore the project's top-level structure (root files, directories)
        3. Write/update `.shelly/context.md` with a concise project overview (under 200 lines)
        4. For each major area that needs indexing, delegate to index-explorer
           via the task board to write deep knowledge files in `.shelly/knowledge/`
        5. After all explorers complete, review `.shelly/context.md` and add
           references to the deep docs in `.shelly/knowledge/`
      toolboxes:
        - fs_read
        - fs_write
        - search
        - tasks
      skills:
        - project-indexer
      max_iterations: 20

    - name: index-explorer
      instructions: |
        You are a project explorer. You receive tasks to explore specific areas
        of the codebase and write knowledge files to `.shelly/knowledge/`.

        For each assigned area:
        1. Read source files, configs, and READMEs in the assigned directory
        2. Understand the purpose, public API, key types, and patterns
        3. Write a focused markdown file to `.shelly/knowledge/<topic>.md`
        4. Keep files concise — focus on what an agent needs to work in that area
      toolboxes:
        - fs_read
        - fs_write
        - search
        - tasks
      skills:
        - project-indexer
      max_iterations: 10

  stores:
    - kind: tasks
      name: indexing
```

### Step 6 — Add `shelly index` CLI subcommand

**File:** `cmd/shelly/shelly.go` (or wherever CLI subcommands are registered)

A dedicated subcommand that creates a session with the `project-indexer` team template. More discoverable than `shelly run --template project-indexer`.

```
shelly index          # Run the project-indexing team to build/update the knowledge graph
shelly index --check  # Only check if the knowledge graph is stale (no indexing)
```

The subcommand:
1. Resolves the `.shelly/` directory (same as other commands)
2. Loads the `project-indexer` team template (embedded, not from `.shelly/skills/`)
3. Creates an engine with the indexing team config
4. Starts a session with the `index-lead` entry agent
5. Runs to completion (non-interactive — the team coordinates autonomously)

The `--check` flag runs only `IsKnowledgeStale()` and prints the result without starting a session.

Both `shelly init` and template `Apply()` print: `Run 'shelly index' to build the project knowledge graph.` after completing.

### Step 7 — Add `project-indexer` skill to existing templates

**File:** `cmd/shelly/internal/templates/settings/simple-assistant.yaml`

Add the skill to the single agent and to the embedded skills list:

```yaml
config:
  agents:
    - name: assistant
      # ... existing config ...
      skills:
        - project-indexer

embedded_skills:
  - name: project-indexer
```

**File:** `cmd/shelly/internal/templates/settings/coding-team.yaml`

Add the skill to the `explorer` agent (natural fit — it already does codebase intelligence) and to the embedded skills list:

```yaml
config:
  agents:
    - name: explorer
      # ... existing config ...
      skills:
        - explorer-workflow
        - project-indexer

embedded_skills:
  # ... existing skills ...
  - name: project-indexer
```

### Step 8 — Remove the bespoke generator

**Files to modify:**
- `pkg/projectctx/projectctx.go` — Remove `Generate()`, `IsStale()`, `loadGenerated()`, the `Generated` field from `Context`, and its inclusion in `String()`
- `pkg/projectctx/generator.go` — Delete the entire file
- `pkg/projectctx/generator_test.go` — Delete the entire file
- `pkg/projectctx/external.go` — Keep as-is (external context is unrelated)

**Changes to `projectctx.go`:**

The `Context` struct becomes:

```go
type Context struct {
    External string // Content from external AI tool context files.
    Curated  string // Content from curated *.md files (knowledge graph entry points).
}
```

`String()` drops the `Generated` block. `Load()` drops the `loadGenerated()` call.

**Engine changes (`pkg/engine/engine.go` lines 342-353):**

Remove the `IsStale` + `Generate` calls from `parallelInit()`:

```go
wg.Go(func() {
    status("Loading project context...")
    start := time.Now()
    projectRoot := filepath.Dir(dir.Root())
    loadedCtx = projectctx.Load(dir, projectRoot)
    status(fmt.Sprintf("Project context ready (%s)", time.Since(start).Round(time.Millisecond)))
})
```

**`shellydir` changes:**

Remove `ContextCachePath()` from `pkg/shellydir/shellydir.go` — no longer needed.

### Step 9 — Add system-level staleness detection

**File:** `pkg/projectctx/staleness.go`

A lightweight Go-side check that compares `context.md` mtime against the latest git commit timestamp. This runs in `parallelInit()` and surfaces a suggestion — no agent involvement.

Git is a requirement. This filters out uncommitted scratch work and only flags staleness when the project has meaningfully progressed. It also handles branch switches naturally — checking out an older branch won't trigger a false positive.

```go
// IsKnowledgeStale checks if the knowledge graph entry point is outdated
// relative to the latest git commit. Returns true if context.md is missing
// or older than the most recent commit in the repository.
func IsKnowledgeStale(projectRoot string, d shellydir.Dir) bool {
    contextPath := filepath.Join(d.Root(), "context.md")
    info, err := os.Stat(contextPath)
    if err != nil {
        return true // missing = stale
    }
    contextMtime := info.ModTime()

    // Get latest commit timestamp via: git -C <projectRoot> log -1 --format=%ct
    // Parse the unix timestamp and compare against contextMtime.
    // If the latest commit is newer than context.md, the graph is stale.
    // If git is not available or not a repo, return false (no staleness
    // detection without git — fail open).
    // ...
}
```

**Engine integration:**

In `parallelInit()`, after loading context, check staleness and store the result for the CLI/TUI to display a hint:

```go
wg.Go(func() {
    status("Loading project context...")
    start := time.Now()
    projectRoot := filepath.Dir(dir.Root())
    loadedCtx = projectctx.Load(dir, projectRoot)
    knowledgeStale = projectctx.IsKnowledgeStale(projectRoot, dir)
    status(fmt.Sprintf("Project context ready (%s)", time.Since(start).Round(time.Millisecond)))
})
```

The engine exposes `KnowledgeStale() bool` so the TUI can show: "Knowledge graph may be outdated. Run `shelly index` to refresh."

### Step 10 — Update READMEs

**File:** `pkg/projectctx/README.md`

Rewrite to reflect the two-source model (external + curated). Remove all references to generated index, `Generate()`, `IsStale()`, and caching. Document the new `IsKnowledgeStale()` function.

**File:** `pkg/shellydir/README.md`

Add `KnowledgeDir()` to the path accessors documentation. Remove `ContextCachePath()`.

### Step 11 — Update tests

- **`pkg/projectctx/projectctx_test.go`** (if it exists): Remove any tests for `Generate`, `IsStale`, `loadGenerated`, or the `Generated` field
- **`pkg/projectctx/staleness_test.go`**: Tests for `IsKnowledgeStale()` — missing context.md returns true, context.md newer than latest commit returns false, context.md older than latest commit returns true, non-git directory returns false (fail open)
- **`pkg/shellydir/init_test.go`**: Add test that `Bootstrap` creates `knowledge/` directory and `context.md` file
- **`cmd/shelly/internal/templates/templates_test.go`**: Verify `Apply` creates `knowledge/` directory and `context.md`; verify `project-indexer` skill is present in templates

### Step 12 — Update NEXT.md

Mark items #9 and #10 as done. Update item #8 to note it was addressed by removing the generator entirely rather than enhancing it.

---

## Files Summary

### Delete
| File | Reason |
|------|--------|
| `pkg/projectctx/generator.go` | Bespoke Go generator replaced by agent-driven indexing |
| `pkg/projectctx/generator_test.go` | Tests for deleted generator |

### Create
| File | Purpose |
|------|---------|
| `cmd/shelly/internal/templates/skills/project-indexer.md` | On-demand skill for agent-driven indexing |
| `cmd/shelly/internal/templates/settings/project-indexer.yaml` | Dedicated indexing team template |
| `pkg/projectctx/staleness.go` | Lightweight knowledge graph staleness check |
| `pkg/projectctx/staleness_test.go` | Tests for staleness detection |

### Modify
| File | Changes |
|------|---------|
| `cmd/shelly/shelly.go` | Add `shelly index` subcommand |
| `pkg/projectctx/projectctx.go` | Remove `Generated` field, `Generate()`, `IsStale()`, `loadGenerated()` |
| `pkg/projectctx/README.md` | Rewrite for two-source model |
| `pkg/shellydir/shellydir.go` | Add `KnowledgeDir()`, remove `ContextCachePath()` |
| `pkg/shellydir/init.go` | Create `knowledge/` dir and `context.md` in bootstrap, print indexing suggestion |
| `pkg/shellydir/README.md` | Update path accessors |
| `pkg/engine/engine.go` | Remove `IsStale`/`Generate` from `parallelInit()`, add staleness check, expose `KnowledgeStale()` |
| `cmd/shelly/internal/templates/templates.go` | Create `knowledge/` and `context.md` in `Apply()`, print indexing suggestion |
| `cmd/shelly/internal/templates/settings/simple-assistant.yaml` | Add `project-indexer` skill |
| `cmd/shelly/internal/templates/settings/coding-team.yaml` | Add `project-indexer` skill to explorer |
| `cmd/shelly/internal/templates/templates_test.go` | Add knowledge dir + skill tests |

### Keep Unchanged
| File | Reason |
|------|---------|
| `pkg/projectctx/external.go` | External context loading is orthogonal |

---

## Risk Assessment

**Low risk:**
- The generated context source was recently wired up (items #6-#8 in NEXT.md) but provides minimal value — a flat package list that any agent can discover with `fs_list` + `search_files`
- The curated context path (`LoadCurated`) already works and is the mechanism the knowledge graph uses
- No behavioral change for users who haven't created `.shelly/*.md` files yet (empty curated = empty context, same as before)

**Migration:**
- Users with an existing `.shelly/local/context-cache.json` won't notice — it was gitignored local state
- The `context.md` created by bootstrap uses `ensureFile` (O_EXCL) so existing files are never overwritten
- Templates create `knowledge/` on `Apply()` but don't require it to exist for loading

**What this does NOT change:**
- External context loading (CLAUDE.md etc.) — completely untouched
- How context flows into agent prompts (`Options.Context` -> `<project_context>`) — untouched
- Skill loading mechanism — untouched, we just add a new skill
- The curated context loader — untouched, it already does what we need
