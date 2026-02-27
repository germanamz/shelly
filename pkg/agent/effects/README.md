# effects

Package `effects` provides reusable `agent.Effect` implementations for the
agent's ReAct loop.

## Purpose

Effects are dynamic, per-iteration hooks that run inside an agent's ReAct loop
at two phases: **before** the LLM call (`PhaseBeforeComplete`) and **after** the
LLM reply (`PhaseAfterComplete`). They allow configuration-driven behaviours
such as context compaction, tool result trimming, observation masking, loop
detection, failure reflection, and progress tracking without modifying the core
loop.

## Architecture

Each effect implements the `agent.Effect` interface:

```go
type Effect interface {
    Eval(ctx context.Context, ic IterationContext) error
}
```

Effects receive an `IterationContext` containing the current phase, iteration
number, chat, completer, and agent name. Returning an error aborts the loop.

Effects that track per-run state (e.g. injection guards, counters) implement
`agent.Resetter` so their state is cleared between `Run()` calls on long-lived
agents:

```go
type Resetter interface {
    Reset()
}
```

## Available Effects

| Effect | Kind | Phase | Resetter | Description |
|--------|------|-------|----------|-------------|
| `CompactEffect` | `compact` | BeforeComplete | No | Full conversation summarisation when token usage exceeds threshold |
| `TrimToolResultsEffect` | `trim_tool_results` | AfterComplete | No | Trims old tool result content to a configurable length, preserving recent messages |
| `SlidingWindowEffect` | `sliding_window` | BeforeComplete | No | Three-zone context management with incremental summarisation |
| `ObservationMaskEffect` | `observation_mask` | BeforeComplete | No | Replaces old tool results with brief placeholders while keeping reasoning intact |
| `LoopDetectEffect` | `loop_detect` | BeforeComplete | Yes | Detects repeated identical tool calls and injects an intervention |
| `ReflectionEffect` | `reflection` | BeforeComplete | Yes | Detects consecutive tool failures and injects a reflection prompt |
| `ProgressEffect` | `progress` | BeforeComplete | No | Periodically prompts the agent to write a progress note |

### CompactEffect -- Full Summarisation

