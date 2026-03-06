# Shelly Project Context

## Overview

**Shelly** is a provider-agnostic, multi-agent orchestration framework written in Go 1.25. It provides a unified foundation for building sophisticated LLM chat applications with support for multiple providers, tool execution, and intelligent agent coordination.

**Module:** `github.com/germanamz/shelly`  
**Entry Point:** `cmd/shelly/shelly.go`  
**Build System:** Taskfile.dev

## Core Architecture

Shelly follows a layered architecture with clean separation of concerns:

### Layer 1: Foundation
- **`pkg/chats/`** - Provider-agnostic chat data model (roles, content parts, messages, thread-safe chat container)
- **`pkg/agentctx/`** - Zero-dependency context key helpers for agent identity + filename sanitizer
- **`pkg/shellydir/`** - `.shelly/` directory path resolution & bootstrapping

### Layer 2: Model Abstraction  
- **`pkg/modeladapter/`** - `Completer` interface (`Complete(ctx, chat, tools) → msg, err`), `UsageReporter`, `RateLimitInfoReporter`, `RateLimitedCompleter`, `AgentUsageCompleter`, shared `Client` (HTTP/WS), `TokenEstimator`, usage tracking, batch support
- **`pkg/providers/`** - LLM provider implementations (Anthropic, OpenAI, Grok, Gemini) + shared `internal/openaicompat`

### Layer 3: Tool Execution
- **`pkg/tools/`** - Toolbox abstraction, MCP client (stdio+HTTP), MCP server
- **`pkg/codingtoolbox/`** - Built-in tools (filesystem, exec, search, git, http, notes, permissions, defaults, ask) with permission gating

### Layer 4: Intelligence
- **`pkg/skill/`** - Folder-based skill loading with YAML frontmatter
- **`pkg/agent/`** - ReAct loop, registry delegation, middleware, 14+ effects, inbox routing, interactive/blocking delegation modes

### Layer 5: Orchestration
- **`pkg/state/`** - Key-value state store with watch support (blackboard pattern)
- **`pkg/tasks/`** - Shared task board for multi-agent coordination (with cancel watching)
- **`pkg/projectctx/`** - Curated context loading, structural project indexing, staleness detection
- **`pkg/sessions/`** - File-based session persistence with JSON serialization & attachments

### Layer 6: Composition
- **`pkg/engine/`** - Composition root, wires everything from YAML config, Engine/Session/EventBus API

### Support Packages
- **`pkg/mcproots/`** - Zero-dependency MCP Roots protocol utilities

### Layer 7: Interface
- **`cmd/shelly/`** - CLI entry point with bubbletea v2 TUI (interactive, batch, index modes)

## Key Features

### Multi-Provider Support
- **Anthropic** (Claude) - Primary provider with advanced tool use
- **OpenAI** (GPT models) - Wide compatibility, structured outputs
- **Grok** (xAI) - OpenAI-compatible API via shared `openaicompat` internal package
- **Gemini** (Google) - Multi-modal capabilities, custom batch support

### Rate Limiting & Usage
- **Proactive rate limiting** - TPM/RPM token-bucket throttling before API calls
- **Reactive retry** - Exponential backoff with jitter on HTTP 429 responses
- **Per-agent usage tracking** - `AgentUsageCompleter` isolates token counts per agent
- **Pricing** - Embedded YAML pricing data with override support

### Tool Integration
- **MCP Protocol** - Model Context Protocol for stdio and HTTP tool servers
- **Built-in Tools** - Filesystem, execution, search, git, HTTP, notes, permissions, defaults, ask
- **Permission Gating** - Approver system for tool execution authorization

### Agent System  
- **Unified Agent Type** - Single agent handles all orchestration patterns
- **Dynamic Delegation** - Registry-based spawning with blocking, auto, and interactive modes
- **ReAct Loop** - Reason-Act-Observe pattern with iteration limits
- **Effects System** - 14+ pluggable effects: compaction, sliding window, loop detection, stall detection, observation masking, offloading, trimming, progress prompts, reflection, token/time budgets, tool scoping
- **Inbox Routing** - `InboxRegistrar`/`InboxUnregistrar` for user message routing to named child agents
- **Handoffs** - Peer-to-peer agent handoff with configurable chain limits

### Context Management
- **Project Context** - Automatic discovery and indexing of project structure
- **Knowledge Graph** - Structured representation in `.shelly/knowledge/`
- **State Management** - Shared blackboard for inter-agent communication
- **Skills System** - Markdown-based procedural knowledge with YAML frontmatter

### Developer Experience
- **TUI Interface** - Rich terminal interface using bubbletea v2
- **Configuration** - YAML-based setup with environment overrides
- **Testing** - Comprehensive test suite using testify
- **Documentation** - Extensive package READMEs and architectural docs

## Project Structure

