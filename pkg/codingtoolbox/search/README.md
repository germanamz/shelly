# search

Package `search` provides tools for searching file contents and finding files by name patterns.

## Permission Model

Directory access is gated by the shared permissions store, using the same directory-approval model as the `filesystem` package. Concurrent permission prompts for the same directory are coalesced via a `pendingResult` map so the user is never asked the same question multiple times.

## Exported API

### Types

- **`Search`** -- provides search tools with permission gating.
- **`AskFunc`** -- `func(ctx context.Context, question string, options []string) (string, error)` callback for permission prompts.

### Functions

- **`New(store *permissions.Store, askFn AskFunc) *Search`** -- creates a Search backed by the given shared permissions store.

### Methods on Search

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox containing the 2 search tools.

## Tools

| Tool | Description |
|------|-------------|
| `search_content` | Search file contents using a regular expression. Returns JSON array of `{path, line, content}`. Default max 100 results, configurable via `max_results`. Total matched content capped at 1MB. |
| `search_files` | Find files by name pattern (supports glob with `**` for recursive matching). Returns JSON array of relative file paths. Default max 100 results, configurable via `max_results`. |

### search_content Details

- Walks the directory tree recursively.
- Skips binary files (first 512 bytes must be valid UTF-8).
- Supports lines up to 1MB (custom scanner buffer).
- Resolves symlinks and skips files whose real path is outside the search directory.
- Paths in results are relative to the search directory.

### search_files Details

- Walks the directory tree recursively.
- Pattern matching supports three modes:
  - Simple glob (e.g. `*.go`) -- matches against the base file name.
  - Glob with directory separator (e.g. `sub/*.go`) -- matches against the full relative path.
  - Double-star glob (e.g. `**/*.go`) -- matches recursively across any number of path segments.
- Resolves symlinks and skips files whose real path is outside the search directory.

## Usage

```go
s := search.New(permStore, askFn)
tb := s.Tools() // *toolbox.ToolBox with 2 search tools
```

The `AskFunc` callback is called whenever permission for a new directory is needed. The user is prompted with `["yes", "no"]` options.

## Dependencies

- `pkg/codingtoolbox/permissions` -- shared permissions store (directory approval)
- `pkg/tools/toolbox` -- Tool and ToolBox types
