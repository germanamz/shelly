# `maxReadSize` Constant Duplicated in Two Handlers

## Severity: Moderate

## Location

- `pkg/codingtoolbox/filesystem/filesystem.go:221` — inside `handleRead`
- `pkg/codingtoolbox/filesystem/filesystem.go:268` — inside `handleReadLines`

## Description

`const maxReadSize = 10 << 20` is defined independently inside both handlers. A change to one that misses the other silently creates an inconsistency between two tools documented as sharing the same 10MB cap.

## Fix

Hoist to a package-level constant:

```go
const maxReadSize = 10 << 20 // 10 MB
```
