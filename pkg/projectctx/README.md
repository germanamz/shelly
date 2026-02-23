# projectctx

Package `projectctx` loads curated context and generates/caches a structural project index.

## Purpose

Agents need to understand the project they are working in. This package assembles project context from three sources:

1. **External context** — context files from other AI coding tools, loaded from the project root:
   - `CLAUDE.md` (Claude Code)
   - `.cursorrules` (Cursor legacy)
   - `.cursor/rules/*.mdc` (Cursor modern, YAML frontmatter stripped)
2. **Curated context** — hand-written `*.md` files in the `.shelly/` root (e.g., `context.md`).
3. **Generated index** — an auto-generated structural overview cached in `.shelly/local/context-cache.json`.

External context appears first in the combined output so that Shelly-specific curated context takes precedence by appearing later.

The combined context is injected into agent system prompts via `agent.Options.Context`.

## Generated Index Contents

The structural index includes:
- Go module path (from `go.mod`)
- Entry points (`cmd/*/main.go`)
- Package listing (`pkg/` subdirectories with `.go` files, depth-limited)

## Staleness

The cache is considered stale when `go.mod` is newer than the cache file. Use `IsStale()` to check and `Generate()` to refresh.

## Usage

```go
d := shellydir.New(".shelly")
ctx := projectctx.Load(d, "/path/to/project")

// Inject into agent options.
opts := agent.Options{
    Context: ctx.String(),
}
```

## Dependencies

- `pkg/shellydir` — path resolution for `.shelly/` directory layout.
