# effects

Package `effects` provides reusable `agent.Effect` implementations for the
agent's ReAct loop.

## Purpose

Effects are dynamic, per-iteration hooks that run inside an agent's ReAct loop
at two phases: **before** the LLM call (`PhaseBeforeComplete`) and **after** the
LLM reply (`PhaseAfterComplete`). They allow configuration-driven behaviours
such as context compaction, tool result trimming, cost limits, progress tracking,
and guardrails without modifying the core loop.

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

| Effect | Kind | Phase | Description |
|--------|------|-------|-------------|
| `CompactEffect` | `compact` | BeforeComplete | Graduated context compaction: first trims old tool results (lightweight), then falls back to full summarisation |
| `TrimToolResultsEffect` | `trim_tool_results` | AfterComplete | Trims old tool result content to a configurable length, preserving recent messages |

### CompactEffect — Graduated Context Compaction

Runs at `PhaseBeforeComplete` (iteration > 0). Uses a two-phase graduated approach:

1. **Phase 1 (lightweight)**: Trims old tool result content in messages beyond the last 6, replacing long results with truncated versions. If this brings token usage below the threshold, no further action is taken.
2. **Phase 2 (full summarisation)**: If still over threshold after trimming, renders the conversation as a text transcript and summarises it via an LLM call. The chat is replaced with the system prompt + a single compacted user message containing the summary.

Configuration:

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | float | 0.8 | Fraction of context window that triggers compaction |

### TrimToolResultsEffect — Tool Result Trimming

Runs at `PhaseAfterComplete` (iteration > 0). Replaces long `ToolResult` content with truncated versions, preserving the most recent tool messages untrimmed. Error results are never trimmed.

Uses message metadata (`"trimmed"` key) to track already-trimmed messages and avoid re-processing.

Configuration:

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `max_result_length` | int | 500 | Maximum characters for tool result content |
| `preserve_recent` | int | 4 | Number of recent tool-role messages to keep untrimmed (0 = trim all) |

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
      - kind: trim_tool_results
        params:
          max_result_length: 500
          preserve_recent: 4
      - kind: compact
        params:
          threshold: 0.8
```

When the effective context window is non-zero and no explicit effects are configured,
the engine auto-generates both `trim_tool_results` and `compact` effects. Known
provider kinds (anthropic, openai, grok) have built-in default context windows, so
compaction is active by default for these providers even without an explicit
`context_window` setting.

They can also be created programmatically:

```go
import "github.com/germanamz/shelly/pkg/agent/effects"

trimEff := effects.NewTrimToolResultsEffect(effects.TrimToolResultsConfig{
    MaxResultLength: 500,
    PreserveRecent:  4,
})

compactEff := effects.NewCompactEffect(effects.CompactConfig{
    ContextWindow: 200000,
    Threshold:     0.8,
})
```
