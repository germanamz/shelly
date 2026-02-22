# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

Uses [Taskfile](https://taskfile.dev/) as task runner. Ensure `$(go env GOPATH)/bin` is in your `PATH`.

```bash
task build          # Build binary to ./bin/shelly
task run            # Run the application
```

Copy `.env.example` to `.env` and fill in API keys (Anthropic, OpenAI, Grok). `.env` is gitignored.

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
- `examples/` — YAML config files for running Shelly (e.g., `simple-assistant.yaml`)
- `pkg/chats/` — provider-agnostic LLM chat data model (role, content, message, chat)
- `pkg/modeladapter/` — LLM adapter abstraction layer (`Completer` interface, `ModelAdapter` base struct, usage tracking)
- `pkg/providers/` — LLM provider implementations (anthropic, openai, grok)
- `pkg/tools/` — tool infrastructure: toolbox, MCP client, MCP server
- `pkg/codingtoolbox/` — built-in coding tools (ask, filesystem, exec, search, git, http, permissions, defaults)
- `pkg/skill/` — skill loading from markdown files (procedures agents follow)
- `pkg/agent/` — unified agent with ReAct loop, registry-based delegation, middleware, and orchestration tools
- `pkg/agentctx/` — shared context key helpers for propagating agent identity across packages
- `pkg/state/` — key-value state store for agents
- `pkg/tasks/` — shared task board for multi-agent coordination (create, claim, watch tasks)
- `pkg/engine/` — composition root that wires all components from YAML config, exposes frontend-agnostic Engine/Session/EventBus API

## Architecture

- `pkg/chats/` is the foundation layer with no dependencies on other `pkg/` packages
- `pkg/modeladapter/` depends on `pkg/chats/` and `pkg/tools/toolbox/` (chat, message types, ToolAware interface)
- `pkg/providers/` depends on `pkg/modeladapter/` and `pkg/chats/`
- `pkg/tools/toolbox/` depends on `pkg/chats/content` (ToolCall, ToolResult types)
- `pkg/tools/mcpclient/` and `pkg/tools/mcpserver/` depend on `pkg/tools/toolbox/` (Tool type)
- `pkg/codingtoolbox/` depends on `pkg/tools/toolbox/` (Tool and ToolBox types)
- `pkg/codingtoolbox/permissions/` is shared by `pkg/codingtoolbox/filesystem/`, `pkg/codingtoolbox/exec/`, `pkg/codingtoolbox/search/`, `pkg/codingtoolbox/git/`, and `pkg/codingtoolbox/http/`
- `pkg/skill/` has no dependencies on other `pkg/` packages
- `pkg/agentctx/` has no dependencies on other `pkg/` packages (zero-dependency by design)
- `pkg/agent/` depends on `pkg/agentctx/`, `pkg/modeladapter/`, `pkg/tools/toolbox/`, `pkg/chats/`, and `pkg/skill/`
- `pkg/tasks/` depends on `pkg/agentctx/` and `pkg/tools/toolbox/`
- `pkg/engine/` depends on all other `pkg/` packages — it is the top-level composition root
- `cmd/shelly/` is the entry point (currently a placeholder)

## Conventions

- Dependencies are managed by Go modules; do not delete `go.mod` and `go.sum`
- Linter extras enabled: gosec, gocritic, gocyclo (max 15), unconvert, misspell, modernize, testifylint
- Tests use testify `assert` by default; use `require` only when a failure must stop the test immediately
- Every top-level package under `pkg/` must include a `README.md` explaining its purpose, architecture, and use cases
