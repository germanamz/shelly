# CLAUDE.md

## Quick Reference

Go 1.25 module `github.com/germanamz/shelly`. Entry point: `cmd/shelly/shelly.go`. Uses [Taskfile](https://taskfile.dev/) — ensure `$(go env GOPATH)/bin` is in `PATH`.

| Command | Purpose |
|---------|---------|
| `task build` | Build to `./bin/shelly` |
| `task run` | Run the application |
| `task fmt` / `task fmt:check` | Format (gofumpt) / check only |
| `task lint` / `task lint:fix` | Lint (golangci-lint v2) / auto-fix |
| `task test` | Run tests (gotestsum + testify) |
| `task test:coverage` | Tests with coverage report |
| `task check` | All checks (fmt:check + lint + test) |

Copy `.env.example` → `.env` for API keys (Anthropic, OpenAI, Grok). `.env` is gitignored.

## Package Map

| Package | Role |
|---------|------|
| `cmd/shelly/` | CLI entry point, bubbletea TUI |
| `pkg/chats/` | Provider-agnostic chat data model (foundation, no pkg deps) |
| `pkg/modeladapter/` | `Completer` interface, usage tracking, `TokenEstimator` → depends on chats, tools/toolbox |
| `pkg/providers/` | LLM providers (anthropic, openai, grok, gemini) → depends on modeladapter, chats |
| `pkg/tools/` | Toolbox abstraction, MCP client (stdio+HTTP), MCP server |
| `pkg/codingtoolbox/` | Built-in tools (ask, filesystem, exec, search, git, http, notes, permissions, defaults) |
| `pkg/skill/` | Folder-based skill loading with YAML frontmatter |
| `pkg/agent/` | ReAct loop, registry delegation, middleware, effects system, `EventNotifier` |
| `pkg/agent/effects/` | Effect implementations (compaction, trimming) — agent never imports this |
| `pkg/agentctx/` | Context key helpers for agent identity (zero-dependency) |
| `pkg/shellydir/` | `.shelly/` dir path resolution & bootstrapping (zero-dependency) |
| `pkg/projectctx/` | Curated context loading, structural project index → depends on shellydir |
| `pkg/state/` | Key-value state store with watch support |
| `pkg/tasks/` | Shared task board for multi-agent coordination |
| `pkg/engine/` | Composition root — wires everything from YAML config, Engine/Session/EventBus API |

## Package Docs

Read the relevant README before modifying a package:

[agent](pkg/agent/README.md) | [agentctx](pkg/agentctx/README.md) | [chats](pkg/chats/README.md) | [codingtoolbox](pkg/codingtoolbox/README.md) | [engine](pkg/engine/README.md) | [modeladapter](pkg/modeladapter/README.md) | [projectctx](pkg/projectctx/README.md) | [providers](pkg/providers/README.md) | [shellydir](pkg/shellydir/README.md) | [skill](pkg/skill/README.md) | [state](pkg/state/README.md) | [tasks](pkg/tasks/README.md) | [tools](pkg/tools/README.md)

## Conventions

- **Read the package README** (`pkg/*/README.md`) before modifying any package
- Tests use testify `assert` by default; `require` only when failure must stop the test
- Linter extras: gosec, gocritic, gocyclo (max 15), unconvert, misspell, modernize, testifylint
- Do not delete `go.mod` / `go.sum`
- Every `pkg/` package must have a `README.md`
