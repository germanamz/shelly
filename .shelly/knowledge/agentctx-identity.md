# Agent Context Identity

## Overview

The `pkg/agentctx` package is a **tiny, zero-dependency** utility package with exactly two source files. It provides context-key helpers for propagating agent identity and a filename sanitizer. Its zero-dependency design lets any package in the system import it without creating cycles.

## Files

```
pkg/agentctx/
├── context.go       # WithAgentName / AgentNameFromContext
├── context_test.go  # Tests
├── sanitize.go      # SanitizeFilename
└── README.md
```

## API — Complete

### context.go

```go
func WithAgentName(ctx context.Context, name string) context.Context
```

Returns a new context carrying the given agent name. Uses an unexported `agentNameCtxKey` struct as the context key.

```go
func AgentNameFromContext(ctx context.Context) string
```

Extracts the agent name from context. Returns the empty string if no name was set.

### sanitize.go

```go
func SanitizeFilename(s string) string
```

Replaces any character that is **not** `[a-zA-Z0-9_-]` with a hyphen (`-`). Used to turn agent names into safe filesystem path components (e.g. for session files, logs).

## Usage Pattern

```go
// Setting identity (agent startup or delegation)
ctx = agentctx.WithAgentName(ctx, "code-reviewer")

// Reading identity (logging, task attribution, file paths)
name := agentctx.AgentNameFromContext(ctx)

// Safe filenames from agent names
filename := agentctx.SanitizeFilename(name)
```

## That's It

This package has **no types, no interfaces, no structs** (other than the unexported context key). It exists solely to break import cycles: both `pkg/agent` and `pkg/engine` need to read/write agent identity on contexts, and this shared package lets them do so without depending on each other.
