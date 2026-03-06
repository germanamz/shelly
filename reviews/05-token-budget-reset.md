# `TokenBudgetEffect.Reset()` Does Not Reset Usage Baseline

## Severity: Important

## Location

- `pkg/agent/effects/token_budget.go:39-41` — `Reset()` method

## Description

`Reset()` only clears `e.warned`. The effect computes usage from `reporter.UsageTracker().Total()`, which is the completer's lifetime cumulative total — not a per-run count.

For a long-lived session agent that calls `Run()` multiple times, the second run starts with all tokens from prior runs already counted. A run that begins when the cumulative total is already past `WarnThreshold * MaxTokens` will inject the wrap-up message on iteration 0 and may immediately return `ErrTokenBudgetExhausted`.

`TimeBudgetEffect.Reset()` correctly resets `e.elapsed = 0` for per-run semantics; `TokenBudgetEffect` lacks the equivalent.

## Fix

Snapshot `UsageTracker().Total()` in `Reset()` and subtract it as a baseline when computing `used`:

```go
func (e *TokenBudgetEffect) Reset() {
    e.warned = false
    if reporter, ok := e.completer.(modeladapter.UsageReporter); ok {
        e.baseline = reporter.UsageTracker().Total()
    }
}
```

Then in the effect logic, compute `used = current.Total() - e.baseline.Total()`.
