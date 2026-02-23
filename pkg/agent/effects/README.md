# effects

Package `effects` provides reusable `agent.Effect` implementations for the
agent's ReAct loop.

## Purpose

Effects are dynamic, per-iteration hooks that run inside an agent's ReAct loop
at two phases: **before** the LLM call (`PhaseBeforeComplete`) and **after** the
LLM reply (`PhaseAfterComplete`). They allow configuration-driven behaviours
such as context compaction, cost limits, progress tracking, and guardrails
without modifying the core loop.

## Architecture

Each effect implements the `agent.Effect` interface:

```go
type Effect interface {
    Eval(ctx context.Context, ic IterationContext) error
}
```

Effects receive an `IterationContext` containing the current phase, iteration
number, chat, completer, and agent name. Returning an error aborts the loop.

### Available Effects

| Effect | Kind | Description |
|--------|------|-------------|
| `CompactEffect` | `compact` | Summarises the conversation when token usage approaches the context window limit |

## Dependency Direction

`pkg/agent/effects/` imports `pkg/agent` (for the `Effect` interface and
`IterationContext`). The `pkg/agent` package never imports `pkg/agent/effects/`,
avoiding circular dependencies.

## Usage

Effects are typically constructed by the engine from YAML configuration:

```yaml
agents:
  - name: coder
    effects:
      - kind: compact
        params:
          threshold: 0.8
```

They can also be created programmatically:

```go
import "github.com/germanamz/shelly/pkg/agent/effects"

eff := effects.NewCompactEffect(effects.CompactConfig{
    ContextWindow: 200000,
    Threshold:     0.8,
})
```
