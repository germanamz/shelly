# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

Uses [Taskfile](https://taskfile.dev/) as task runner. Ensure `$(go env GOPATH)/bin` is in your `PATH`.

```bash
task build          # Build binary to ./bin/shelly
task run            # Run the application
```

## Code Quality

```bash
task fmt            # Format code with gofumpt
task fmt:check      # Check formatting (CI-friendly, no writes)
task lint           # Run golangci-lint v2
task lint:fix       # Run golangci-lint with auto-fix
task test           # Run tests with gotestsum
task test:coverage  # Run tests with coverage report
task check          # Run all checks (fmt:check + lint + test)
```

## Tooling

- **Formatter**: [gofumpt](https://github.com/mvdan/gofumpt) (strict superset of gofmt)
- **Linter**: [golangci-lint v2](https://golangci-lint.run/) (config: `.golangci.yml`)
- **Testing**: `go test` + [testify](https://github.com/stretchr/testify) assertions + [gotestsum](https://github.com/gotestyourself/gotestsum) output
- **Task runner**: [go-task](https://taskfile.dev/) (config: `Taskfile.yml`)

## Project Overview

Shelly is a Go project (module: `germanamz/shelly`, Go 1.25). Currently a single-package CLI application with `main.go` as the entry point.
