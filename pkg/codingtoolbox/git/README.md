# git

Package `git` provides tools that give agents controlled access to git operations.

## Permission Model

Command execution is gated by the shared permissions store using the command trust model. The trust key is `"git"` -- trusting it allows all future git operations without being prompted. Users are prompted with three options: **yes** (single invocation), **trust** (permanent), and **no** (deny).

## Exported API

### Types

- **`Git`** -- provides git tools with permission gating. All commands run in a configurable working directory.
- **`AskFunc`** -- `func(ctx context.Context, question string, options []string) (string, error)` callback for permission prompts.

### Functions

- **`New(store *permissions.Store, askFn AskFunc, workDir string) *Git`** -- creates a Git that checks the given permissions store for trusted commands and prompts the user via askFn when git is not yet trusted. `workDir` sets the working directory for all git subprocess invocations.

### Methods on Git

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox containing the 4 git tools.

## Tools

| Tool | Description |
|------|-------------|
| `git_status` | Show the working tree status. Supports `short` flag for short format. |
| `git_diff` | Show changes between commits and working tree. Supports `staged` (--cached) and `path` filtering. |
| `git_log` | Show commit logs. Default: last 10 commits in oneline format. Configurable `count` and `format`. |
| `git_commit` | Create a git commit. Supports `files` (stage specific files), `all` (stage all tracked changes), and `message`. |

### git_log Format Restrictions

Only built-in git format names are allowed: `oneline`, `short`, `medium`, `full`, `fuller`, `reference`, `email`, `raw`. Custom format strings (e.g. `format:%H`) are rejected to prevent exfiltration of sensitive repository metadata.

### git_commit Safety

- Commit message must not start with `-` (prevents flag injection).
- `files` and `all` cannot be used together.
- File paths must be relative and cannot contain `..` (path traversal protection).
- Absolute paths are rejected.

## Output Limits

All git commands capture stdout/stderr with a 1MB cap via `limitedBuffer`.

## Usage

```go
g := git.New(permStore, askFn, "/path/to/repo")
tb := g.Tools() // *toolbox.ToolBox with 4 git tools
```

## Dependencies

- `pkg/codingtoolbox/permissions` -- shared permissions store (command trust)
- `pkg/tools/toolbox` -- Tool and ToolBox types
