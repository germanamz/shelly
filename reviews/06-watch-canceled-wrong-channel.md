# `WatchCanceled` Waits on Wrong Signal Channel

## Severity: Important

## Location

- `pkg/tasks/store.go:334-337`

## Description

`WatchCanceled` blocks on `s.signal`, which is only closed by `notify()`. `notify()` is called from `Update`, `Claim`, `Reassign`, and `Cancel` — but not from `Create`.

If a new task is created while `WatchCanceled` is blocked, the goroutine will not wake up. The correct channel for "any mutation happened" is `s.changeCh` (returned by `Changes()`), which `notifyChange()` closes on every mutation including `Create`.

## Fix

Replace `sig := s.signal` with `sig := s.changeCh` in the `WatchCanceled` loop.
