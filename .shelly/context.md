# Shelly Project Context

## Overview

**Shelly** is a provider-agnostic, multi-agent orchestration framework written in Go 1.25. It provides a unified foundation for building sophisticated LLM chat applications with support for multiple providers, tool execution, and intelligent agent coordination.

**Module:** `github.com/germanamz/shelly`  
**Entry Point:** `cmd/shelly/shelly.go`  
**Build System:** Taskfile.dev

## Core Architecture

Shelly follows a layered architecture with clean separation of concerns:

### Layer 1: Foundation
- **`pkg/chats/`** - Provider-agnostic chat data model (roles, content, messages)
- **`pkg/agentctx/`** - Zero-dependency context key helpers for agent identity
- **`pkg/shellydir/`** - `.shelly/` directory path resolution & bootstrapping

### Layer 2: Model Abstraction  
- **`pkg/modeladapter/`** - `Completer` interface, usage tracking, token estimation
- **`pkg/providers/`** - LLM provider implementations (Anthropic, OpenAI, Grok, Gemini)

### Layer 3: Tool Execution
- **`pkg/tools/`** - Toolbox abstraction, MCP client (stdio+HTTP), MCP server
- **`pkg/codingtoolbox/`** - Built-in tools (filesystem, exec, search, git, http, etc.)

### Layer 4: Intelligence
- **`pkg/skill/`** - Folder-based skill loading with YAML frontmatter
- **`pkg/agent/`** - ReAct loop, registry delegation, middleware, effects system

### Layer 5: Orchestration
- **`pkg/state/`** - Key-value state store with watch support (blackboard pattern)
- **`pkg/tasks/`** - Shared task board for multi-agent coordination
- **`pkg/projectctx/`** - Curated context loading, structural project indexing

### Layer 6: Composition
- **`pkg/engine/`** - Composition root, wires everything from YAML config

### Layer 7: Interface
- **`cmd/shelly/`** - CLI entry point with bubbletea TUI

## Key Features

### Multi-Provider Support
- **Anthropic** (Claude) - Primary provider with advanced tool use
- **OpenAI** (GPT models) - Wide compatibility, structured outputs
- **Grok** (xAI) - Alternative provider with X.com context
- **Gemini** (Google) - Multi-modal capabilities

### Tool Integration
- **MCP Protocol** - Model Context Protocol for stdio and HTTP tool servers  
- **Built-in Tools** - Filesystem, execution, search, git, HTTP, notes, permissions
- **External Tools** - Plugin architecture for custom tool integration

### Agent System  
- **Unified Agent Type** - Single agent handles all orchestration patterns
- **Dynamic Delegation** - Runtime discovery and spawning of specialized agents
- **ReAct Loop** - Reason-Act-Observe pattern for autonomous behavior
- **Registry System** - Factory-based agent creation and management

### Context Management
- **Project Context** - Automatic discovery and indexing of project structure
- **Knowledge Graph** - Structured representation in `.shelly/knowledge/`
- **State Management** - Shared blackboard for inter-agent communication
- **Skills System** - Markdown-based procedural knowledge with YAML frontmatter

### Developer Experience
- **TUI Interface** - Rich terminal interface using bubbletea
- **Configuration** - YAML-based setup with environment overrides
- **Testing** - Comprehensive test suite using testify
- **Documentation** - Extensive architectural documentation and package READMEs

## Project Structure

```
shelly/
├── cmd/shelly/           # CLI entry point, TUI interface  
├── pkg/                  # Core packages (see layer architecture above)
├── .shelly/              # Project-specific configuration
│   ├── config.yaml       # Engine configuration
│   ├── skills/           # Custom skills directory
│   ├── knowledge/        # Project knowledge graph (this indexing)
│   └── local/            # Runtime state (notes, permissions, etc.)
├── docs/                 # Additional documentation
├── ARCHITECTURE.md       # Detailed architectural reference
├── FEATURE_SPEC.md       # Comprehensive feature specification  
├── CLAUDE.md            # Quick developer reference
└── Taskfile.yml         # Build and development tasks
```

## Development Workflow

Key development commands via Taskfile:
- `task build` - Build to `./bin/shelly`
- `task run` - Run the application  
- `task check` - Run all checks (fmt, lint, test)
- `task test:coverage` - Tests with coverage report

Environment setup: Copy `.env.example` → `.env` for API keys.

## Knowledge Index

This context file serves as the entry point to the project knowledge graph. Detailed documentation for specific areas can be found in:

*Knowledge files will be added here as indexing progresses...*

---

*Last updated: Initial indexing - comprehensive project overview created*