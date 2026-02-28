# ARCHITECTURE.md - Shelly Architecture Reference

## Table of Contents

1. [Overview](#1-overview)
2. [Package Map](#2-package-map)
3. [Dependency Graph](#3-dependency-graph)
4. [Layer Architecture](#4-layer-architecture)
5. [Layer 1 - Chat Data Model](#5-layer-1---chat-data-model)
6. [Layer 2 - Model Adapter](#6-layer-2---model-adapter)
7. [Layer 3 - LLM Providers](#7-layer-3---llm-providers)
8. [Layer 4 - Tools](#8-layer-4---tools)
9. [Layer 5 - Skills](#9-layer-5---skills)
10. [Layer 6 - Agent](#10-layer-6---agent)
11. [Layer 7 - Cross-Cutting Concerns](#11-layer-7---cross-cutting-concerns)
12. [Data Flow](#12-data-flow)
13. [Concurrency Model](#13-concurrency-model)
14. [Usage Examples](#14-usage-examples)
15. [Interface Reference](#15-interface-reference)
16. [External Dependencies](#16-external-dependencies)

---

## 1. Overview

Shelly is a provider-agnostic, multi-agent orchestration framework written in Go. It provides:

- A clean chat data model decoupled from any LLM provider
- Pluggable LLM adapters (Anthropic, OpenAI, Grok, or custom)
- Tool execution with MCP (Model Context Protocol) integration
- A **unified Agent type** that runs a ReAct loop and self-orchestrates via dynamic delegation
- Markdown-based **Skills** that teach agents step-by-step procedures
- A **Registry** for runtime agent discovery and factory-based spawning
- Composable middleware for guardrails, logging, and lifecycle hooks
- A shared state store (blackboard pattern) for structured inter-agent data

The framework is designed around a **single Agent type** that handles all orchestration patterns. Rather than requiring users to compose multiple types (ReAct, Delegate, Handoff, Reactor, Coordinator), the unified Agent discovers and delegates to other agents dynamically through built-in orchestration tools.

```
Module:     github.com/germanamz/shelly
Go version: 1.25
Entry:      cmd/shelly/shelly.go
```

---

## 2. Package Map

```
shelly/
├── cmd/shelly/                          CLI entry point
│
├── pkg/chats/                           Layer 1: Chat data model
│   ├── role/                              Message roles (system, user, assistant, tool)
│   ├── content/                           Content parts (text, image, tool_call, tool_result)
│   ├── message/                           Message value type (sender + role + parts + metadata)
│   └── chat/                              Thread-safe conversation container
│
├── pkg/modeladapter/                    Layer 2: LLM abstraction
│   └── usage/                             Token usage tracking
│
├── pkg/providers/                       Layer 3: Concrete LLM providers
│   ├── anthropic/                         Anthropic Claude (Messages API)
│   ├── grok/                              Grok/xAI (OpenAI-compatible)
│   └── openai/                            OpenAI GPT (Chat Completions API)
│
├── pkg/tools/                           Layer 4: Tool execution
│   ├── toolbox/                           Tool definition, registration, and execution
│   ├── mcpclient/                         MCP client (connects to external MCP servers)
│   └── mcpserver/                         MCP server (exposes tools via MCP protocol)
│
├── pkg/skill/                           Layer 5: Skill loading
│                                          Markdown-based procedures for agents
│
├── pkg/shellydir/                       .shelly/ directory path resolution
│                                          Bootstrapping, permissions migration
│
├── pkg/projectctx/                      Project context loading
│                                          Curated *.md files + structural index
│
├── pkg/agent/                           Layer 6: Unified agent
│                                          ReAct loop, registry, delegation, middleware
│
├── pkg/agentctx/                        Agent identity propagation
│                                          Zero-dependency context key helpers
│
├── pkg/tasks/                           Shared task board
│                                          Multi-agent coordination (create, claim, watch)
│
├── pkg/state/                           Layer 7: Shared state
│                                          Thread-safe KV store (blackboard pattern)
│
└── pkg/engine/                          Composition root
                                           Wires config, .shelly/ dir, sessions, events
```

---

## 3. Dependency Graph

Arrows point from dependent to dependency. The graph enforces a strict layered architecture with no circular dependencies.

```
                         ┌──────────────────────────────────────┐
                         │            cmd/shelly/                │
                         └────────────────┬─────────────────────┘
                                          │ imports engine
                         ┌────────────────▼─────────────────────┐
                         │             engine/                    │
                         │  (composition root, wires everything) │
                         └──┬─────┬─────┬─────┬─────┬─────┬────┘
                            │     │     │     │     │     │
              ┌─────────────┘     │     │     │     │     └──────────────┐
              │                   │     │     │     │                    │
    ┌─────────▼─────────┐        │     │     │   ┌─▼────────────┐ ┌────▼──────────┐
    │      agent/        │        │     │     │   │ shellydir/   │ │ projectctx/   │
    │  (unified agent,   │        │     │     │   │ (.shelly/    │ │ (curated +    │
    │   registry,        │        │     │     │   │  paths,      │ │  generated    │
    │   middleware,       │        │     │     │   │  bootstrap)  │ │  context)     │
    │   orchestration)   │        │     │     │   └──────────────┘ └──────┬────────┘
    └─────────┬──────────┘        │     │     │                          │ uses
              │                   │     │     │                   ┌──────▼────────┐
     ┌────────┼───────────────┐   │     │     │                   │  shellydir/   │
     │        │               │   │     │     │                   └───────────────┘
     │        │               │   │     │     │
┌────▼────┐ ┌─▼──────────┐ ┌─▼──────┐ │   ┌──▼────────┐   ┌──────────┐
│ toolbox/ │ │modeladapter/│ │ skill/ │ │   │  chats/    │   │  state/  │
│          │ │  └── usage/ │ │        │ │   │ ├── role/  │   │  tasks/  │
│          │ │             │ │        │ │   │ ├── content│   └──────────┘
│          │ └─────┬───────┘ └────────┘ │   │ ├── message│
│          │       │                    │   │ └── chat/   │
└────┬─────┘       │                    │   └────────┬────┘
     │             │                    │            │
     └─────────────┴────────────────────┴────────────┘
                   │
           ┌───────▼────────┐
           │  chats/content  │  (ToolCall, ToolResult shared types)
           └────────────────┘

    ┌──────────────┐   ┌──────────────┐
    │  mcpclient/  │   │  mcpserver/  │
    │ (MCP client) │   │ (MCP server) │
    └──────┬───────┘   └──────┬───────┘
           │                  │
           └──────┬───────────┘
                  │ uses
           ┌──────▼───────┐
           │   toolbox/   │
           └──────────────┘

    ┌────────────────┐   ┌────────────────┐   ┌────────────────┐
    │ providers/      │   │ providers/      │   │ providers/      │
    │ anthropic/      │   │ openai/         │   │ grok/           │
    └───────┬────────┘   └───────┬────────┘   └───────┬────────┘
            │                    │                    │
            └────────────────────┼────────────────────┘
                                 │ embeds
                          ┌──────▼───────┐
                          │ modeladapter/ │
                          └──────────────┘
```

**Rule:** A package may only import from the same layer or a lower layer. Never upward.

---

## 4. Layer Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Engine                     engine/                                      │
│  Composition root: wires config, .shelly/ dir, sessions, events          │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 7: Cross-cutting     state/  tasks/  agentctx/                    │
│  Shared state, task board, agent identity propagation                    │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 6: Agent             agent/                                       │
│  Unified agent: ReAct loop, registry, delegation, skills, middleware     │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 5: Skills + Context  skill/  shellydir/  projectctx/              │
│  Skill loading, .shelly/ path resolution, project context generation     │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 4: Tools             toolbox/  mcpclient/  mcpserver/             │
│  Tool definition, execution, MCP protocol integration                    │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 3: Providers         anthropic/  openai/  grok/                   │
│  Concrete LLM implementations                                           │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 2: Model Adapter     modeladapter/  usage/                        │
│  Completer interface, HTTP/WS helpers, auth, token tracking              │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 1: Chat Data Model   role/  content/  message/  chat/             │
│  Provider-agnostic types, thread-safe containers                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 5. Layer 1 - Chat Data Model

**Package:** `pkg/chats/`
**Dependencies:** None (foundation layer)

The chat data model defines how conversations are represented independently of any LLM provider.

### 5.1 Roles (`pkg/chats/role/`)

```go
type Role string

const (
    System    Role = "system"      // System instructions
    User      Role = "user"        // Human or external input
    Assistant Role = "assistant"   // LLM response
    Tool      Role = "tool"        // Tool execution result
)
```

### 5.2 Content Parts (`pkg/chats/content/`)

Content is multi-modal. The `Part` interface allows extension with custom types.

```
       ┌──────────┐
       │   Part    │  interface { PartKind() string }
       └────┬─────┘
            │
   ┌────────┼──────────┬────────────┐
   │        │          │            │
┌──▼──┐ ┌──▼───┐ ┌────▼─────┐ ┌───▼────────┐
│Text │ │Image │ │ToolCall  │ │ToolResult  │
│     │ │      │ │          │ │            │
│.Text│ │.URL  │ │.ID       │ │.ToolCallID │
│     │ │.Data │ │.Name     │ │.Content    │
│     │ │.Media│ │.Arguments│ │.IsError    │
└─────┘ └──────┘ └──────────┘ └────────────┘
```

`ToolCall` and `ToolResult` are the bridge between agents and tools. An assistant message containing `ToolCall` parts triggers tool execution. The results come back as `ToolResult` parts in a `Tool`-role message.

### 5.3 Messages (`pkg/chats/message/`)

```go
type Message struct {
    Sender   string          // Who created this message (agent name, "user", etc.)
    Role     role.Role       // Conversation role
    Parts    []content.Part  // Multi-modal content
    Metadata map[string]any  // Extensible key-value metadata
}
```

Key methods:
- `NewText(sender, role, text)` - Create a text message
- `TextContent() string` - Concatenate all Text parts
- `ToolCalls() []ToolCall` - Extract all tool call parts
- `SetMeta(key, value)` / `GetMeta(key)` - Metadata access

The `Sender` field is critical for multi-agent scenarios. When delegated agents produce output, `Sender` identifies who said what.

### 5.4 Chat (`pkg/chats/chat/`)

Thread-safe, mutable conversation container with signaling for async coordination.

```go
type Chat struct {
    mu       sync.RWMutex
    once     sync.Once
    signal   chan struct{}      // closed on Append, recreated immediately
    messages []message.Message
}
```

Key methods:

| Method | Description |
|--------|-------------|
| `New(...msgs)` | Create pre-populated chat |
| `Append(...msgs)` | Add messages, notify waiters |
| `Len()` | Message count |
| `At(i)` | Message by index |
| `Last()` | Most recent message |
| `Messages()` | Defensive copy of all messages |
| `Since(offset)` | Copy from offset onward |
| `BySender(name)` | Filter by sender |
| `SystemPrompt()` | Text of first system message |
| `Wait(ctx, n)` | Block until > n messages or ctx done |

**Signaling pattern:** `Append` closes the current signal channel and creates a new one. `Wait` selects on the signal channel and context cancellation. This is the same pattern used in `state.Store.Watch`.

```
  Goroutine A (writer)              Goroutine B (waiter)
  ─────────────────────             ─────────────────────
  chat.Append(msg)                  chat.Wait(ctx, n)
    │                                 │
    ├─ lock                           ├─ rlock, check len
    ├─ append message                 ├─ get signal chan
    ├─ close(signal)  ──────────────► ├─ select { <-signal }
    ├─ signal = make(chan)            ├─ wake up, recheck len
    └─ unlock                         └─ return new count
```

---

## 6. Layer 2 - Model Adapter

**Package:** `pkg/modeladapter/`
**Dependencies:** `chats/`

### 6.1 Completer Interface

The single most important interface in the system. Everything that can produce an LLM response implements this.

```go
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}
```

The contract:
- Read the conversation from `*chat.Chat`
- Call an LLM API
- Return the assistant's reply as a `message.Message`
- Do NOT append to the chat (the caller handles that)

### 6.2 ModelAdapter Base Struct

Provides HTTP helpers, auth, and usage tracking for concrete providers.

```go
type ModelAdapter struct {
    Name        string            // Model identifier (e.g. "claude-sonnet-4-20250514")
    Temperature float64
    MaxTokens   int
    Auth        Auth              // API key, header, scheme
    BaseURL     string            // API base URL
    Client      *http.Client      // nil -> http.DefaultClient
    Headers     map[string]string // Extra headers per request
    Usage       usage.Tracker     // Thread-safe token accounting
}
```

HTTP methods: `NewRequest`, `Do`, `PostJSON`, `DialWS`

### 6.3 Usage Tracker (`pkg/modeladapter/usage/`)

Thread-safe token accounting across multiple completions.

```go
type TokenCount struct {
    InputTokens  int
    OutputTokens int
}

type Tracker struct {
    mu      sync.Mutex
    entries []TokenCount
}
```

Methods: `Add(tc)`, `Last()`, `Total()`, `Count()`, `Reset()`

---

## 7. Layer 3 - LLM Providers

**Packages:** `pkg/providers/anthropic/`, `pkg/providers/openai/`, `pkg/providers/grok/`
**Dependencies:** `modeladapter/`, `chats/`, `tools/toolbox`

Each provider embeds `ModelAdapter` and implements `Completer`. Providers handle the translation between Shelly's generic data model and provider-specific API formats.

### 7.1 Provider Comparison

```
                  Anthropic              OpenAI                 Grok
                  ─────────              ──────                 ────
API Endpoint      /v1/messages           /v1/chat/completions   /chat/completions
Auth Header       x-api-key              Authorization: Bearer  Authorization: Bearer
System Prompt     Top-level "system"     "system" role message  "system" role message
                  parameter              in array               in array
Tool Results      Inside "user" message  Separate "tool" role   Separate "tool" role
                  (grouped)              messages               messages
Stop Signal       stop_reason            finish_reason          finish_reason
Tool Signal       "tool_use"             "tool_calls"           "tool_calls"
Tools Field       Adapter.Tools          Adapter.Tools          Via MarshalToolDef()
```

### 7.2 Message Translation Flow

```
  Shelly Message                  Provider API Request           Provider API Response
  ──────────────                  ────────────────────           ────────────────────
  ┌─────────────┐   buildRequest  ┌──────────────────┐         ┌──────────────────┐
  │ Chat        │───────────────►│ JSON body         │         │ JSON response    │
  │ .Messages() │                │ model, messages,  │         │ content blocks   │
  │ .System()   │                │ tools, max_tokens │         │ usage, stop      │
  └─────────────┘                └────────┬─────────┘         └────────┬─────────┘
                                          │ PostJSON                   │ parseResponse
                                          ▼                            ▼
                                   ┌──────────────┐            ┌──────────────┐
                                   │ HTTP Request  │            │ Message      │
                                   │ to provider   │───────────►│ .Parts       │
                                   └──────────────┘            │ .Role        │
                                                               └──────────────┘
```

### 7.3 Anthropic-Specific Behavior

Anthropic's API requires special handling:

1. **System prompt extraction:** The first system message is extracted from the chat and sent as a top-level `system` parameter, not in the messages array.

2. **Tool result grouping:** Tool results must be inside a `"user"` role message. When multiple tool results follow an assistant message, they are grouped into a single user message with multiple content blocks.

3. **Consecutive role merging:** If two messages in a row have the same role, they are merged into one API message with multiple content blocks.

### 7.4 OpenAI-Specific Behavior

1. **System prompt as message:** System messages stay in the messages array as `"system"` role entries.

2. **Nullable content:** Assistant messages with tool calls may have `null` content. The Content field is `*string` to handle this.

3. **Tool results as separate messages:** Each `ToolResult` becomes its own `"tool"` role message with a `tool_call_id`.

---

## 8. Layer 4 - Tools

**Packages:** `pkg/tools/toolbox/`, `pkg/tools/mcpclient/`, `pkg/tools/mcpserver/`
**Dependencies:** `chats/content`

### 8.1 Tool Definition

```go
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

type Tool struct {
    Name        string          // Unique identifier
    Description string          // Human-readable description (sent to LLM)
    InputSchema json.RawMessage // JSON Schema for the input
    Handler     Handler         // Execution function
}
```

### 8.2 ToolBox

Registration and dispatch center for tools. Agents can have multiple ToolBoxes, searched in order.

```go
type ToolBox struct {
    tools map[string]Tool
}
```

```
  LLM Response          ToolBox                Tool Handler
  ────────────          ───────                ────────────
  ┌──────────┐   Call   ┌──────────────┐   Handler   ┌──────────────┐
  │ ToolCall │────────►│ Lookup by    │──────────────►│ Execute      │
  │ .Name    │         │ name         │              │ function     │
  │ .ID      │         │              │              │              │
  │ .Args    │         └──────────────┘              └──────┬───────┘
  └──────────┘                                              │
                                                            ▼
                                                   ┌──────────────┐
                                                   │ ToolResult   │
                                                   │ .ToolCallID  │
                                                   │ .Content     │
                                                   │ .IsError     │
                                                   └──────────────┘
```

### 8.3 MCP Integration

The MCP (Model Context Protocol) packages bridge Shelly's tool system with the standard MCP protocol, enabling interoperability with external tool servers and clients.

```
  ┌──────────────┐                              ┌──────────────┐
  │  MCP Client  │◄── stdio/transport ──────────│ External MCP │
  │  (mcpclient) │                              │   Server     │
  └──────┬───────┘                              └──────────────┘
         │ ListTools()
         │ -> converts to []toolbox.Tool
         │ -> each handler calls CallTool() remotely
         ▼
  ┌──────────────┐
  │   ToolBox    │ (local registry, same interface)
  └──────────────┘

  ┌──────────────┐                              ┌──────────────┐
  │  MCP Server  │── stdio/transport ──────────►│ External MCP │
  │  (mcpserver) │                              │   Client     │
  └──────┬───────┘                              └──────────────┘
         │ Register()
         │ -> converts toolbox.Tool to MCP Tool
         │ -> wraps Handler as MCP ToolHandler
         ▼
  ┌──────────────┐
  │ Serve(in,out)│ (reads requests, executes handlers, writes responses)
  └──────────────┘
```

---

## 9. Layer 5 - Skills

**Package:** `pkg/skill/`
**Dependencies:** None (uses only standard library)

Skills are folder-based definitions that teach agents step-by-step procedures. Each skill lives in its own directory with a mandatory `SKILL.md` entry point and optional supplementary files. They are NOT identity -- they are knowledge that agents apply.

### 9.1 Skill Type

```go
type Skill struct {
    Name        string // Derived from folder name (e.g., "code-review")
    Description string // From YAML frontmatter; empty if no frontmatter
    Content     string // SKILL.md body after frontmatter
    Dir         string // Absolute path to the skill folder
}
```

### 9.2 Loading

```go
func Load(path string) (Skill, error)       // Load skill from a folder containing SKILL.md
func LoadDir(dir string) ([]Skill, error)   // Load skills from subdirectories with SKILL.md
```

`Load` reads `SKILL.md` from the given folder, derives the name from `filepath.Base(path)`, and resolves `Dir` to an absolute path. `LoadDir` iterates subdirectories, silently skipping any without a `SKILL.md` file.

### 9.3 How Skills Are Used

Skills are injected into the agent's system prompt as `### {name}` subsections under a `## Skills` heading. The LLM sees them as part of its instructions and follows the procedures described. The `load_skill` tool returns the skill content plus a footer with the skill directory path, allowing agents to access supplementary files via filesystem tools.

Example skill folder (`skills/orchestration/SKILL.md`):
```markdown
---
description: Orchestration procedures for complex tasks
---
When you receive a complex task:
1. Break it into subtasks
2. Check available agents with list_agents
3. Delegate subtasks to appropriate agents using delegate
4. Synthesize the results into a final answer
```

Supplementary files (checklists, scripts, templates) can be placed alongside `SKILL.md` in the skill folder.

---

## 10. Layer 6 - Agent

**Package:** `pkg/agent/`
**Dependencies:** `chats/`, `modeladapter/`, `tools/toolbox/`, `skill/`

The agent package provides a **single unified Agent type** that replaces all previous orchestration patterns. It runs a ReAct loop, can dynamically discover and delegate to other agents via a Registry, learns procedures from Skills, and applies middleware.

### 10.1 Agent

```go
type Agent struct {
    name         string
    description  string                    // Short description (shown in registry)
    instructions string                    // Detailed system instructions
    completer    modeladapter.Completer
    chat         *chat.Chat
    toolboxes    []*toolbox.ToolBox
    registry     *Registry                 // nil = no delegation capability
    options      Options
    depth        int                       // delegation depth (set internally)
}

type Options struct {
    MaxIterations      int           // ReAct loop limit (0 = unlimited)
    MaxDelegationDepth int           // Max tree depth for delegation (0 = cannot delegate)
    Skills             []skill.Skill // Procedures the agent knows
    Middleware         []Middleware   // Applied around Run()
    Context            string        // Project context injected into the system prompt
}
```

**Constructor:** `New(name, description, instructions, completer, opts) *Agent`

**Methods:**
- `Run(ctx) (message.Message, error)` - ReAct loop with middleware
- `Name() string`, `Description() string`, `Chat() *chat.Chat`
- `SetRegistry(r *Registry)` - Enable dynamic delegation
- `AddToolBoxes(tbs ...*toolbox.ToolBox)` - Add user tools

### 10.2 Agent.Run() Algorithm

```
Run(ctx):
  1. Apply middleware chain (wrap internal run function)
     - First middleware in Options.Middleware is outermost
  2. Call internal run(ctx):
     a. If chat is empty, build and prepend system prompt:
        - Identity: "You are {name}. {description}"
        - Instructions block (## Instructions)
        - Project context block (## Project Context)
        - Skills block (## Skills, each as ### subsection)
        - Agent directory from registry (## Available Agents, names + descriptions)
     b. Collect all toolboxes:
        - User-provided toolboxes
        - Orchestration toolbox (if registry is set): list_agents, delegate
     c. ReAct loop:
        for i := 0; maxIterations == 0 || i < maxIterations; i++ {
          - completer.Complete(ctx, chat) -> reply
          - reply.Sender = name; chat.Append(reply)
          - if no tool calls -> return reply (final answer)
          - for each tool call: search toolboxes, execute, append ToolResult to chat
        }
     d. return ErrMaxIterations
```

```
  ReAct Loop
  ──────────
                    ┌──────────────────────────────────────────┐
                    │                                          │
                    ▼                                          │
            ┌──────────────┐                                   │
            │   Complete   │  Call LLM with current Chat       │
            │   (Reason)   │                                   │
            └──────┬───────┘                                   │
                   │                                           │
                   ▼                                           │
            ┌──────────────┐  Yes                              │
            │ Tool calls?  ├──────────────────────┐            │
            └──────┬───────┘                      │            │
                   │ No                           ▼            │
                   │                       ┌──────────────┐    │
                   ▼                       │  Execute     │    │
            ┌──────────────┐               │  tools       │    │
            │ Return final │               │  (Act)       │    │
            │ answer       │               └──────┬───────┘    │
            └──────────────┘                      │            │
                                                  │ Results    │
                                                  │ appended   │
                                                  │ to Chat    │
                                                  └────────────┘
```

### 10.3 Registry

Thread-safe directory of agent factories. Each delegation spawns a **fresh agent instance** with a clean chat to avoid state leakage.

```go
type Factory func() *Agent

type Entry struct {
    Name        string
    Description string
}

type Registry struct {
    mu        sync.RWMutex
    factories map[string]Factory
    entries   map[string]Entry
}
```

**Methods:**
- `NewRegistry() *Registry`
- `Register(name, description, factory)` - Add agent factory
- `Get(name) (Factory, bool)` - Look up factory
- `List() []Entry` - All entries sorted by name
- `Spawn(name, depth) (*Agent, bool)` - Create fresh instance with delegation depth set

### 10.4 Built-in Orchestration Tools

Two tools are auto-injected when a Registry is set on the agent:

| Tool | Input | Behavior |
|------|-------|----------|
| `list_agents` | `{}` | Returns JSON array of `{name, description}` for all agents except self |
| `delegate` | `{tasks: [{agent, task, context}, ...]}` | Spawns agents concurrently (one per task), waits for all, returns JSON results array |

**Safety guards:**
- Self-delegation rejected (prevents A calling A)
- `MaxDelegationDepth` enforced (depth increments per delegation level, prevents infinite A->B->A chains)
- Each spawned agent receives the parent's registry (enabling nested delegation)

```
  Delegation Flow (single task)
  ─────────────────────────────
  ┌─────────────────────┐      delegate               ┌──────────────────────┐
  │  Orchestrator Agent  │      {tasks:[{agent:"worker",│   Worker Agent        │
  │  (ReAct loop)        │       task:"do X",          │   (fresh instance)    │
  │                      │       context:"..."}]}      │                       │
  │                      │─────────────────────────►│                       │
  │  Has registry with   │                           │  Clean chat:          │
  │  "worker" registered │                           │  ├─ system: prompt    │
  │                      │                           │  ├─ user: context     │
  │                      │                           │  ├─ user: "do X"      │
  │                      │◄─────────────────────────│  ├─ assistant: ...     │
  │                      │      Tool result          │  ├─ tool: ...         │
  │                      │      (JSON results array) │  └─ assistant: answer │
  └─────────────────────┘                           └──────────────────────┘

  Key: Each delegation spawns a fresh agent via the Factory.
       Results are returned as a JSON array of delegateResult objects.
       All intermediate reasoning stays in the child's private chat.
       The child agent is discarded after returning.
```

```
  Concurrent Delegation Flow (multiple tasks)
  ────────────────────────────────────────────
  ┌─────────────────────┐    delegate
  │  Orchestrator Agent  │    {tasks: [
  │                      │      {agent:"a", task:"...", context:"..."},
  │                      │      {agent:"b", task:"...", context:"..."}
  │                      │    ]}
  │                      │─────────────┬─────────────────┐
  │                      │             │                  │
  │                      │    ┌────────▼───────┐ ┌───────▼────────┐
  │                      │    │  Agent A       │ │  Agent B       │
  │                      │    │  (goroutine)   │ │  (goroutine)   │
  │                      │    └────────┬───────┘ └───────┬────────┘
  │                      │             │                  │
  │                      │◄────────────┴──────────────────┘
  │                      │    JSON: [{agent:"a", result:"..."},
  │                      │           {agent:"b", result:"..."}]
  └─────────────────────┘
```

### 10.5 Middleware

Composable wrappers around the agent's `Run` method using the `Runner` interface. Middleware is applied internally by `Agent.Run()` from `Options.Middleware`.

```go
type Runner interface {
    Run(ctx context.Context) (message.Message, error)
}

type RunnerFunc func(ctx context.Context) (message.Message, error)

type Middleware func(next Runner) Runner
```

Built-in middleware:

| Middleware | What It Does |
|-----------|-------------|
| `Timeout(d)` | Wraps context with `context.WithTimeout` |
| `Recovery()` | Catches panics, converts to `fmt.Errorf("agent panicked: %v", r)` |
| `Logger(log, name)` | Logs agent name, start/stop, duration, errors via `slog.Logger` |
| `OutputGuardrail(fn)` | Validates final message; returns error if `fn(msg)` fails |

```
  Middleware Stack (outermost -> innermost)
  ─────────────────────────────────────────

  ┌────────────────────────────────────────────────────────────┐
  │  Logger                                                    │
  │  ┌──────────────────────────────────────────────────────┐  │
  │  │  Timeout(30s)                                        │  │
  │  │  ┌────────────────────────────────────────────────┐  │  │
  │  │  │  Recovery                                      │  │  │
  │  │  │  ┌──────────────────────────────────────────┐  │  │  │
  │  │  │  │  OutputGuardrail(check)                  │  │  │  │
  │  │  │  │  ┌────────────────────────────────────┐  │  │  │  │
  │  │  │  │  │  Agent.run() (ReAct loop)          │  │  │  │  │
  │  │  │  │  └────────────────────────────────────┘  │  │  │  │
  │  │  │  └──────────────────────────────────────────┘  │  │  │
  │  │  └────────────────────────────────────────────────┘  │  │
  │  └──────────────────────────────────────────────────────┘  │
  └────────────────────────────────────────────────────────────┘

  Call order: Logger -> Timeout -> Recovery -> Guardrail -> run
  Return order: run returns -> Guardrail checks -> Recovery catches -> Timeout checks -> Logger logs
```

### 10.6 System Prompt Structure

When the agent starts with an empty chat, it auto-generates a system prompt with this structure:

```
You are {name}. {description}

## Instructions

{instructions}

## Project Context

{context}

## Skills

### {skill-1-name}

{skill-1-content}

### {skill-2-name}

{skill-2-content}

## Available Agents

- **{agent-name}**: {agent-description}
- **{agent-name}**: {agent-description}
```

Only non-empty sections are included. The "Project Context" section is populated from curated `.shelly/*.md` files and the auto-generated structural index. The "Available Agents" section is only present when a Registry is set and contains entries other than self.

---

## 11. Layer 7 - Cross-Cutting Concerns

### 11.1 State Store (`pkg/state/`)

Thread-safe key-value store implementing the blackboard pattern. Agents coordinate through structured data rather than natural language.

```go
type Store struct {
    mu     sync.RWMutex
    once   sync.Once
    signal chan struct{}   // Closed on Set/Delete, recreated immediately
    data   map[string]any
}
```

```
  Blackboard Pattern
  ──────────────────

  ┌──────────────┐     Set("findings", data)    ┌──────────────────┐
  │  Researcher  │─────────────────────────────►│                  │
  │  Agent       │                              │   State Store    │
  └──────────────┘                              │   (Blackboard)   │
                                                │                  │
  ┌──────────────┐     Get("findings")          │  "findings": ... │
  │  Writer      │◄─────────────────────────────│  "outline": ...  │
  │  Agent       │     -> data                  │  "status": ...   │
  └──────────────┘                              └──────────────────┘
```

**Tool integration:** The Store exposes itself as tools so agents can interact with it through their normal ReAct loop:

```go
stateTools := store.Tools("team")
// Creates: team_state_get, team_state_set, team_state_list
```

| Tool | Input | Output |
|------|-------|--------|
| `{ns}_state_get` | `{"key": "findings"}` | JSON-encoded value |
| `{ns}_state_set` | `{"key": "findings", "value": {...}}` | `"ok"` |
| `{ns}_state_list` | `{}` | `["findings", "outline", "status"]` |

**Watch:** `store.Watch(ctx, "findings")` blocks until the key exists (same signal channel pattern as `Chat`). Useful for agents that need to wait for another agent to produce a result.

---

## 12. Data Flow

### 12.1 Single Agent (ReAct)

```
  User                      Agent                    LLM              Tool
  ────                      ─────                    ───              ────
   │   "Summarize X"          │                        │                │
   │──────────────────────── ►│                        │                │
   │                           │   Complete(Chat)       │                │
   │                           │───────────────────────►│                │
   │                           │   ToolCall: search(X)  │                │
   │                           │◄───────────────────────│                │
   │                           │                        │   search(X)    │
   │                           │───────────────────────────────────────►│
   │                           │                        │   "results..." │
   │                           │◄───────────────────────────────────────│
   │                           │   Complete(Chat)       │                │
   │                           │───────────────────────►│                │
   │                           │   "Summary: ..."       │                │
   │                           │◄───────────────────────│                │
   │   "Summary: ..."          │                        │                │
   │◄──────────────────────── │                        │                │
```

### 12.2 Delegation Flow

```
  Orchestrator Agent               Worker Agent (spawned fresh)
  ──────────────────               ─────────────────────────────
  │ LLM: delegate                  │
  │      {tasks:[{agent:"worker", │
  │       task:"find X",           │
  │       context:"..."}]}        │
  │────────────────────────────── ►│
  │                                │ System prompt auto-generated
  │                                │ User message: "find X" injected
  │                                │ LLM: call search tool
  │                                │ search("X") -> "results..."
  │                                │ LLM: "Found: ..."
  │◄────────────────────────────── │ Return "Found: ..."
  │ ToolResult: "Found: ..."      │ (agent discarded)
  │ LLM: Now write the summary    │
  │ -> "Summary: ..."             │
```

### 12.3 Concurrent Delegation (multiple tasks)

```
  Orchestrator Agent           Worker A (goroutine)    Worker B (goroutine)
  ──────────────────           ────────────────────    ────────────────────
  │ LLM: delegate                │                       │
  │ {tasks:[                     │                       │
  │   {agent:"a", task:"...",    │                       │
  │    context:"..."},           │                       │
  │   {agent:"b", task:"...",    │                       │
  │    context:"..."}            │                       │
  │ ]}                           │                       │
  │──────────────────────────── ►│                       │
  │──────────────────────────────────────────────────── ►│
  │                              │ Run full ReAct loop   │ Run full ReAct loop
  │                              │ -> "result A"         │ -> "result B"
  │◄─────────────────────────── │                       │
  │◄───────────────────────────────────────────────────│
  │ WaitGroup.Wait()            │                       │
  │ ToolResult: JSON array      │                       │
  │ [{agent:"a",result:"..."},  │                       │
  │  {agent:"b",result:"..."}]  │                       │
```

---

## 13. Concurrency Model

### 13.1 Thread Safety Summary

| Component | Thread-Safe? | Mechanism |
|-----------|-------------|-----------|
| `Chat` | Yes | `sync.RWMutex` + signal channel |
| `State.Store` | Yes | `sync.RWMutex` + signal channel |
| `usage.Tracker` | Yes | `sync.Mutex` |
| `Agent` | **No** | Each delegation spawns a fresh instance |
| `Registry` | Yes | `sync.RWMutex` |
| `ToolBox` | No (read-only after setup) | Register before use, then read-only |

### 13.2 Delegate Concurrent Execution

```go
// Inside delegate tool handler:
ctx, cancel := context.WithCancel(ctx)
defer cancel()

results := make([]delegateResult, len(tasks))

var wg sync.WaitGroup
wg.Add(len(tasks))

for i, t := range tasks {
    go func() {
        defer wg.Done()

        child, ok := registry.Spawn(t.Agent, depth+1)
        if !ok {
            results[i] = spawnResult{Agent: t.Agent, Error: "not found"}
            cancel()
            return
        }

        child.registry = registry
        child.chat.Append(userMessage(t.Task))

        reply, err := child.Run(ctx)
        if err != nil {
            results[i] = spawnResult{Agent: t.Agent, Error: err.Error()}
            cancel()  // Cancel siblings on first error
            return
        }

        results[i] = spawnResult{Agent: t.Agent, Result: reply.TextContent()}
    }()
}

wg.Wait()
```

Key properties:
- Each spawned agent is a fresh instance (no shared mutable state)
- Results array uses index-based access (no mutex needed for separate indices)
- First error cancels the shared context, causing siblings to abort
- Results are returned in deterministic order (matching input task order)

### 13.3 Chat Signal Pattern

Both `Chat` and `Store` use the same signaling pattern for async notification:

```go
// Writer side (Append / Set):
s.mu.Lock()
s.data[key] = value
close(s.signal)              // Wake all waiters
s.signal = make(chan struct{}) // New channel for future waiters
s.mu.Unlock()

// Reader side (Wait / Watch):
for {
    s.mu.RLock()
    val, ok := s.data[key]
    sig := s.signal           // Capture current signal
    s.mu.RUnlock()

    if ok { return val, nil }

    select {
    case <-ctx.Done(): return nil, ctx.Err()
    case <-sig:        // Woken up, recheck
    }
}
```

---

## 14. Usage Examples

### 14.1 Simple Agent with Tools

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/germanamz/shelly/pkg/agent"
    "github.com/germanamz/shelly/pkg/providers/anthropic"
    "github.com/germanamz/shelly/pkg/tools/toolbox"
)

func main() {
    // 1. Create an LLM provider
    llm := anthropic.New("https://api.anthropic.com", "sk-...", "claude-sonnet-4-20250514")

    // 2. Create tools
    tb := toolbox.New()
    tb.Register(toolbox.Tool{
        Name:        "get_weather",
        Description: "Get the current weather for a city",
        InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {"city": {"type": "string"}},
            "required": ["city"]
        }`),
        Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
            return `{"temp": 72, "condition": "sunny"}`, nil
        },
    })

    // 3. Set tools on the provider (so they're sent to the LLM)
    llm.Tools = tb.Tools()

    // 4. Create and run the agent
    a := agent.New("weather-bot", "A helpful weather assistant", "Answer weather questions.", llm, agent.Options{
        MaxIterations: 5,
    })
    a.AddToolBoxes(tb)

    // 5. Add user message to the chat
    a.Chat().Append(message.NewText("user", role.User, "What's the weather in Paris?"))

    reply, err := a.Run(context.Background())
    if err != nil {
        panic(err)
    }

    fmt.Println(reply.TextContent())
}
```

### 14.2 Multi-Agent Delegation

```go
// Register worker agents in a registry
reg := agent.NewRegistry()

reg.Register("researcher", "Searches the web for information", func() *agent.Agent {
    llm := anthropic.New(baseURL, apiKey, model)
    llm.Tools = searchTB.Tools()
    a := agent.New("researcher", "Searches the web for information",
        "Find relevant information using search tools.", llm, agent.Options{
            MaxIterations: 10,
        })
    a.AddToolBoxes(searchTB)
    return a
})

reg.Register("writer", "Writes articles and summaries", func() *agent.Agent {
    llm := anthropic.New(baseURL, apiKey, model)
    return agent.New("writer", "Writes articles and summaries",
        "Write clear, well-structured content based on provided information.", llm, agent.Options{
            MaxIterations: 5,
        })
})

// Create orchestrator that can delegate to workers
orchLLM := anthropic.New(baseURL, apiKey, model)
orch := agent.New("orchestrator", "Coordinates research and writing",
    "Break complex tasks into research and writing subtasks.", orchLLM, agent.Options{
        MaxIterations:      10,
        MaxDelegationDepth: 3,
    })
orch.SetRegistry(reg)

// The orchestrator's ReAct loop will dynamically call list_agents,
// delegate as needed
orch.Chat().Append(message.NewText("user", role.User,
    "Write a summary of recent AI developments"))

result, err := orch.Run(ctx)
```

### 14.3 Agent with Skills

Skills are loaded globally from `.shelly/skills/` by the engine and injected into
all agents automatically. When using the engine, you don't need to load skills manually.

For standalone agent usage without the engine:

```go
// Load skills from a directory
skills, _ := skill.LoadDir(".shelly/skills/") // loads from subdirectories containing SKILL.md

// Create an agent that knows procedures
a := agent.New("project-lead", "Manages software projects",
    "You lead software development projects.", llm, agent.Options{
        MaxIterations: 20,
        Skills:        skills,
    })
a.SetRegistry(reg)

// The system prompt will include all skills as ### subsections,
// teaching the agent how to orchestrate, review code, etc.
```

### 14.4 Concurrent Work with delegate

```go
// The orchestrator can delegate to multiple agents concurrently
// by passing multiple tasks. The LLM decides when to use it:
//
// LLM: "I need to research US and Europe in parallel."
// LLM calls: delegate({tasks: [
//   {agent: "researcher", task: "Research AI developments in the US", context: "..."},
//   {agent: "researcher", task: "Research AI developments in Europe", context: "..."}
// ]})
//
// Both researchers run concurrently, results collected into JSON array.
// LLM: "Now I'll synthesize both results..."
```

### 14.5 Agent with Middleware

```go
a := agent.New("assistant", "Helpful bot", "Be helpful.", llm, agent.Options{
    MaxIterations: 10,
    Middleware: []agent.Middleware{
        agent.Logger(slog.Default(), "assistant"),     // Outermost: log everything
        agent.Timeout(60 * time.Second),               // 60s deadline
        agent.Recovery(),                              // Catch panics
        agent.OutputGuardrail(func(msg message.Message) error {
            if strings.Contains(msg.TextContent(), "CONFIDENTIAL") {
                return errors.New("output contains confidential information")
            }
            return nil
        }),
    },
})

result, err := a.Run(ctx)
```

### 14.6 Shared State Store

```go
// Agents coordinate through structured data
store := state.New()
stateTools := store.Tools("project")

// Researcher writes findings to state store
researcher := agent.New("researcher", "Finds information",
    "Research the topic. Store findings using project_state_set with key 'findings'.",
    researcherLLM, agent.Options{MaxIterations: 10})
researcher.AddToolBoxes(searchTB, stateTools)

// Writer reads findings from state store
writer := agent.New("writer", "Writes content",
    "Read findings from project_state_get with key 'findings' and write an article.",
    writerLLM, agent.Options{MaxIterations: 5})
writer.AddToolBoxes(stateTools)

// Register both in a registry for orchestration
reg := agent.NewRegistry()
reg.Register("researcher", "Finds information", func() *agent.Agent { /* ... */ })
reg.Register("writer", "Writes content", func() *agent.Agent { /* ... */ })
```

### 14.7 MCP Tool Integration

```go
// Connect to an external MCP tool server
mcpClient, _ := mcpclient.New(ctx, "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")

// Fetch its tools and register them
tools, _ := mcpClient.ListTools(ctx)
tb := toolbox.New()
tb.Register(tools...)

// Use in an agent -- the agent doesn't know these are remote tools
llm := anthropic.New(baseURL, apiKey, model)
llm.Tools = tb.Tools()

a := agent.New("file-agent", "Manages files", "Use the available tools to manage files.", llm, agent.Options{
    MaxIterations: 10,
})
a.AddToolBoxes(tb)
a.Chat().Append(message.NewText("user", role.User, "List files in /tmp"))

result, err := a.Run(ctx)
mcpClient.Close()
```

---

## 15. Interface Reference

### Core Interfaces

```go
// Completer -- LLM completion (the most important interface)
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}

// Runner -- middleware target
type Runner interface {
    Run(ctx context.Context) (message.Message, error)
}

// Part -- content within a message
type Part interface {
    PartKind() string
}

// Handler -- tool execution
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Middleware -- agent wrapper
type Middleware func(next Runner) Runner

// Factory -- creates fresh agent instances for delegation
type Factory func() *Agent
```

### Who Implements What

| Type | Implements |
|------|-----------|
| `agent.Agent` | `Runner` (via `Run` method) |
| `agent.RunnerFunc` | `Runner` |
| `anthropic.Adapter` | `Completer` |
| `openai.Adapter` | `Completer` |
| `grok.GrokAdapter` | `Completer` |
| `content.Text` | `Part` |
| `content.Image` | `Part` |
| `content.ToolCall` | `Part` |
| `content.ToolResult` | `Part` |

---

## 16. External Dependencies

The project maintains a minimal dependency footprint:

| Dependency | Purpose | Used By |
|-----------|---------|--------|
| `github.com/modelcontextprotocol/go-sdk` | MCP protocol implementation | `mcpclient/`, `mcpserver/` |
| `github.com/coder/websocket` | WebSocket client (indirect via modeladapter) | `modeladapter/` |
| `github.com/stretchr/testify` | Test assertions | All `*_test.go` files |
| `rsc.io/quote` | Legacy (placeholder) | `cmd/shelly/` |

Everything else uses Go's standard library (`net/http`, `encoding/json`, `sync`, `context`, `log/slog`).
