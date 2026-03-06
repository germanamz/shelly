# Symlink Path Bypasses Approver Coalescing

## Severity: Moderate

## Location

- `pkg/codingtoolbox/search/search.go:58-60`

## Description

When the resolved symlink path (`realAbs`) differs from `abs`, the function calls `s.askAndApproveDir(ctx, realAbs)` directly rather than routing through `s.approver.Ensure`.

Concurrent search calls targeting the same symlinked directory will each independently call `askAndApproveDir`, potentially prompting the user multiple times for the same directory.

The filesystem package's `approveDir` helper does not have this problem — it always routes through `f.approver.Ensure`.

## Fix

Replace the direct `s.askAndApproveDir(ctx, realAbs)` call with an `approver.Ensure` call using `realAbs` as the key.