Runs at `PhaseBeforeComplete` (iteration > 0). When token usage (from the last
LLM call's `InputTokens`, obtained via `modeladapter.UsageReporter`) exceeds
`ContextWindow * Threshold`, renders the entire conversation as a text
transcript and summarises it via a separate LLM call. The chat is replaced
with the system prompt + a single compacted user message containing the
structured summary. The summary uses a fixed format with sections for Goal,
Completed Work, Files Touched, Key Decisions, Errors & Blockers, Current State,
and Next Steps.

On compaction failure, if `AskFunc` is provided the user is prompted to continue
or retry. Context errors (cancellation, deadline) are always propagated. Without
`AskFunc`, failures are silently swallowed.

**Config:**

```go
type CompactConfig struct {
    ContextWindow int     // Provider's context window size.
    Threshold     float64 // Fraction triggering compaction (e.g. 0.8).
    AskFunc       func(ctx context.Context, text string, options []string) (string, error)
    NotifyFunc    func(ctx context.Context, message string)
}
```

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | float | -- | Fraction of context window that triggers compaction |

```yaml
- kind: compact
  params:
    threshold: 0.8
```

### TrimToolResultsEffect -- Tool Result Trimming

Runs at `PhaseAfterComplete`. Replaces long `ToolResult` content in older tool
messages with truncated versions, preserving the most recent N tool-role messages
untrimmed. Error results (`IsError: true`) are never trimmed. Uses metadata
tagging (`trimmed` key) to avoid re-trimming already-processed messages.

**Config:**

```go
type TrimToolResultsConfig struct {
    MaxResultLength int // Max chars for tool result content (default: 500).
    PreserveRecent  int // Keep last N tool-role messages untrimmed (default: 4).
}
```

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `max_result_length` | int | 500 | Maximum runes for tool result content |
| `preserve_recent` | int | 4 | Number of recent tool-role messages to keep untrimmed |

```yaml
- kind: trim_tool_results
  params:
    max_result_length: 500
    preserve_recent: 4
```

### SlidingWindowEffect -- Three-Zone Context Management

Runs at `PhaseBeforeComplete` (iteration > 0). When token usage exceeds the
threshold, divides non-system messages into three zones:

1. **Recent zone** (last N messages): full fidelity, untouched.
2. **Medium zone** (next M messages before recent): tool results trimmed to
   `TrimLength`, text preserved. Uses `sw_trimmed` metadata to avoid
   re-trimming. Error results are never trimmed.
3. **Old zone** (everything before medium): incrementally summarised into a
   running summary block via a separate LLM call. Old messages are removed
   and replaced by a `[Context summary ...]` user message.

The running summary is accumulated across multiple evictions: each time old
messages are evicted, the LLM is asked to update the existing summary with new
information. If summarisation fails, old messages are retained and no trimming
is performed (graceful degradation).

The effect is thread-safe -- the running summary is protected by a mutex, and
the LLM call is performed outside the lock.

**Config:**

```go
type SlidingWindowConfig struct {
    ContextWindow int     // Provider's context window size.
    Threshold     float64 // Fraction triggering window management (e.g. 0.7).
    RecentZone    int     // Messages kept at full fidelity (default: 10).
    MediumZone    int     // Messages where tool results are trimmed (default: 10).
    TrimLength    int     // Max runes for tool results in the medium zone (default: 200).
    NotifyFunc    func(ctx context.Context, message string)
}
```

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | float | -- | Fraction of context window that triggers window management |
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

### ObservationMaskEffect -- Observation Masking

Runs at `PhaseBeforeComplete` (iteration > 0). When token usage exceeds the
threshold, replaces old tool result content with brief placeholders of the form
`[tool result for <name>: <preview>]` while keeping assistant reasoning (text)
and actions (tool calls) intact. Error results are never masked. Uses
`obs_masked` metadata to skip already-processed messages. The preview is
truncated to 80 runes.

A lightweight first tier before heavier compaction effects.

**Config:**

```go
type ObservationMaskConfig struct {
    ContextWindow int     // Provider's context window size.
    Threshold     float64 // Fraction triggering masking (default: 0.6).
    RecentWindow  int     // Messages to keep at full fidelity (default: 10).
}
```

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

### LoopDetectEffect -- Loop Detection

Runs at `PhaseBeforeComplete` (iteration > 0). Scans a sliding window of recent
tool calls (from assistant messages, scanning from the end) for consecutive
identical calls (same tool name + same arguments). When the count reaches the
threshold, injects an intervention message asking the agent to try a different
approach or tool.

Implements `agent.Resetter` to clear the injection guard between runs. The
re-injection guard ensures the intervention message is only injected once per
count increase, preventing repeated interventions at the same failure count.

**Config:**

```go
type LoopDetectConfig struct {
    Threshold  int // Consecutive identical calls before intervention (default: 3).
    WindowSize int // Sliding window of tool calls to track (default: 10).
}
```

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

### ReflectionEffect -- Failure Reflection

Runs at `PhaseBeforeComplete` (iteration > 0). Counts consecutive error-only
tool messages from the tail of the chat (skipping assistant messages between
tool results, stopping at user messages or successful tool results). When the
count reaches the threshold, injects a reflection prompt asking the agent to
analyse root causes and describe a different strategy.

Implements `agent.Resetter` to clear the injection guard between runs. Includes
a re-injection guard so the same prompt is not injected repeatedly at the same
failure count -- it only triggers when the count exceeds the last injected count.

**Config:**

```go
type ReflectionConfig struct {
    FailureThreshold int // Consecutive failures before reflection (default: 2).
}
```

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `failure_threshold` | int | 2 | Consecutive failures before reflection is injected |

```yaml
- kind: reflection
  params:
    failure_threshold: 2
```

### ProgressEffect -- Progress Notes

Runs at `PhaseBeforeComplete` (iteration > 0). Every N iterations (when
`iteration % interval == 0`), injects a prompt asking the agent to write a
progress note via `write_note`, documenting accomplishments, remaining work,
and blockers. Only activates when `HasNotesTool` is true in the config.

**Config:**

```go
type ProgressConfig struct {
    Interval     int  // Inject progress prompt every N iterations (default: 5).
    HasNotesTool bool // Whether the write_note tool is available.
}
```

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `interval` | int | 5 | Inject progress prompt every N iterations |

```yaml
- kind: progress
  params:
    interval: 5
```

## Shared Helpers

The package includes internal helpers used by multiple effects:

- `truncate(s string, maxLen int) string` -- Truncates to maxLen runes, appending an ellipsis if needed. Correctly handles multi-byte UTF-8 characters.
- `renderConversation(c *chat.Chat) string` -- Converts chat messages into a compact text transcript, skipping system messages. Tool call arguments are truncated to 200 runes, tool results to 500 runes.
- `renderMessages(msgs []message.Message) string` -- Same as `renderConversation` but operates on a message slice.

## Dependency Direction

`pkg/agent/effects/` imports `pkg/agent` (for the `Effect` interface,
`Resetter` interface, `IterationContext`, and `IterationPhase`). The `pkg/agent`
package never imports `pkg/agent/effects/`, avoiding circular dependencies.

Other dependencies:

- `pkg/chats/` -- chat, message, content, role types
- `pkg/modeladapter/` -- `Completer`, `UsageReporter`, and `usage.Tracker` for token-aware effects

## Usage

Effects are typically constructed by the engine from YAML configuration:

```yaml
agents:
  - name: coder
    effects:
      - kind: observation_mask
        params:
          threshold: 0.6
          recent_window: 10
      - kind: trim_tool_results
        params:
          max_result_length: 500
          preserve_recent: 4
      - kind: compact
        params:
          threshold: 0.8
      - kind: loop_detect
        params:
          threshold: 3
      - kind: reflection
        params:
          failure_threshold: 2
      - kind: progress
        params:
          interval: 5
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

slidingEff := effects.NewSlidingWindowEffect(effects.SlidingWindowConfig{
    ContextWindow: 200000,
    Threshold:     0.7,
    RecentZone:    10,
    MediumZone:    10,
    TrimLength:    200,
})

obsMaskEff := effects.NewObservationMaskEffect(effects.ObservationMaskConfig{
    ContextWindow: 200000,
    Threshold:     0.6,
    RecentWindow:  10,
})

loopEff := effects.NewLoopDetectEffect(effects.LoopDetectConfig{
    Threshold:  3,
    WindowSize: 10,
})

reflectEff := effects.NewReflectionEffect(effects.ReflectionConfig{
    FailureThreshold: 2,
})

progressEff := effects.NewProgressEffect(effects.ProgressConfig{
    Interval:     5,
    HasNotesTool: true,
})
```
