# Unregistered Effects: `time_budget` and `stall_detect`

## Severity: Critical

## Location

- `pkg/engine/effects.go:27-38` — `effectFactories` map missing both entries
- `pkg/agent/effects/time_budget.go` — full implementation exists
- `pkg/agent/effects/stall_detect.go` — full implementation exists

## Description

`TimeBudgetEffect` and `StallDetectEffect` have full implementations and tests (commits `296722c`, `2fd4ac6`, `e3eab82` — the three most recent), but neither is registered in the engine's `effectFactories` map. Any YAML config using `kind: time_budget` or `kind: stall_detect` will fail `Validate()` with "unknown kind". `KnownEffectKinds()` will not list them. They are dead code from the user's perspective.

## Fix

1. Add `"time_budget": buildTimeBudgetEffect` and `"stall_detect": buildStallDetectEffect` to `effectFactories`.
2. Implement both builder functions following the pattern of `buildTokenBudgetEffect`.
3. Add type assertions for both in `effectPriority` if they belong to the compaction-priority class.
4. Update the engine README table to document the two new effect kinds.