```
shelly/
├── cmd/shelly/              # CLI entry point, bubbletea v2 TUI
│   └── internal/            # TUI components (model, views, styles, bridge)
├── pkg/
│   ├── agent/               # ReAct loop, delegation, registry, middleware
│   │   └── effects/         # 14+ effects: compaction, sliding window, loop/stall detect, etc.
│   ├── agentctx/            # Context key helpers for agent identity
│   ├── chats/               # Provider-agnostic chat data model
│   │   ├── role/            # System, User, Assistant, Tool roles
│   │   ├── content/         # Text, Image, Document, ToolCall, ToolResult parts
│   │   ├── message/         # Message type with multi-modal parts
│   │   └── chat/            # Thread-safe conversation container
│   ├── codingtoolbox/       # Built-in tools (fs, exec, search, git, http, etc.)
│   ├── engine/              # Composition root — YAML config → full runtime
│   ├── mcproots/            # MCP Roots protocol utilities
│   ├── modeladapter/        # Completer interface, rate limiting, usage, batching
│   │   ├── usage/           # Token tracking and pricing
│   │   └── batch/           # Batch completion support
│   ├── projectctx/          # Project context loading, staleness detection
│   ├── providers/           # LLM adapters (anthropic, openai, grok, gemini)
│   │   └── internal/openaicompat/  # Shared OpenAI-compatible provider code
│   ├── sessions/            # File-based session persistence with attachments
│   ├── shellydir/           # .shelly/ directory resolution & bootstrapping
│   ├── skill/               # Folder-based skill loading with YAML frontmatter
│   ├── state/               # Shared KV store (blackboard pattern)
│   ├── tasks/               # Shared task board for multi-agent coordination
│   └── tools/               # Toolbox abstraction, MCP client/server
│       ├── toolbox/         # Tool/ToolBox types
│       ├── mcpclient/       # MCP client (stdio + HTTP)
│       ├── mcpserver/       # MCP server
│       └── schema/          # JSON schema utilities
├── .shelly/                 # Project-specific configuration
│   ├── config.yaml          # Engine configuration
│   ├── skills/              # Custom skills directory
│   ├── knowledge/           # Project knowledge graph (11 files)
│   └── local/               # Runtime state (notes, permissions, etc.)
├── ARCHITECTURE.md          # Detailed architectural reference
├── FEATURE_SPEC.md          # Comprehensive feature specification
├── CLAUDE.md                # Quick developer reference
└── Taskfile.yml             # Build and development tasks
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

### Foundation Layer (✅ Indexed)
- **[Foundation Overview](knowledge/foundation-layer.md)** — Foundational packages overview, relationships, and dependency rules
- **[Chat Data Model](knowledge/chats-foundation.md)** — `role`, `content`, `message`, `chat` sub-packages with full API reference
- **[Agent Context](knowledge/agentctx-identity.md)** — Zero-dependency context key helpers (`WithAgentName`, `AgentNameFromContext`, `SanitizeFilename`)
- **[Shelly Directory](knowledge/shellydir-paths.md)** — `Dir` value object for `.shelly/` path resolution and bootstrapping

### Model Abstraction (✅ Indexed)
- **[Model Adapter](knowledge/modeladapter-abstraction.md)** — `Completer`, `UsageReporter`, `RateLimitInfoReporter` interfaces; `Client` HTTP/WS transport; `RateLimitedCompleter`; `AgentUsageCompleter`; `TokenEstimator`; `usage/` and `batch/` sub-packages
- **[LLM Providers](knowledge/providers-layer.md)** — Anthropic, OpenAI, Grok, Gemini adapters; `internal/openaicompat` shared code; message translation, tool-call mapping

### Tool System (✅ Indexed)
- **[Tool System](knowledge/tool-system.md)** — `ToolBox` abstraction, `Tool` type, MCP client (stdio+HTTP) and server, JSON schema, built-in coding tools, permission gating via Approver

### Agent System (✅ Indexed)
- **[Agent System](knowledge/agent-system.md)** — ReAct loop, dynamic delegation (blocking/auto/interactive modes), registry, middleware, 14+ effects (compaction, sliding window, loop/stall detection, observation masking, offloading, trimming, progress, reflection, token/time budgets, tool scoping), inbox routing, interaction channels, handoffs

### Orchestration Layer (✅ Indexed)
- **[Orchestration](knowledge/orchestration-layer.md)** — State store (blackboard KV with watch), task board (multi-agent coordination with cancel watching), project context loading & staleness, MCP roots utilities, session persistence with attachments

### Composition Layer (✅ Indexed)
- **[Skills & Engine](knowledge/composition-layer.md)** — Skill loading with YAML frontmatter, engine composition root (YAML config → full runtime), rate limit wiring, batch sessions

### CLI & TUI Interface (✅ Indexed)
- **[CLI & TUI](knowledge/cli-tui-interface.md)** — Entry point, bubbletea v2 TUI architecture, state machine, event bridge, display rendering, input handling, batch mode, index mode

---

*Last updated: Incremental update — all 11 knowledge files refreshed to match current codebase*
