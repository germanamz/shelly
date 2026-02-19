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
task test:coverage:html  # Run tests with HTML coverage report
task check          # Run all checks (fmt:check + lint + test)
```

## Tooling

- **Formatter**: [gofumpt](https://github.com/mvdan/gofumpt) (strict superset of gofmt)
- **Linter**: [golangci-lint v2](https://golangci-lint.run/) (config: `.golangci.yml`)
- **Testing**: `go test` + [testify](https://github.com/stretchr/testify) assertions + [gotestsum](https://github.com/gotestyourself/gotestsum) output
- **Task runner**: [go-task](https://taskfile.dev/) (config: `Taskfile.yml`)

## Project Overview

Shelly is a Go project (module: `github.com/germanamz/shelly`, Go 1.25). CLI entry point: `cmd/shelly/shelly.go`. Tests live alongside source files (e.g., `cmd/shelly/shelly_test.go`).

## Project Structure

- `cmd/shelly/` — main package (entry point + tests)
- `pkg/chatty/` — provider-agnostic LLM chat data model (role, content, message, chat)
- `pkg/providers/` — LLM provider abstraction layer (model config, `Provider` interface)
- `pkg/tools/` — tool execution and MCP integration (toolbox, mcpclient, mcpserver)
- `pkg/reactor/` — reserved for future use (empty)

## Architecture

- `pkg/chatty/` is the foundation layer with no dependencies on other `pkg/` packages
- `pkg/providers/` depends on `pkg/chatty/` (chat, message types)
- `pkg/tools/toolbox/` depends on `pkg/chatty/content` (ToolCall, ToolResult types)
- `pkg/tools/mcpclient/` and `pkg/tools/mcpserver/` depend on `pkg/tools/toolbox/` (Tool type)
- `cmd/shelly/` is the entry point (currently a placeholder)

## Conventions

- Dependencies are managed by Go modules; do not delete `go.mod` and `go.sum`
- Linter extras enabled: gosec, gocritic, gocyclo (max 15), unconvert, misspell, modernize, testifylint
- Tests use testify `assert` by default; use `require` only when a failure must stop the test immediately
- Every package under `pkg/` must include a `README.md` explaining its purpose, architecture, and use cases
