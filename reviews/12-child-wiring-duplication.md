# Child-Wiring Duplicated Three Times in Delegation

## Severity: Moderate

## Location

- `pkg/agent/delegation.go:284-311` — `buildDelegateChild`
- `pkg/agent/delegation.go:545-578` — `buildInteractiveDelegateChild`
- `pkg/agent/delegation.go:509-527` — `handleHandoff`

## Description

All three locations independently perform the same 8-10 field assignments when spawning a child agent:

- Set instance name
- Assign `registry`
- Propagate `events.notifier`
- Wrap `events.eventFunc` with `delegationProgressFunc`
- Propagate `cancelRegistrar` / `cancelUnregistrar`
- Set `reflectionDir`
- Set `taskBoard`

The only differences: `buildInteractiveDelegateChild` uses `NewSharedInteractionChannel` and skips `searchReflections`.

## Fix

Extract a `propagateParentConfig(parent, child *Agent, t delegateTask)` helper called from all three sites.
