# agentctx

Shared context key helpers for propagating agent identity across package boundaries.

## Architecture

```
agentctx/
└── context.go   WithAgentName / AgentNameFromContext
```

This package is intentionally zero-dependency so both `pkg/agent` and `pkg/engine` can import it without creating import cycles.

## API

```go
ctx = agentctx.WithAgentName(ctx, "worker-1")
name := agentctx.AgentNameFromContext(ctx) // "worker-1"
```

`AgentNameFromContext` returns `""` if no agent name is present.

## Why

The agent name was previously stored via a private context key in `pkg/engine/session.go`. Child agents spawned via delegation carried their parent's name because the key was never overwritten. This package lets `pkg/agent` inject the correct agent name into the context at the start of each `run()` call.
