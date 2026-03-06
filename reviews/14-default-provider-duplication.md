# Default-Provider Resolution Duplicated Three Times

## Severity: Moderate

## Location

- `pkg/engine/registration.go:191-194` — `resolveCompleter`
- `pkg/engine/registration.go:257-260` — `resolveAgentContextWindow`
- `pkg/engine/engine.go:269-277` — `resolveProviderInfo`

## Description

The identical pattern — "use `ac.Provider`, fall back to `e.cfg.Providers[0].Name` if empty" — is copy-pasted across all three sites.

## Fix

Extract into a single unexported helper:

```go
func (e *Engine) agentProviderName(agentProvider string) string {
    if agentProvider != "" {
        return agentProvider
    }
    if len(e.cfg.Providers) > 0 {
        return e.cfg.Providers[0].Name
    }
    return ""
}
```
