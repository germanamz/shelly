# stdout/stderr Capture Pattern and `maxBufferSize` Duplicated

## Severity: Moderate

## Location

- `pkg/codingtoolbox/exec/exec.go:97-122`
- `pkg/codingtoolbox/git/git.go:97-128`

## Description

Both packages independently implement the same pattern:

1. Allocate two `LimitedBuffer` instances (stdout + stderr)
2. Run a command
3. Assemble output by appending stderr after stdout with a newline separator

The `maxBufferSize` constant (`1 << 20`) is also duplicated in both files. The `git` package encapsulates this in `runGit`; `exec` inlines it.

Any behavioral change to the output assembly (separator format, truncation labeling) must be applied in two places.

## Fix

Extract a shared helper into the `codingtoolbox` root package or a sub-package that both `exec` and `git` can import.
