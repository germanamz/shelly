# search

Package search provides tools for searching file contents and finding files by name patterns.

## Permission Model

Directory access is gated by the shared permissions store, using the same directory-approval model as the filesystem package.

## Tools

| Tool | Description |
|------|-------------|
| `search_content` | Search file contents using a regular expression. Returns matching lines with path and line number |
| `search_files` | Find files by name pattern (supports glob with `**` for recursive matching) |

## Usage

```go
s := search.New(permStore, askFn)
tb := s.Tools() // *toolbox.ToolBox with search tools
```

The `AskFunc` callback is called whenever permission for a new directory is needed.
