# projectctx

Package `projectctx` loads curated context and generates/caches a structural project index.

## Purpose

Agents need to understand the project they are working in. This package assembles project context from two sources:

1. **Curated context** — hand-written `*.md` files in the `.shelly/` root (e.g., `context.md`).
2. **Generated index** — an auto-generated structural overview cached in `.shelly/local/context-cache.json`.

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
ctx := projectctx.Load(d)

// Inject into agent options.
opts := agent.Options{
    Context: ctx.String(),
}
```

## Dependencies

- `pkg/shellydir` — path resolution for `.shelly/` directory layout.
