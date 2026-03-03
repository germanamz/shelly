# mcproots

Package `mcproots` provides shared utilities for MCP Roots protocol support.
It handles context plumbing and path-checking logic used by both the client
side (sending approved directories as roots to MCP servers) and the server
side (constraining filesystem access based on client-declared roots).

This package is intentionally zero-dependency (no MCP SDK imports) so it can
be imported by `mcpclient`, `mcpserver`, and `filesystem` without introducing
circular dependencies. SDK-dependent conversions (`DirToRoot`, `RootPaths`)
live in their respective packages.

## Exported API

### Functions

- **`WithRoots(ctx context.Context, roots []string) context.Context`** — returns a new context carrying the given root paths.
- **`FromContext(ctx context.Context) []string`** — extracts root paths from the context. Returns `nil` if no roots were set (unconstrained access).
- **`IsPathAllowed(absPath string, roots []string) bool`** — reports whether `absPath` falls under at least one root. `nil` roots = unconstrained (returns true). Empty `[]string{}` = nothing allowed (returns false).

## Semantics

| `roots` value | Meaning | `IsPathAllowed` behavior |
|---------------|---------|--------------------------|
| `nil` | No roots constraint | Always returns `true` |
| `[]string{}` | Explicit empty set | Always returns `false` |
| `[]string{"/home/user"}` | One root | Returns `true` for paths under `/home/user` |

## Usage

```go
// Server side: constrain filesystem tools based on client roots.
ctx := mcproots.WithRoots(ctx, []string{"/home/user/projects"})

// In filesystem tool's checkPermission:
roots := mcproots.FromContext(ctx)
if roots != nil {
    if !mcproots.IsPathAllowed(absPath, roots) {
        return fmt.Errorf("path outside client roots")
    }
    return nil // allowed, skip interactive prompt
}
```
