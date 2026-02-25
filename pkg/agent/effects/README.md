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
| `CompactEffect` | `compact` | BeforeComplete | Full conversation summarisation when token usage exceeds threshold |
| `TrimToolResultsEffect` | `trim_tool_results` | AfterComplete | Trims old tool result content to a configurable length, preserving recent messages |
| `SlidingWindowEffect` | `sliding_window` | BeforeComplete | Three-zone context management with incremental summarisation |
| `ObservationMaskEffect` | `observation_mask` | BeforeComplete | Replaces old tool results with brief placeholders while keeping reasoning intact |
| `ReflectionEffect` | `reflection` | BeforeComplete | Detects consecutive tool failures and injects a reflection prompt |
| `ProgressEffect` | `progress` | BeforeComplete | Periodically prompts the agent to write a progress note |
| `LoopDetectEffect` | `loop_detect` | BeforeComplete | Detects repeated identical tool calls and injects an intervention |

### CompactEffect — Full Summarisation

Runs at `PhaseBeforeComplete` (iteration > 0). When token usage exceeds the
threshold, renders the conversation as a text transcript and summarises it via
an LLM call. The chat is replaced with the system prompt + a single compacted
user message containing the summary.

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | float | 0.8 | Fraction of context window that triggers compaction |

```yaml
- kind: compact
  params:
    threshold: 0.8
```

### TrimToolResultsEffect — Tool Result Trimming

Runs at `PhaseAfterComplete` (iteration > 0). Replaces long `ToolResult` content
with truncated versions, preserving the most recent tool messages untrimmed.
Error results are never trimmed.

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `max_result_length` | int | 500 | Maximum runes for tool result content |
| `preserve_recent` | int | 4 | Number of recent tool-role messages to keep untrimmed (0 = trim all) |

```yaml
- kind: trim_tool_results
  params:
    max_result_length: 500
    preserve_recent: 4
```

### SlidingWindowEffect — Three-Zone Context Management

Runs at `PhaseBeforeComplete` (iteration > 0). Divides messages into three zones:

1. **Recent zone** (last N messages): full fidelity
2. **Medium zone** (next M messages before recent): tool results trimmed, text preserved
3. **Old zone** (everything before medium): incrementally summarised into a running summary

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | float | 0.7 | Fraction of context window that triggers window management |
| `recent_zone` | int | 10 | Messages kept at full fidelity |
| `medium_zone` | int | 10 | Messages where tool results are trimmed |
| `trim_length` | int | 200 | Max runes for tool results in the medium zone |

```yaml
- kind: sliding_window
  params:
    threshold: 0.7
    recent_zone: 10
    medium_zone: 10
    trim_length: 200
```

### ObservationMaskEffect — Observation Masking

Runs at `PhaseBeforeComplete` (iteration > 0). Replaces old tool result content
with brief placeholders (`[tool result for <name>: <preview>]`) while keeping
assistant reasoning (text) and actions (tool calls) intact. A lightweight first
tier before heavier compaction.

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | float | 0.6 | Fraction of context window that triggers masking |
| `recent_window` | int | 10 | Messages to keep at full fidelity |

```yaml
- kind: observation_mask
  params:
    threshold: 0.6
    recent_window: 10
```

### ReflectionEffect — Failure Reflection

Runs at `PhaseBeforeComplete` (iteration > 0). Counts consecutive error-only
tool messages from the tail of the chat. When the count reaches the threshold,
injects a reflection prompt asking the agent to analyse root causes. Includes a
re-injection guard to avoid injecting the same prompt repeatedly at the same
failure count.

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `failure_threshold` | int | 2 | Consecutive failures before reflection is injected |

```yaml
- kind: reflection
  params:
    failure_threshold: 2
```

### ProgressEffect — Progress Notes

Runs at `PhaseBeforeComplete` (iteration > 0). Every N iterations, injects a
prompt asking the agent to write a progress note via `write_note`, documenting
accomplishments, remaining work, and blockers.

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `interval` | int | 5 | Inject progress prompt every N iterations |

```yaml
- kind: progress
  params:
    interval: 5
```

### LoopDetectEffect — Loop Detection

Runs at `PhaseBeforeComplete` (iteration > 0). Scans a sliding window of recent
tool calls for consecutive identical calls (same tool name + arguments). When
the count reaches the threshold, injects an intervention message asking the
agent to try a different approach.

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | int | 3 | Identical calls before intervention |
| `window_size` | int | 10 | Number of recent tool calls to track |

```yaml
- kind: loop_detect
  params:
    threshold: 3
    window_size: 10
```

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
