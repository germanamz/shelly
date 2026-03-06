# System Prompt Self-Exclusion Uses Exact-Match Instead of Case-Insensitive

## Severity: Important

## Location

- `pkg/agent/prompt.go:172`
- `pkg/agent/delegation.go:56, 109` (for comparison)

## Description

`build()` in `prompt.go` filters the agent registry with `e.Name != pb.ConfigName` (exact string match). The `list_agents` tool handler at `delegation.go:56` and the self-delegation guard at `delegation.go:109` both use `strings.EqualFold`.

If a registry key and `configName` differ only in case, the agent appears in its own `<available_agents>` system prompt section but is correctly blocked by the runtime delegation tools. This is inconsistent.

## Fix

Change `prompt.go:172` to:

```go
if !strings.EqualFold(e.Name, pb.ConfigName) {
```
