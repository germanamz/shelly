# projectctx

Package `projectctx` loads curated context and checks knowledge graph staleness for injection into agent system prompts.

## Purpose

Agents need to understand the project they are working in. This package assembles project context from two sources and combines them into a single `Context` value:

1. **External context** -- context files from other AI coding tools, loaded from the project root:
   - `CLAUDE.md` (Claude Code)
   - `.cursorrules` (Cursor legacy)
   - `.cursor/rules/*.mdc` (Cursor modern, sorted alphabetically, YAML frontmatter stripped)
2. **Curated context** -- hand-written `*.md` files in the `.shelly/` root (e.g., `context.md`). These serve as the knowledge graph entry points, auto-loaded into every agent's system prompt.

External context appears first in the combined output, followed by curated -- so project-specific Shelly context takes precedence by appearing later.

The combined context is injected into agent system prompts via `agent.Options.Context`.

### Knowledge Graph

The knowledge graph is a filesystem-based markdown graph that agents build and maintain:

```
.shelly/
  context.md              <- Entry point (auto-loaded into prompts via LoadCurated)
  *.md                    <- Additional indexes (auto-loaded)
  knowledge/              <- Deep nodes (read on-demand by agents)
    architecture.md
    api-contracts.md
    ...
```

**Entry points** (`.shelly/*.md`) are loaded automatically. **Deep nodes** (`.shelly/knowledge/*.md`) are read on-demand by agents when needed.

The knowledge graph is built by a dedicated **project-indexing team** (`shelly index`), not by task agents.

## Exported Types

### Context

```go
type Context struct {
    External string // Content from external AI tool context files.
    Curated  string // Content from curated *.md files in .shelly/.
}
```

- **`String()`** -- returns the combined context string. Sections are separated by `\n\n`. Empty sections are omitted.

## Exported Functions

### Load

```go
func Load(d shellydir.Dir, projectRoot string) Context
```

Assembles project context from both sources (external, curated). All sources are best-effort: missing files are silently skipped.

### LoadCurated

```go
func LoadCurated(d shellydir.Dir) string
```

Reads all `*.md` files from the `.shelly/` root directory (via `Dir.ContextFiles()`) and concatenates their trimmed contents, separated by `\n\n`. Returns empty string if no files exist or all are empty/whitespace.

### LoadExternal

```go
func LoadExternal(projectRoot string) string
```

Reads context files from external AI coding tools at the project root. Returns concatenated content separated by `\n\n`. Missing files and empty files are silently skipped.

### IsKnowledgeStale

```go
func IsKnowledgeStale(projectRoot string, d shellydir.Dir) bool
```

Checks whether the knowledge graph entry point (`context.md`) is outdated relative to the latest git commit. Returns `true` if `context.md` is missing or older than the most recent commit. Returns `false` if git is not available or the project is not a git repository (fail open).

## Usage

```go
d := shellydir.New(".shelly")

// Load all context sources.
ctx := projectctx.Load(d, "/path/to/project")

// Check if knowledge graph needs refreshing.
if projectctx.IsKnowledgeStale("/path/to/project", d) {
    // Suggest: "Run 'shelly index' to refresh."
}

// Inject into agent options.
opts := agent.Options{
    Context: ctx.String(),
}
```

## Dependencies

- `pkg/shellydir` -- path resolution for `.shelly/` directory layout.
