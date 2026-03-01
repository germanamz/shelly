---
description: "Explore the project and build/update the knowledge graph in .shelly/"
---

# Project Indexer

You are indexing this project to build a knowledge graph that helps all agents
understand the codebase. Follow these steps:

## 1. Check Existing State

Read `.shelly/context.md` first. If it already has real content (not just the
starter template), this is an **incremental update**:
- Compare the existing index against the current project structure
- Focus on areas that have changed (new directories, renamed packages, etc.)
- Update only the sections that are stale rather than rewriting everything

If `context.md` is empty or contains only the starter template, this is a
**full index** — proceed with a complete exploration.

## 2. Explore Project Structure

Use filesystem and search tools to understand the project:
- Read the project root for key files (README, config files, entry points)
- List top-level directories to understand layout
- Identify the language/framework (go.mod, package.json, Cargo.toml, etc.)
- Find entry points and main packages

## 3. Write the Entry Point Index

Write `.shelly/context.md` — this is auto-loaded into every agent's prompt.
Keep it concise (under 200 lines) since it consumes context window space.

Include:
- **Project overview**: What the project does (1-2 sentences)
- **Tech stack**: Language, framework, key dependencies
- **Directory layout**: Top-level structure with one-line descriptions
- **Package/module map**: Key modules with their roles
- **Entry points**: Main executables or endpoints
- **Key conventions**: Naming, testing, build commands
- **Knowledge references**: Links to deeper docs in `.shelly/knowledge/`

Example format:
```
# Project Context

## Overview
<project-name> is a <brief description>.

## Tech Stack
- Language: Go 1.25
- Key deps: bubbletea, yaml.v3, testify

## Structure
- `cmd/` — CLI entry points
- `pkg/` — Library packages
- `internal/` — Private implementation

## Package Map
| Package | Role |
|---------|------|
| pkg/engine | Composition root, wires config |
| pkg/agent | ReAct loop, delegation |

## Entry Points
- `cmd/myapp/main.go` — Main CLI

## Conventions
- Tests: `go test` with testify assertions
- Format: gofumpt
- Build: `task build`

## Deep Docs
- [Architecture](.shelly/knowledge/architecture.md)
- [API Contracts](.shelly/knowledge/api-contracts.md)
```

## 4. Write Deep Knowledge Files

Write detailed topic files to `.shelly/knowledge/`. These are NOT loaded
into prompts — agents read them on-demand when a task requires it.

Suggested topics (create only what's relevant):
- `architecture.md` — Component relationships, data flow, design patterns
- `api-contracts.md` — API surface, request/response formats
- `dependencies.md` — Key dependency graph, version constraints
- `testing.md` — Test patterns, fixtures, how to run tests

## 5. Verify

After writing, read back `.shelly/context.md` to confirm it's well-formed
and provides a useful project overview.
