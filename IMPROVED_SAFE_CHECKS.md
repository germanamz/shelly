# Improved Safe Checks: Budget-Based Agent Controls

## Problem

`max_iterations` is a blunt instrument. A 20-iteration limit stops a coder agent mid-refactor the same as it stops a stuck agent looping infinitely. The team hits the limit on legitimate work because iteration count doesn't correlate with cost or progress.

## Design Principle

**Iteration limit becomes a safety net, not a constraint.** Real limits are token budgets, time budgets, and progress detection — all implemented as effects, fitting the existing architecture perfectly.

## Industry Context

| Framework | Default Limit | Unit |
|-----------|-------------:|------|
| Claude Code | None | No hard cap — relies on context window + user control |
| OpenAI Codex | None | Uses context compaction + per-job time limits |
| Vercel AI SDK | 20 | Steps (tool calls) |
| LangGraph | 25 | Graph nodes (~12 tool rounds) |
| LangChain AgentExecutor | 15 | Iterations |

The industry trend is moving away from hard iteration caps toward multi-layered controls: token budgets, time budgets, loop/stall detection, and context compaction.

---

## Phase 1: Token Budget Effect

**New effect: `token_budget`** (`pkg/agent/effects/token_budget.go`)

Tracks cumulative actual token usage via the existing `UsageReporter` interface on the completer. This is the primary cost control — it directly limits spend rather than proxying it through iteration count.

### Config

```yaml
- kind: token_budget
  params:
    max_tokens: 500000        # Hard cap on total tokens (input + output)
    warn_threshold: 0.8       # At 80%, inject "wrap up" message
```

### Behavior

- Runs at `PhaseAfterComplete` (needs actual usage from the LLM call)
- Calls `ic.Completer.(modeladapter.UsageReporter).UsageTracker().Total()` each iteration
- At `warn_threshold`: injects a user message — *"You've used 80% of your token budget. Prioritize completing the most critical remaining work."*
- At 100%: returns a new sentinel error `ErrTokenBudgetExhausted`
- Implements `Resetter` to clear warn-injected flag between runs

### Why tokens not dollars

Token counts are provider-agnostic and already tracked. Dollar costs require per-model pricing tables that change constantly. Users can mentally convert (e.g., Sonnet 4 at ~$3/M input tokens, so 500K tokens ~ $1.50).

### Implementation notes

- Uses actual usage from `UsageTracker`, not the heuristic `TokenEstimator` — accuracy matters for budgets
- The warn message is injected once (guard flag), not every iteration
- `IterationContext` already has `Completer` — no new fields needed

---

## Phase 2: Time Budget Effect

**New effect: `time_budget`** (`pkg/agent/effects/time_budget.go`)

Tracks cumulative LLM inference time only — excludes tool execution, user response time, and network latency. Catches agents that are consuming excessive compute without producing results.

### Config

```yaml
- kind: time_budget
  params:
    max_duration: "15m"       # 15 min of actual LLM compute time
    warn_threshold: 0.8       # At 80% (12 min), inject "wrap up" message
```

### Behavior

- Runs at both phases to measure LLM call duration:
  - `PhaseBeforeComplete`: records `callStart = time.Now()`
  - `PhaseAfterComplete`: accumulates `elapsed += time.Since(callStart)`, then checks thresholds
- At `warn_threshold`: injects wrap-up message (once)
- At 100%: returns `ErrTimeBudgetExhausted`
- Implements `Resetter` to clear accumulated time between runs

### What is measured vs excluded

| Measured (LLM inference) | Excluded |
|--------------------------|----------|
| Time inside `completer.Complete()` | Tool execution time |
| | User response time (`ask_user`) |
| | MCP server latency |
| | Network delays |

The effect naturally captures only LLM time because tool execution happens *after* `PhaseAfterComplete` and before the next iteration's `PhaseBeforeComplete`.

### Calibration

Values are calibrated for pure inference time. A single Sonnet 4 call typically takes 5-30s, so 15 minutes of inference ~ 30-180 LLM calls — much more meaningful than wall-clock time.

---

## Phase 3: Stall Detection Effect

**New effect: `stall_detect`** (`pkg/agent/effects/stall_detect.go`)

The existing `loop_detect` catches identical consecutive tool calls. `stall_detect` is broader — it catches agents that are *active but not progressing*: calling different tools but getting the same errors, reading the same files, or producing no meaningful output.

### Config

```yaml
- kind: stall_detect
  params:
    window: 6                 # Look at last N iterations
    similarity_threshold: 0.8 # Fraction of "wasted" iterations to trigger
```

### Behavior

