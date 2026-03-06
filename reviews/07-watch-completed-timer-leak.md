# `WatchCompleted` Leaks a Timer on Every Loop Iteration

## Severity: Important

## Location

- `pkg/tasks/store.go:393-406`

## Description

`time.After(remaining)` is called inside the `select` on every loop iteration. Each call allocates a new `time.Timer` and associated channel. When a task has an assignee, `remaining` is always the full 15-second `unclaimedTimeout`, so each pass leaks a timer that stays alive for 15 seconds before GC can collect it.

A long-running watch on a slow task generates many live timer goroutines.

## Fix

Allocate one `time.NewTimer` before the loop, `defer timer.Stop()`, and call `timer.Reset(remaining)` at the bottom of each iteration:

```go
timer := time.NewTimer(remaining)
defer timer.Stop()

for {
    // ... loop body ...

    if !timer.Stop() {
        select {
        case <-timer.C:
        default:
        }
    }
    timer.Reset(remaining)

    select {
    case <-ctx.Done(): ...
    case <-sig: ...
    case <-timer.C: ...
    }
}
```
