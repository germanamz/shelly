# exec

Package `exec` provides a tool that gives agents the ability to run CLI
commands with explicit user permission gating.

## Architecture

The `Exec` struct wraps a shared `permissions.Store` and an `AskFunc`. When
an agent invokes `exec_run`, the handler:

1. Checks whether the command (program name) is already trusted in the store.
2. If not trusted, prompts the user with three options:
   - **yes** — allow this single invocation.
   - **trust** — allow this command permanently (persisted to the trust file).
   - **no** — deny execution.
3. On approval, runs the command via `os/exec.CommandContext` and returns
   the combined stdout/stderr output.

Commands are executed directly (no shell interpretation). Arguments are passed
as separate strings, avoiding shell expansion vulnerabilities.

## Tool

| Name | Description |
|------|-------------|
| `exec_run` | Run a program with the given arguments. |

**Input schema:**

```json
{
  "command": "git",
  "args": ["status"]
}
```

## Trust Model

Trust is granted at the program level — trusting `git` allows all future
invocations of `git` regardless of arguments. This matches how users
typically think about command permissions.

Trusted commands are persisted alongside filesystem permissions in the shared
trust file managed by the `permissions` package.

## Usage

```go
store, _ := permissions.New(".shelly/permissions.json")
e := exec.New(store, responder.Ask)
tb := e.Tools() // register in your agent's toolbox
```
