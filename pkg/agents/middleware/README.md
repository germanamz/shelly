# middleware

Composable middleware for `agents.Agent`. Each middleware wraps an Agent, returning a new Agent with added behaviour. Middleware composes via `Chain` or `Apply`.

## Architecture

```
middleware/
├── middleware.go        Middleware type, Chain, Apply, built-in middleware
└── middleware_test.go   Tests for all middleware and composition
```

### Core types

```go
type Middleware func(next agents.Agent) agents.Agent

func Chain(mws ...Middleware) Middleware   // compose; first = outermost
func Apply(agent agents.Agent, mws ...Middleware) agents.Agent
```

### Built-in middleware

| Middleware | Description |
|---|---|
| `Timeout(d)` | Wraps context with a deadline |
| `Recovery()` | Catches panics, converts to errors |
| `Logger(log)` | Logs agent name, start time, duration, error |
| `OutputGuardrail(check)` | Validates the final message; returns error if check fails |

### NamedAgent preservation

Every middleware wrapper implements `reactor.NamedAgent`. If the inner agent also implements `NamedAgent`, `AgentName()` and `AgentChat()` delegate through. Otherwise they return zero values.

## Examples

### Apply middleware to an agent

```go
wrapped := middleware.Apply(myAgent,
    middleware.Recovery(),
    middleware.Logger(slog.Default()),
    middleware.Timeout(30 * time.Second),
    middleware.OutputGuardrail(func(m message.Message) error {
        if strings.Contains(m.TextContent(), "forbidden") {
            return errors.New("output rejected")
        }
        return nil
    }),
)

reply, err := wrapped.Run(ctx)
```

### Chain middleware for reuse

```go
standard := middleware.Chain(
    middleware.Recovery(),
    middleware.Logger(log),
    middleware.Timeout(time.Minute),
)

a := standard(agentA)
b := standard(agentB)
```
