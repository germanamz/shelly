# git

Package git provides tools that give agents controlled access to git operations.

## Permission Model

Command execution is gated by the shared permissions store using the command trust model. Users can "trust" git to allow all future git operations without being prompted.

## Tools

| Tool | Description |
|------|-------------|
| `git_status` | Show the working tree status |
| `git_diff` | Show changes between commits and working tree |
| `git_log` | Show commit logs |
| `git_commit` | Create a git commit, optionally staging files first |

## Usage

```go
g := git.New(permStore, askFn, "/path/to/repo")
tb := g.Tools() // *toolbox.ToolBox with git tools
```

The `workDir` parameter sets the working directory for all git subprocess invocations.
