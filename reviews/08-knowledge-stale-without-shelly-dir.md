# `knowledgeStale` Always True When `.shelly/` Doesn't Exist

## Severity: Important

## Location

- `pkg/engine/init.go:43-49`

## Description

`projectctx.IsKnowledgeStale` runs unconditionally regardless of `dir.Exists()`. When `.shelly/` is absent, `os.Stat(d.ContextPath())` fails, causing `IsKnowledgeStale` to return `true` (missing = stale).

This sets `e.knowledgeStale = true` permanently, so `KnowledgeStale()` returns `true` for every project without a `.shelly/` directory. Any frontend displaying a staleness warning will show it permanently for projects that don't use `.shelly/`.

## Fix

Guard the staleness check:

```go
if dir.Exists() {
    knowledgeStale = projectctx.IsKnowledgeStale(projectRoot, dir)
}
```