- Runs at `PhaseBeforeComplete`, iteration > 0
- Tracks a rolling window of iteration "fingerprints":
  - Tool names called
  - Whether tool results were errors
  - Hash of tool result content (truncated)
- If `similarity_threshold` fraction of the window produces duplicate fingerprints, inject intervention: *"You appear stalled. The last N iterations produced similar results. Step back and reconsider your approach."*
- Escalates: first intervention is a nudge, second intervention after another `window` iterations of stall returns `ErrStallDetected`

### Implementation notes

- Fingerprint is lightweight: `toolName + isError + hash(first 200 chars of result)`
- Complements `loop_detect` — loop_detect catches exact repetition, stall_detect catches semantic stall
- Implements `Resetter`

---

## Phase 4: Graceful Exhaustion Protocol

**Change: Replace abrupt `ErrMaxIterations` with a two-phase shutdown.**

Currently when `max_iterations` is hit, the agent just stops and returns an error. Add a warning phase.

### Modifications to `pkg/agent/agent.go`

New `Options` field:

```go
WarnIterations int // Inject wrap-up message at this iteration (0 = no warning)
```

At iteration `WarnIterations`, inject a system message: *"You are approaching your iteration limit. Complete your current task and call task_complete."*

### Config

```yaml
options:
  max_iterations: 200         # Safety net (raised from 20)
  warn_iterations: 150        # Soft warning
```

---

## Phase 5: Raise Default Limits & Update Templates

### New defaults

| Agent | Current `max_iterations` | New `max_iterations` | Budget effects added |
|-------|------------------------:|---------------------:|---------------------|
| lead | 20 | 100 | `token_budget: 1000000`, `time_budget: 30m` |
| explorer | 10 | 50 | `token_budget: 500000`, `time_budget: 15m` |
| planner | 10 | 30 | `token_budget: 300000`, `time_budget: 10m` |
| coder | 20 | 200 | `token_budget: 1000000`, `time_budget: 30m`, `stall_detect` |
| reviewer | 10 | 50 | `token_budget: 500000`, `time_budget: 15m` |

The iteration limit is now 5-10x higher — it only fires if all other safety nets fail. The real constraints are the budget effects.

### Files to update

- `cmd/shelly/internal/templates/settings/dev-team.yaml`
- `cmd/shelly/internal/templates/settings/simple-assistant.yaml`
- `cmd/shelly/internal/templates/settings/project-indexer.yaml`
- `pkg/shellydir/init.go` (default config template)
- `ARCHITECTURE.md` (examples)

---

## Phase 6: Wire Everything in Engine

### New effect factories in `pkg/engine/effects.go`

```go
var effectFactories = map[string]EffectFactory{
    // ... existing ...
    "token_budget": buildTokenBudgetEffect,
    "time_budget":  buildTimeBudgetEffect,
    "stall_detect": buildStallDetectEffect,
}
```

### New config fields in `AgentOptions`

```go
type AgentOptions struct {
    MaxIterations      int     `yaml:"max_iterations"`
    WarnIterations     int     `yaml:"warn_iterations"`     // NEW
    MaxDelegationDepth int     `yaml:"max_delegation_depth"`
    // ... rest unchanged ...
}
```

### Auto-generated effects update

When no explicit effects are configured, add `token_budget` and `time_budget` to the auto-generated defaults alongside `trim_tool_results`, `observation_mask`, and `compact`. This ensures even unconfigured agents get budget protection.

---

## New Sentinel Errors

```go
var ErrTokenBudgetExhausted = errors.New("agent: token budget exhausted")
var ErrTimeBudgetExhausted  = errors.New("agent: time budget exhausted")
var ErrStallDetected        = errors.New("agent: stall detected")
```

These integrate with the existing delegation error handling in `pkg/agent/delegation.go:404` — the same pattern that catches `ErrMaxIterations` and produces a synthetic `CompletionResult`.

---

## What Stays the Same

- `max_iterations` field — kept as a safety net, not removed
- `ErrMaxIterations` — still exists, just rarely hit
- All existing effects — unchanged, composable with new ones
- `Effect` interface — no changes needed
- `IterationContext` — no changes needed (already has `Completer` for usage access)

---

## Implementation Order

1. ~~**Phase 1** (token_budget) — Highest impact, replaces iteration limit as primary cost control~~ **DONE**
2. **Phase 2** (time_budget) — Quick to implement, shares pattern with token_budget
3. **Phase 4** (graceful exhaustion) — Small change, big UX improvement
4. **Phase 5** (raise limits + update templates) — Applies the new effects to real configs
5. **Phase 6** (engine wiring) — Factories and auto-generation
6. **Phase 3** (stall_detect) — Most complex, lowest priority since loop_detect already covers the worst case
