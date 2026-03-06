# `os.IsNotExist` Instead of `errors.Is`

## Severity: Minor

## Location

- `cmd/shelly/index.go:74`

## Description

Uses `os.IsNotExist(err)` which is the deprecated form. The rest of the codebase consistently uses `errors.Is(err, os.ErrNotExist)` (see `helpers.go:15`, `shellydir/migrate.go:19`, `config.go:33`).

`os.IsNotExist` does not unwrap errors, so a wrapped `ErrNotExist` will not be recognized. This also violates the `modernize` linter rule listed in CLAUDE.md.

## Fix

Replace `os.IsNotExist(err)` with `errors.Is(err, os.ErrNotExist)` and add `"errors"` to the import block.
