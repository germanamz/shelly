# `sanitizeFilename` Duplicated Across Two Packages

## Severity: Moderate

## Location

- `pkg/agent/reflection.go:86-98`
- `pkg/agent/effects/offload.go:230-242`

## Description

Both functions implement the same replacement logic (non-alphanumeric/non-hyphen/non-underscore characters replaced with hyphens). `effects` imports `agent`, so it cannot call `agent.sanitizeFilename` because it is unexported.

## Fix

Either:
- Export `SanitizeFilename` from `agent` and call it from `effects/offload.go`
- Move it to a small shared internal helper reachable from both packages (e.g., `pkg/agentctx` or a new `internal/stringutil`)
