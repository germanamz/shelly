# exec

Package `exec` provides a tool that gives agents the ability to run CLI
commands with explicit user permission gating.

## Architecture

The **`Exec`** struct wraps a shared `permissions.Store` and an `AskFunc`. When
an agent invokes `exec_run`, the handler:

1. Checks whether the command (program name) is already trusted in the store.
2. If not trusted, prompts the user with three options:
   - **yes** -- allow this single invocation.
   - **trust** -- allow this command permanently (persisted to the trust file).
   - **no** -- deny execution.
3. On approval, runs the command via `os/exec.CommandContext` and returns
   the combined stdout/stderr output (capped at 1MB via `limitedBuffer`).

Commands are executed directly (no shell interpretation). Arguments are passed
as separate strings, avoiding shell expansion vulnerabilities.

Concurrent permission prompts for the same command are coalesced via a
`pendingResult` map so the user is never asked twice for the same program
simultaneously.

## Exported API

### Types

- **`Exec`** -- provides command execution tools with permission gating.
- **`AskFunc`** -- `func(ctx context.Context, question string, options []string) (string, error)` callback for permission prompts.
- **`OnExecFunc`** -- `func(ctx context.Context, display string)` non-blocking callback invoked when a trusted command is about to execute, giving the frontend an opportunity to display what is being run.
- **`Option`** -- functional option for configuring Exec behaviour.

### Functions

- **`New(store *permissions.Store, askFn AskFunc, opts ...Option) *Exec`** -- creates an Exec backed by the given permissions store.
- **`WithOnExec(fn OnExecFunc) Option`** -- sets a callback invoked when a trusted command is about to execute.

### Methods on Exec

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox containing the `exec_run` tool.

## Tool

| Name | Description |
|------|-------------|
| `exec_run` | Run a program with the given arguments. For git operations, prefer the dedicated git tools. |

**Input schema:**

```json
{
  "command": "git",
  "args": ["status"]
}
```

## Trust Model

Trust is granted at the program level -- trusting `git` allows all future
invocations of `git` regardless of arguments. This matches how users
typically think about command permissions.

Trusted commands are persisted alongside filesystem permissions in the shared
trust file managed by the `permissions` package.

## Usage

```go
store, _ := permissions.New(".shelly/local/permissions.json")
e := exec.New(store, responder.Ask, exec.WithOnExec(func(ctx context.Context, display string) {
    fmt.Println("Running:", display)
}))
tb := e.Tools() // register in your agent's toolbox
```

## Dependencies

- `pkg/codingtoolbox/permissions` -- shared permissions store (command trust)
- `pkg/tools/toolbox` -- Tool and ToolBox types
