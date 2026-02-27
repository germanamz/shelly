# agentctx

Shared context-key helpers for propagating agent identity across package boundaries.

## Purpose

In a multi-agent system, many packages need to know *which* agent is currently executing -- for logging, task attribution, event routing, and similar concerns. Storing the agent name in `context.Context` is the natural Go idiom, but the context key must be defined in a package that all consumers can import without creating cycles. `agentctx` fills that role: it is intentionally **zero-dependency** (no imports from other `pkg/` packages), so `pkg/agent`, `pkg/engine`, `pkg/tasks`, and any future package can all safely depend on it.

## Architecture

```
agentctx/
├── context.go        # WithAgentName / AgentNameFromContext
├── context_test.go   # round-trip, empty-context, and overwrite tests
└── README.md
```

The package exposes a single unexported context-key type (`agentNameCtxKey`) and two exported functions that wrap `context.WithValue` / `context.Value`.

## Exported API

### Functions

| Function | Signature | Description |
|---|---|---|
| `WithAgentName` | `func WithAgentName(ctx context.Context, name string) context.Context` | Returns a child context carrying the given agent name. Calling it again on the same context chain overwrites the previous value. |
| `AgentNameFromContext` | `func AgentNameFromContext(ctx context.Context) string` | Extracts the agent name from the context. Returns `""` if no agent name has been set. |

### Usage

```go
// Store the agent name at the start of an agent run.
ctx = agentctx.WithAgentName(ctx, "worker-1")

// Retrieve it anywhere downstream.
name := agentctx.AgentNameFromContext(ctx) // "worker-1"

// Overwrite for a child agent (the parent value is shadowed).
ctx = agentctx.WithAgentName(ctx, "child-agent")
agentctx.AgentNameFromContext(ctx) // "child-agent"
```

## Consumers

- **`pkg/agent`** -- calls `WithAgentName` at the start of each `run()` call so every tool invocation and effect within that iteration sees the correct agent name.
- **`pkg/engine`** -- calls `WithAgentName` when starting a session and reads the name via `AgentNameFromContext` for event routing and logging.
- **`pkg/tasks`** -- reads the agent name with `AgentNameFromContext` to attribute task creation and to determine which agent is claiming a task.

## Dependencies

None. This package imports only the standard library (`context`). It has no dependencies on other `pkg/` packages, which is a deliberate design choice to prevent import cycles.

## Background

The agent name was previously stored via a private context key inside `pkg/engine/session.go`. Child agents spawned through delegation inherited their parent's name because the key was never overwritten. Extracting the key into this standalone package lets `pkg/agent` inject the correct agent name into the context at the start of each `run()` call, ensuring accurate identity propagation throughout the entire call chain.
