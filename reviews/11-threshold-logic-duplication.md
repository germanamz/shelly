# Threshold Logic Duplicated Across Four Effects

## Severity: Moderate

## Location

- `pkg/agent/effects/compact.go:78-101` — `shouldCompact`
- `pkg/agent/effects/sliding_window.go:119-141` — `shouldManage`
- `pkg/agent/effects/observation_mask.go:66-88` — `shouldMask`
- `pkg/agent/effects/offload.go:129-152` — `shouldOffload`

## Description

All four methods implement identical logic:

1. Validate `ContextWindow > 0 && Threshold > 0`
2. Compute `limit = int(float64(ContextWindow) * Threshold)`
3. Return `true` if `estimatedTokens >= limit`
4. Fall back to `UsageReporter.Last().InputTokens >= limit`

A bug fix or heuristic change must be applied in four places.

## Fix

Extract a shared helper in the `effects` package:

```go
func exceedsThreshold(completer modeladapter.Completer, estimatedTokens, contextWindow int, threshold float64) bool {
    if contextWindow <= 0 || threshold <= 0 {
        return false
    }
    limit := int(float64(contextWindow) * threshold)
    if estimatedTokens > 0 {
        return estimatedTokens >= limit
    }
    if ur, ok := completer.(modeladapter.UsageReporter); ok {
        if last, ok := ur.UsageTracker().Last(); ok {
            return last.InputTokens >= limit
        }
    }
    return false
}
```
