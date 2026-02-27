# projectctx

Package `projectctx` loads curated context and generates/caches a structural project index for injection into agent system prompts.

## Purpose

Agents need to understand the project they are working in. This package assembles project context from three sources and combines them into a single `Context` value:

1. **External context** -- context files from other AI coding tools, loaded from the project root:
   - `CLAUDE.md` (Claude Code)
   - `.cursorrules` (Cursor legacy)
   - `.cursor/rules/*.mdc` (Cursor modern, sorted alphabetically, YAML frontmatter stripped)
2. **Curated context** -- hand-written `*.md` files in the `.shelly/` root (e.g., `context.md`).
3. **Generated index** -- an auto-generated structural overview cached in `.shelly/local/context-cache.json`.

External context appears first in the combined output, followed by curated, then generated -- so project-specific Shelly context takes precedence by appearing later.

The combined context is injected into agent system prompts via `agent.Options.Context`.

## Exported Types

### Context

```go
type Context struct {
    External  string // Content from external AI tool context files.
    Curated   string // Content from curated *.md files in .shelly/.
    Generated string // Auto-generated structural index.
}
```

- **`String()`** -- returns the combined context string. Sections are separated by `\n\n`. Empty sections are omitted.

## Exported Functions

### Load

```go
func Load(d shellydir.Dir, projectRoot string) Context
```

Assembles project context from all three sources (external, curated, generated cache). All sources are best-effort: missing files are silently skipped.

### LoadCurated

```go
func LoadCurated(d shellydir.Dir) string
```

Reads all `*.md` files from the `.shelly/` root directory (via `Dir.ContextFiles()`) and concatenates their trimmed contents, separated by `\n\n`. Returns empty string if no files exist or all are empty/whitespace.

### LoadExternal

```go
func LoadExternal(projectRoot string) string
```

Reads context files from external AI coding tools at the project root:
- `CLAUDE.md`
- `.cursorrules`
- `.cursor/rules/*.mdc` (sorted alphabetically, YAML frontmatter stripped)

Returns concatenated content separated by `\n\n`. Missing files and empty files are silently skipped.

### Generate

```go
func Generate(projectRoot string, d shellydir.Dir) (string, error)
```

Creates a structural project index and writes it to the cache file at `.shelly/local/context-cache.json`. Creates the `local/` directory if needed. Returns the generated index string.

### IsStale

```go
func IsStale(projectRoot string, d shellydir.Dir) bool
```

Checks whether the cached index is older than the project's `go.mod`. Returns `true` if the cache file is missing or stale. Returns `false` if there is no `go.mod` (staleness cannot be determined).

## Generated Index Contents

The structural index includes:
- Go module path (from `go.mod`)
- Entry points (`cmd/*/main.go` patterns)
- Package listing (`pkg/` subdirectories containing `.go` files, depth-limited to 4 levels)

## Usage

```go
d := shellydir.New(".shelly")

// Load all context sources.
ctx := projectctx.Load(d, "/path/to/project")

// Check if regeneration is needed.
if projectctx.IsStale("/path/to/project", d) {
    generated, err := projectctx.Generate("/path/to/project", d)
    // ...
}

// Inject into agent options.
opts := agent.Options{
    Context: ctx.String(),
}
```

## Dependencies

- `pkg/shellydir` -- path resolution for `.shelly/` directory layout.
