# SYSTEM.md - Shelly Architecture Reference

## Table of Contents

1. [Overview](#1-overview)
2. [Package Map](#2-package-map)
3. [Dependency Graph](#3-dependency-graph)
4. [Layer Architecture](#4-layer-architecture)
5. [Layer 1 - Chat Data Model](#5-layer-1---chat-data-model)
6. [Layer 2 - Model Adapter](#6-layer-2---model-adapter)
7. [Layer 3 - LLM Providers](#7-layer-3---llm-providers)
8. [Layer 4 - Tools](#8-layer-4---tools)
9. [Layer 5 - Agents](#9-layer-5---agents)
10. [Layer 6 - Coordination Patterns](#10-layer-6---coordination-patterns)
11. [Layer 7 - Cross-Cutting Concerns](#11-layer-7---cross-cutting-concerns)
12. [Data Flow](#12-data-flow)
13. [Concurrency Model](#13-concurrency-model)
14. [Composition & Nesting](#14-composition--nesting)
15. [Usage Examples](#15-usage-examples)
16. [Interface Reference](#16-interface-reference)
17. [External Dependencies](#17-external-dependencies)

---

## 1. Overview

Shelly is a provider-agnostic, multi-agent orchestration framework written in Go. It provides:

- A clean chat data model decoupled from any LLM provider
- Pluggable LLM adapters (Anthropic, OpenAI, Grok, or custom)
- Tool execution with MCP (Model Context Protocol) integration
- Multiple agent coordination patterns: ReAct loops, delegation, handoff, and reactor teams
- Composable middleware for guardrails, logging, and lifecycle hooks
- A shared state store (blackboard pattern) for structured inter-agent data

The framework is designed around **composition over inheritance**: every agent type implements the same `Agent` interface, and orchestration patterns (Reactor, Handoff, Delegation) compose with each other through this interface.

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
│   ├── usage/                             Token usage tracking
│   └── grok/                              Grok/xAI adapter (OpenAI-compatible)
│
├── pkg/providers/                       Layer 3: Concrete LLM providers
│   ├── anthropic/                         Anthropic Claude (Messages API)
│   └── openai/                            OpenAI GPT (Chat Completions API)
│
├── pkg/tools/                           Layer 4: Tool execution
│   ├── toolbox/                           Tool definition, registration, and execution
│   ├── mcpclient/                         MCP client (connects to external MCP servers)
│   └── mcpserver/                         MCP server (exposes tools via MCP protocol)
│
├── pkg/agents/                          Layer 5: Agent system
│   ├── react/                             ReAct loop (reason + act + observe)
│   ├── delegate/                          Agent-as-Tool (delegation pattern)
│   ├── handoff/                           Transfer control between agents
│   └── middleware/                        Composable agent middleware
│
├── pkg/reactor/                         Layer 6: Multi-agent orchestration
│                                          Coordinator-driven team execution
│
└── pkg/state/                           Layer 7: Shared state
                                           Thread-safe KV store (blackboard pattern)
```

---

## 3. Dependency Graph

Arrows point from dependent to dependency. The graph enforces a strict layered architecture with no circular dependencies.

```
                            ┌─────────────────────────────────┐
                            │          cmd/shelly/             │
                            └──────────────┬──────────────────┘
                                           │ imports everything
              ┌────────────────────────────┼────────────────────────────┐
              │                            │                            │
    ┌─────────▼─────────┐   ┌─────────────▼────────────┐   ┌──────────▼─────────┐
    │     reactor/       │   │    agents/middleware/     │   │      state/         │
    │  (orchestration)   │   │    (agent wrappers)       │   │  (shared KV store)  │
    └─────────┬──────────┘   └─────────────┬────────────┘   └──────────┬─────────┘
              │                            │                            │
              │ uses                       │ wraps                      │ exposes as
              │                            │                            │
    ┌─────────▼──────────────────┐   ┌─────▼──────┐             ┌──────▼──────┐
    │         agents/            │   │  reactor/   │             │   toolbox/   │
    │  ├── react/                │   │ (NamedAgent)│             │   (Tool)     │
    │  ├── delegate/             │   └─────────────┘             └─────────────┘
    │  └── handoff/              │
    └─────────┬──────────────────┘
              │
     ┌────────┼──────────────────────┐
     │        │                      │
┌────▼────┐ ┌─▼──────────┐ ┌────────▼───────┐
│ toolbox/ │ │modeladapter/│ │    chats/       │
│          │ │  ├── usage/ │ │ ├── role/       │
│          │ │  └── grok/  │ │ ├── content/    │
│          │ └─────┬───────┘ │ ├── message/    │
│          │       │         │ └── chat/        │
└────┬─────┘       │         └────────┬─────────┘
     │             │                  │
     └─────────────┴──────────────────┘
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

    ┌────────────────┐   ┌────────────────┐
    │ providers/      │   │ providers/      │
    │ anthropic/      │   │ openai/         │
    └───────┬────────┘   └───────┬────────┘
            │                    │
            └────────┬───────────┘
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
│  Layer 7: Cross-cutting     state/  middleware/                         │
│  Shared state, guardrails, logging, lifecycle hooks                     │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 6: Orchestration     reactor/                                    │
│  Multi-agent teams, coordinator-driven scheduling, concurrent execution │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 5: Agents            agents/  react/  delegate/  handoff/        │
│  Agent interface, ReAct loop, delegation, handoff                       │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 4: Tools             toolbox/  mcpclient/  mcpserver/            │
│  Tool definition, execution, MCP protocol integration                   │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 3: Providers         anthropic/  openai/  grok/                  │
│  Concrete LLM implementations                                          │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 2: Model Adapter     modeladapter/  usage/                       │
│  Completer interface, HTTP/WS helpers, auth, token tracking             │
├─────────────────────────────────────────────────────────────────────────┤
│  Layer 1: Chat Data Model   role/  content/  message/  chat/            │
│  Provider-agnostic types, thread-safe containers                        │
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

The `Sender` field is critical for multi-agent scenarios. When multiple agents write to the same chat, `Sender` identifies who said what. The Reactor uses this to filter self-messages during sync.

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
    Client      *http.Client      // nil → http.DefaultClient
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

**Packages:** `pkg/providers/anthropic/`, `pkg/providers/openai/`, `pkg/modeladapter/grok/`
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
         │ → converts to []toolbox.Tool
         │ → each handler calls CallTool() remotely
         ▼
  ┌──────────────┐
  │   ToolBox    │ (local registry, same interface)
  └──────────────┘

  ┌──────────────┐                              ┌──────────────┐
  │  MCP Server  │── stdio/transport ──────────►│ External MCP │
  │  (mcpserver) │                              │   Client     │
  └──────┬───────┘                              └──────────────┘
         │ Register()
         │ → converts toolbox.Tool to MCP Tool
         │ → wraps Handler as MCP ToolHandler
         ▼
  ┌──────────────┐
  │ Serve(in,out)│ (reads requests, executes handlers, writes responses)
  └──────────────┘
```

---

## 9. Layer 5 - Agents

**Packages:** `pkg/agents/`, `pkg/agents/react/`, `pkg/agents/delegate/`, `pkg/agents/handoff/`
**Dependencies:** `chats/`, `modeladapter/`, `tools/toolbox/`

### 9.1 Agent Interface

The universal contract. Every agent type, orchestration pattern, and middleware implements this.

```go
type Agent interface {
    Run(ctx context.Context) (message.Message, error)
}
```

### 9.2 AgentBase

Shared functionality embedded by concrete agent types.

```go
type AgentBase struct {
    Name         string
    ModelAdapter modeladapter.Completer
    ToolBoxes    []*toolbox.ToolBox
    Chat         *chat.Chat
}
```

```
                AgentBase
                ─────────
  ┌──────────────────────────────────┐
  │                                  │
  │  Complete(ctx)                   │    1. Call ModelAdapter.Complete()
  │    → model reply                 │    2. Set reply.Sender = Name
  │    → appended to Chat            │    3. Append to Chat
  │                                  │
  │  CallTools(ctx, msg)             │    1. Extract ToolCalls from msg
  │    → search ToolBoxes in order   │    2. Find handler in ToolBoxes
  │    → execute each handler        │    3. Append ToolResults to Chat
  │    → return results              │
  │                                  │
  │  Tools()                         │    Aggregate from all ToolBoxes
  │  AgentName() → Name             │
  │  AgentChat() → Chat             │
  └──────────────────────────────────┘
```

**AgentBase is NOT safe for concurrent use.** Callers must synchronize externally (the Reactor handles this).

### 9.3 ReAct Agent (`pkg/agents/react/`)

Implements the Reason + Act pattern: iterative cycles of LLM completion and tool execution.

```go
type ReActAgent struct {
    agents.AgentBase
    Options Options  // MaxIterations (0 = unlimited)
}
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
                   ▼                       │  CallTools   │    │
            ┌──────────────┐               │  (Act)       │    │
            │ Return final │               └──────┬───────┘    │
            │ answer       │                      │            │
            └──────────────┘                      │ Tool results  │
                                                  │ appended to   │
                                                  │ Chat (Observe)│
                                                  └────────────────┘
```

### 9.4 Delegation (`pkg/agents/delegate/`)

Wraps any `NamedAgent` as a `toolbox.Tool`, enabling an agent to call another agent through its normal tool-calling mechanism.

```go
type AgentTool struct {
    agent       reactor.NamedAgent
    description string
    inputSchema json.RawMessage
}
```

```
  Delegation Flow
  ───────────────
  ┌─────────────────────┐      Tool call        ┌──────────────────────┐
  │  Orchestrator Agent  │      "researcher"     │   Researcher Agent    │
  │  (ReAct loop)        │─────────────────────►│   (ReAct loop)        │
  │                      │      {"task":"..."}   │                       │
  │  Has researcher      │                       │  Private chat:        │
  │  as a Tool           │                       │  ├─ user: "task..."   │
  │                      │◄─────────────────────│  ├─ assistant: ...     │
  │                      │      Tool result      │  ├─ tool: ...         │
  │                      │      (text reply)     │  └─ assistant: answer │
  └─────────────────────┘                       └──────────────────────┘

  Key: Only the final text reply crosses the boundary.
       All intermediate reasoning stays in the sub-agent's private chat.
       The sub-agent is stateful (keeps history across calls).
```

Usage:

```go
// Wrap an agent as a tool
tool := delegate.NewAgentTool(researcherAgent, "Researches topics")

// Register in a toolbox
tb := toolbox.New()
tb.Register(tool.Tool())

// The orchestrator can now call the researcher through its ReAct loop
orchestrator := react.New(agents.NewAgentBase("orchestrator", llm, chat, tb), react.Options{})
```

### 9.5 Handoff (`pkg/agents/handoff/`)

Enables agents to dynamically transfer control to each other mid-conversation. Unlike Delegation (where the caller stays in control), Handoff is a full transfer -- the original agent gives up control.

```go
type HandoffAgent struct {
    name    string
    members map[string]reactor.NamedAgent
    active  string         // Currently active agent name
    chat    *chat.Chat     // SHARED chat (all agents see everything)
    opts    Options        // MaxHandoffs
}
```

```
  Handoff Flow
  ────────────
  ┌─────────────────────────────────────────────────────────────────────┐
  │                        SHARED CHAT                                  │
  │  user: "Book a flight and a hotel"                                 │
  │  assistant (triage): "I'll connect you to our flights specialist"  │
  │  assistant (flights): "Flight booked: NYC→LAX on March 5"         │
  │  assistant (flights): "Let me transfer you to hotel bookings"      │
  │  assistant (hotels): "Hotel booked: Hilton LAX, March 5-7"        │
  └─────────────────────────────────────────────────────────────────────┘

  Internal flow:

  ┌─────────┐  transfer_to_flights  ┌──────────┐  transfer_to_hotels  ┌─────────┐
  │ Triage  │──────────────────────►│ Flights  │─────────────────────►│ Hotels  │
  │ Agent   │                       │ Agent    │                      │ Agent   │
  └─────────┘                       └──────────┘                      └────┬────┘
                                                                           │
                                                                    Final answer
                                                                    (no handoff)
```

**Key difference from Reactor:** In Handoff, all agents share ONE chat and the agents decide who runs next (by calling `transfer_to_<name>` tools). In Reactor, each agent has a PRIVATE chat and a Coordinator decides who runs next.

**Transfer mechanism:**

```
  Agent calls transfer_to_hotels tool
         │
         ▼
  Tool handler returns HandoffError{Target: "hotels"}
         │
         ▼
  ReAct loop propagates error up to HandoffAgent.Run()
         │
         ▼
  HandoffAgent catches HandoffError via errors.As()
         │
         ▼
  Sets active = "hotels", continues loop
```

**Agent factory pattern:** Since HandoffAgent controls the shared chat and transfer tools, it uses a factory to build each agent:

```go
type AgentFactory func(shared *chat.Chat, transferTools *toolbox.ToolBox) reactor.NamedAgent
```

---

## 10. Layer 6 - Coordination Patterns

**Package:** `pkg/reactor/`
**Dependencies:** `agents/`, `chats/`

### 10.1 Reactor

The Reactor orchestrates multiple agents over a shared conversation using a pluggable `Coordinator`. Each agent maintains its own private chat; the Reactor bridges messages between shared and private using per-agent cursors.

```go
type Reactor struct {
    name        string
    members     []TeamMember
    shared      *chat.Chat       // Shared conversation
    cursors     []int            // Per-agent sync position
    coordinator Coordinator
}
```

### 10.2 Core Types

```go
type TeamRole string                    // Agent's function ("research", "write", "review")

type TeamMember struct {
    Agent NamedAgent
    Role  TeamRole
}

type Selection struct {
    Members []int                       // Indices to run (concurrent if multiple)
    Done    bool                        // True = orchestration complete
}

type Coordinator interface {
    Next(ctx context.Context, shared *chat.Chat, members []TeamMember) (Selection, error)
}
```

### 10.3 Message Sync Algorithm

```
  SHARED CHAT                                  AGENT PRIVATE CHAT
  ───────────                                  ──────────────────
  ┌─────────────────────┐  cursor=2            ┌─────────────────────┐
  │ [0] user: "task"    │                      │ system: "You are.." │
  │ [1] researcher: "x" │  ──── skip ────────► │                     │
  │ [2] writer: "y"     │  ──── sync ────────► │ user (writer): "y"  │
  │ [3] user: "more"    │  ──── sync ────────► │ user (user): "more" │
  └─────────────────────┘  cursor→4            └─────────────────────┘

  Rules:
  1. Fetch Since(cursor) from shared chat
  2. Skip messages where Sender == agent name (already in private chat)
  3. Remap role to User (preserve Sender, Parts, Metadata)
  4. Append to agent's private chat
  5. Advance cursor
```

**Why role remapping?** Agents receive other agents' messages as User-role messages. This way, from each agent's perspective, it's having a normal conversation with a user. The agent doesn't need to know about the multi-agent setup.

### 10.4 Execution Modes

```
  Single member selected:           Multiple members selected:
  ──────────────────────            ──────────────────────────
  sync → run → append               sync all → run concurrently → append in order
                                     │                              │
                                     ├── goroutine A (agent 0)      │
                                     ├── goroutine B (agent 1)      │
                                     └── WaitGroup.Wait()           │
                                                                    │
                                     Append results[0], results[1]
                                     (deterministic order)
```

### 10.5 Built-in Coordinators

```
  ┌────────────────────────────────────────────────────────────────────┐
  │                        Coordinators                                │
  ├──────────────────┬─────────────────────────────────────────────────┤
  │ Sequence         │ Run each member once in order, then done        │
  │                  │ [0] → [1] → [2] → done                         │
  ├──────────────────┼─────────────────────────────────────────────────┤
  │ Loop             │ Round-robin cycling with optional max rounds    │
  │                  │ [0] → [1] → [0] → [1] → ... → ErrMaxRounds    │
  ├──────────────────┼─────────────────────────────────────────────────┤
  │ RoundRobinUntil  │ Loop until predicate(chat) returns true        │
  │                  │ [0] → [1] → [0] → ... → predicate true → done │
  ├──────────────────┼─────────────────────────────────────────────────┤
  │ RoleRoundRobin   │ Cycle through roles. All members matching      │
  │                  │ the current role run concurrently               │
  │                  │ [research×2] → [write×1] → [research×2] → ...  │
  ├──────────────────┼─────────────────────────────────────────────────┤
  │ LLMCoordinator   │ An LLM decides who runs next via JSON response │
  │                  │ Maintains its own private reasoning chat         │
  │                  │ Sliding window of shared chat as context         │
  │                  │ Retry on invalid JSON (once)                    │
  └──────────────────┴─────────────────────────────────────────────────┘
```

### 10.6 LLM Coordinator Detail

The LLMCoordinator uses an LLM to dynamically decide which agent(s) should act next.

```go
type LLMCoordinator struct {
    completer   modeladapter.Completer
    maxRounds   int
    step        int
    chat        *chat.Chat         // Coordinator's own reasoning chat
    descriptors []MemberDescriptor // Optional human-readable descriptions
    windowSize  int                // Sliding window size (default 20)
}
```

```
  LLM Coordinator Flow
  ────────────────────
  ┌──────────────┐     "Recent conversation:       ┌──────────────┐
  │ Shared Chat  │     [user] task: ...             │ LLM          │
  │ (last 20 msg)│────►[researcher] findings: ...   │ (coordinator │
  │              │     [writer] draft: ..."          │  completer)  │
  └──────────────┘                                  └──────┬───────┘
                                                           │
                       ┌───────────────────────────────────┘
                       │  {"members": [1], "done": false}
                       ▼
                ┌──────────────┐
                │  Selection   │
                │  Members: [1]│   → Run writer
                │  Done: false │
                └──────────────┘
```

System prompt template:
```
You are a team coordinator. Based on the conversation, decide which
team member(s) should act next.

Team members:
- [0] "researcher" (role: research) — Searches the web for information
- [1] "writer" (role: write) — Writes articles and summaries

Respond with ONLY a JSON object (no markdown, no explanation):
{"members": [0], "done": false}
```

---

## 11. Layer 7 - Cross-Cutting Concerns

### 11.1 Middleware (`pkg/agents/middleware/`)

Composable wrappers around the `Agent` interface.

```go
type Middleware func(next agents.Agent) agents.Agent

func Chain(mws ...Middleware) Middleware
func Apply(agent agents.Agent, mws ...Middleware) agents.Agent
```

```
  Middleware Stack (outermost → innermost)
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
  │  │  │  │  │  Actual Agent (ReAct, Handoff, etc)│  │  │  │  │
  │  │  │  │  └────────────────────────────────────┘  │  │  │  │
  │  │  │  └──────────────────────────────────────────┘  │  │  │
  │  │  └────────────────────────────────────────────────┘  │  │
  │  └──────────────────────────────────────────────────────┘  │
  └────────────────────────────────────────────────────────────┘

  Call order: Logger.Run → Timeout.Run → Recovery.Run → Guardrail.Run → Agent.Run
  Return order: Agent returns → Guardrail checks → Recovery catches panics → Timeout checks → Logger logs
```

Built-in middleware:

| Middleware | What It Does |
|-----------|-------------|
| `Timeout(d)` | Wraps context with `context.WithTimeout` |
| `Recovery()` | Catches panics, converts to `fmt.Errorf("agent panicked: %v", r)` |
| `Logger(log)` | Logs agent name, start/stop, duration, errors via `slog.Logger` |
| `OutputGuardrail(fn)` | Validates final message; returns error if `fn(msg)` fails |

**NamedAgent preservation:** All middleware wrappers embed `namedAgentBase` which delegates `AgentName()` and `AgentChat()` to the inner agent when it implements `reactor.NamedAgent`. This means middleware-wrapped agents can still be used inside Reactors and Handoff groups.

### 11.2 State Store (`pkg/state/`)

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
  │  Agent       │     → data                   │  "status": ...   │
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
  User                    ReActAgent                 LLM              Tool
  ────                    ──────────                 ───              ────
   │   "Summarize X"        │                         │                │
   │──────────────────────►│                         │                │
   │                        │   Complete(Chat)        │                │
   │                        │────────────────────────►│                │
   │                        │   ToolCall: search(X)   │                │
   │                        │◄────────────────────────│                │
   │                        │                         │   search(X)    │
   │                        │────────────────────────────────────────►│
   │                        │                         │   "results..." │
   │                        │◄────────────────────────────────────────│
   │                        │   Complete(Chat)        │                │
   │                        │────────────────────────►│                │
   │                        │   "Summary: ..."        │                │
   │                        │◄────────────────────────│                │
   │   "Summary: ..."       │                         │                │
   │◄──────────────────────│                         │                │
```

### 12.2 Multi-Agent Team (Reactor)

```
  User              Reactor            Coordinator       Researcher      Writer
  ────              ───────            ───────────       ──────────      ──────
   │  "Write about X" │                   │                │              │
   │────────────────►│                   │                │              │
   │                  │  Next()           │                │              │
   │                  │──────────────────►│                │              │
   │                  │  Selection:[0]    │                │              │
   │                  │◄──────────────────│                │              │
   │                  │                   │                │              │
   │                  │  sync + Run()     │                │              │
   │                  │───────────────────────────────────►│              │
   │                  │  "Found info..."  │                │              │
   │                  │◄──────────────────────────────────│              │
   │                  │                   │                │              │
   │                  │  Next()           │                │              │
   │                  │──────────────────►│                │              │
   │                  │  Selection:[1]    │                │              │
   │                  │◄──────────────────│                │              │
   │                  │                   │                │              │
   │                  │  sync + Run()     │                │              │
   │                  │─────────────────────────────────────────────────►│
   │                  │  "Article: ..."   │                │              │
   │                  │◄─────────────────────────────────────────────────│
   │                  │                   │                │              │
   │                  │  Next()           │                │              │
   │                  │──────────────────►│                │              │
   │                  │  Done             │                │              │
   │                  │◄──────────────────│                │              │
   │  "Article: ..."  │                   │                │              │
   │◄────────────────│                   │                │              │
```

### 12.3 Delegation Flow

```
  Orchestrator (ReAct)           Researcher (ReAct, as Tool)
  ────────────────────           ──────────────────────────
  │ LLM: call researcher         │
  │      {"task":"find X"}       │
  │──────────────────────────────►│
  │                               │ Append "find X" to private chat
  │                               │ LLM: call search tool
  │                               │ search("X") → "results..."
  │                               │ LLM: "Found: ..."
  │◄──────────────────────────────│ Return "Found: ..."
  │ ToolResult: "Found: ..."     │
  │ LLM: Now write the summary   │
  │ → "Summary: ..."             │
```

---

## 13. Concurrency Model

### 13.1 Thread Safety Summary

| Component | Thread-Safe? | Mechanism |
|-----------|-------------|-----------|
| `Chat` | Yes | `sync.RWMutex` + signal channel |
| `State.Store` | Yes | `sync.RWMutex` + signal channel |
| `usage.Tracker` | Yes | `sync.Mutex` |
| `AgentBase` | **No** | Callers must synchronize |
| `ToolBox` | No (read-only after setup) | Register before use, then read-only |
| `Reactor` | Manages its own sync | Syncs agents before concurrent launch |

### 13.2 Reactor Concurrent Execution

```go
// When multiple members are selected:
func (r *Reactor) runConcurrent(ctx context.Context, indices []int) error {
    // 1. Sync all agents BEFORE launching goroutines
    for _, idx := range indices {
        r.syncToAgent(idx)
    }

    // 2. Cancel context on first error
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    // 3. Run agents in parallel
    results := make([]result, len(indices))
    var wg sync.WaitGroup
    wg.Add(len(indices))
    for i, idx := range indices {
        go func() {
            defer wg.Done()
            reply, err := r.members[idx].Agent.Run(ctx)
            results[i] = result{reply, err}
            if err != nil { cancel() }
        }()
    }
    wg.Wait()

    // 4. Append replies in selection order (deterministic)
    for i := range indices {
        if results[i].err == nil {
            r.shared.Append(results[i].reply)
        }
    }
}
```

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

## 14. Composition & Nesting

Every coordination pattern implements `Agent` (and usually `NamedAgent`), making them composable. Here's how patterns nest:

```
  ┌────────────────────────────────────────────────────────────────────┐
  │  Reactor (top-level)                                               │
  │  Coordinator: LLMCoordinator                                       │
  │                                                                    │
  │  ┌────────────────────────┐  ┌──────────────────────────────────┐  │
  │  │ Member 0: ReActAgent   │  │ Member 1: HandoffAgent           │  │
  │  │ "researcher"           │  │ "customer-support"                │  │
  │  │                        │  │                                   │  │
  │  │ Tools:                 │  │  ┌─────────┐    ┌──────────┐     │  │
  │  │  - web_search          │  │  │ Triage  │───►│ Billing  │     │  │
  │  │  - analyze_data        │  │  │ Agent   │    │ Agent    │     │  │
  │  │                        │  │  └─────────┘    └──────────┘     │  │
  │  │ Middleware:             │  │       │              │           │  │
  │  │  - Timeout(60s)        │  │       └──────────────┘           │  │
  │  │  - Logger              │  │       transfer_to_* tools        │  │
  │  └────────────────────────┘  └──────────────────────────────────┘  │
  │                                                                    │
  │  ┌────────────────────────┐                                        │
  │  │ Member 2: ReActAgent   │                                        │
  │  │ "writer"               │                                        │
  │  │                        │                                        │
  │  │ Tools:                 │                                        │
  │  │  - write_document      │                                        │
  │  │  - researcher (delegate│  ← Agent-as-Tool wrapping Member 0    │
  │  │    AgentTool)           │                                        │
  │  └────────────────────────┘                                        │
  └────────────────────────────────────────────────────────────────────┘
```

**What this enables:**
- A Reactor can contain HandoffAgents as members
- A ReActAgent can delegate to another agent via AgentTool
- Middleware wraps any agent transparently
- Nested Reactors: a Reactor member can itself be a Reactor

---

## 15. Usage Examples

### 15.1 Simple ReAct Agent with Tools

```go
package main

import (
    "context"
    "fmt"

    "github.com/germanamz/shelly/pkg/agents"
    "github.com/germanamz/shelly/pkg/agents/react"
    "github.com/germanamz/shelly/pkg/chats/chat"
    "github.com/germanamz/shelly/pkg/chats/message"
    "github.com/germanamz/shelly/pkg/chats/role"
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

    // 4. Create a chat with a system prompt and user message
    c := chat.New(
        message.NewText("", role.System, "You are a helpful weather assistant."),
        message.NewText("user", role.User, "What's the weather in Paris?"),
    )

    // 5. Create and run a ReAct agent
    base := agents.NewAgentBase("weather-bot", llm, c, tb)
    agent := react.New(base, react.Options{MaxIterations: 5})

    reply, err := agent.Run(context.Background())
    if err != nil {
        panic(err)
    }

    fmt.Println(reply.TextContent())
}
```

### 15.2 Research + Writing Pipeline (Reactor with Sequence)

```go
// Two agents: one researches, one writes. Run in sequence.
llm := anthropic.New(baseURL, apiKey, "claude-sonnet-4-20250514")

// Researcher with search tool
searchTB := toolbox.New()
searchTB.Register(searchTool)
researcherLLM := anthropic.New(baseURL, apiKey, "claude-sonnet-4-20250514")
researcherLLM.Tools = searchTB.Tools()

researcher := react.New(
    agents.NewAgentBase("researcher", researcherLLM, chat.New(
        message.NewText("", role.System, "You are a research specialist. Find relevant information."),
    ), searchTB),
    react.Options{MaxIterations: 10},
)

// Writer with no special tools
writer := react.New(
    agents.NewAgentBase("writer", llm, chat.New(
        message.NewText("", role.System, "You are a skilled writer. Write based on research findings."),
    )),
    react.Options{MaxIterations: 5},
)

// Shared chat with the user's request
shared := chat.New(
    message.NewText("user", role.User, "Write a summary of recent AI developments"),
)

// Create reactor with sequence coordinator
r, _ := reactor.New("pipeline", shared, []reactor.TeamMember{
    {Agent: researcher, Role: "research"},
    {Agent: writer, Role: "write"},
}, reactor.Options{
    Coordinator: reactor.NewSequence(),
})

// Run: researcher goes first, writer goes second
result, err := r.Run(context.Background())
fmt.Println(result.TextContent())
```

### 15.3 LLM-Orchestrated Team

```go
// An LLM decides who should act next
coordinatorLLM := anthropic.New(baseURL, apiKey, "claude-sonnet-4-20250514")

coordinator := reactor.NewLLMCoordinator(coordinatorLLM, 10,
    reactor.MemberDescriptor{Description: "Searches the web for information"},
    reactor.MemberDescriptor{Description: "Writes articles and summaries"},
    reactor.MemberDescriptor{Description: "Reviews content for accuracy and style"},
)

r, _ := reactor.New("smart-team", shared, []reactor.TeamMember{
    {Agent: researcher, Role: "research"},
    {Agent: writer,     Role: "write"},
    {Agent: reviewer,   Role: "review"},
}, reactor.Options{
    Coordinator: coordinator,
})

// The LLM coordinator sees the conversation and decides:
// "The user asked for a researched article. Let's start with research."
// → Selection{Members: [0]} (researcher)
// After research: "Good findings. Now the writer should draft."
// → Selection{Members: [1]} (writer)
// After writing: "Let's have the reviewer check this."
// → Selection{Members: [2]} (reviewer)
// After review: "The article looks good. We're done."
// → Selection{Done: true}
result, err := r.Run(ctx)
```

### 15.4 Agent Delegation (Agent-as-Tool)

```go
// A researcher agent that an orchestrator can call as a tool
researcher := react.New(
    agents.NewAgentBase("researcher", researcherLLM, chat.New(
        message.NewText("", role.System, "You are a research specialist."),
    ), searchTB),
    react.Options{MaxIterations: 10},
)

// Wrap as a tool
researchTool := delegate.NewAgentTool(researcher, "Delegate a research task to a specialist")

// Create orchestrator with the delegation tool
orchTB := toolbox.New()
orchTB.Register(researchTool.Tool())
orchLLM := anthropic.New(baseURL, apiKey, "claude-sonnet-4-20250514")
orchLLM.Tools = orchTB.Tools()

orchestrator := react.New(
    agents.NewAgentBase("orchestrator", orchLLM, chat.New(
        message.NewText("", role.System, "You coordinate tasks. Use the researcher tool for fact-finding."),
        message.NewText("user", role.User, "What are the top 3 AI trends in 2026?"),
    ), orchTB),
    react.Options{MaxIterations: 10},
)

// The orchestrator's ReAct loop will call the researcher tool,
// which triggers a full ReAct loop inside the researcher agent.
// Only the researcher's final answer comes back as the tool result.
result, err := orchestrator.Run(ctx)
```

### 15.5 Customer Support with Handoffs

```go
shared := chat.New(
    message.NewText("", role.System, "You are part of a customer support team."),
    message.NewText("user", role.User, "I need to change my flight and also update my hotel booking."),
)

h, _ := handoff.New("support", shared, []handoff.Member{
    {
        Name: "triage",
        Factory: func(c *chat.Chat, transfers *toolbox.ToolBox) reactor.NamedAgent {
            triageLLM := anthropic.New(baseURL, apiKey, model)
            triageLLM.Tools = transfers.Tools() // transfer_to_flights, transfer_to_hotels
            return react.New(
                agents.NewAgentBase("triage", triageLLM, c, transfers),
                react.Options{MaxIterations: 3},
            )
        },
    },
    {
        Name: "flights",
        Factory: func(c *chat.Chat, transfers *toolbox.ToolBox) reactor.NamedAgent {
            flightLLM := anthropic.New(baseURL, apiKey, model)
            combined := toolbox.New()
            combined.Register(flightBookingTool)
            combined.Register(transfers.Tools()...) // Can hand off to hotels
            flightLLM.Tools = append(flightBookingTools(), transfers.Tools()...)
            return react.New(
                agents.NewAgentBase("flights", flightLLM, c, combined, transfers),
                react.Options{MaxIterations: 5},
            )
        },
    },
    {
        Name: "hotels",
        Factory: func(c *chat.Chat, transfers *toolbox.ToolBox) reactor.NamedAgent {
            hotelLLM := anthropic.New(baseURL, apiKey, model)
            hotelLLM.Tools = append(hotelBookingTools(), transfers.Tools()...)
            combined := toolbox.New()
            combined.Register(hotelBookingTool)
            return react.New(
                agents.NewAgentBase("hotels", hotelLLM, c, combined, transfers),
                react.Options{MaxIterations: 5},
            )
        },
    },
}, handoff.Options{MaxHandoffs: 10})

// Flow: triage → flights → hotels → final answer
result, err := h.Run(ctx)
```

### 15.6 Shared State Store (Blackboard Pattern)

```go
// Agents coordinate through structured data
shared := state.New()
stateTools := shared.Tools("project")

// Researcher writes findings to state store
researcherTB := toolbox.New()
researcherTB.Register(searchTool)
researcherLLM := anthropic.New(baseURL, apiKey, model)
researcherLLM.Tools = append(researcherTB.Tools(), stateTools.Tools()...)

researcher := react.New(
    agents.NewAgentBase("researcher", researcherLLM, chat.New(
        message.NewText("", role.System,
            "Research the topic. Store your findings using project_state_set with key 'findings'."),
    ), researcherTB, stateTools),
    react.Options{MaxIterations: 10},
)

// Writer reads findings from state store
writerLLM := anthropic.New(baseURL, apiKey, model)
writerLLM.Tools = stateTools.Tools()

writer := react.New(
    agents.NewAgentBase("writer", writerLLM, chat.New(
        message.NewText("", role.System,
            "Read findings from project_state_get with key 'findings' and write an article."),
    ), stateTools),
    react.Options{MaxIterations: 5},
)

// Use reactor to orchestrate
r, _ := reactor.New("pipeline", chatWithUserRequest, []reactor.TeamMember{
    {Agent: researcher, Role: "research"},
    {Agent: writer, Role: "write"},
}, reactor.Options{Coordinator: reactor.NewSequence()})

result, err := r.Run(ctx)

// You can also inspect the state programmatically:
findings, _ := shared.Get("findings")
```

### 15.7 Middleware-Wrapped Agent

```go
agent := react.New(base, react.Options{MaxIterations: 10})

// Wrap with middleware
guarded := middleware.Apply(agent,
    middleware.Logger(slog.Default()),           // Outermost: log everything
    middleware.Timeout(60 * time.Second),        // 60s deadline
    middleware.Recovery(),                       // Catch panics
    middleware.OutputGuardrail(func(msg message.Message) error {
        if strings.Contains(msg.TextContent(), "CONFIDENTIAL") {
            return errors.New("output contains confidential information")
        }
        return nil
    }),
)

// Use like any other agent (still implements NamedAgent!)
result, err := guarded.Run(ctx)

// Can also be used inside a Reactor
r, _ := reactor.New("safe-team", shared, []reactor.TeamMember{
    {Agent: guarded.(reactor.NamedAgent), Role: "worker"},
}, reactor.Options{Coordinator: reactor.NewSequence()})
```

### 15.8 Concurrent Agents with Role-Based Scheduling

```go
// Two researchers work in parallel, then one writer processes their combined output
shared := chat.New(
    message.NewText("user", role.User, "Compare AI developments in US vs Europe in 2026"),
)

r, _ := reactor.New("parallel-research", shared, []reactor.TeamMember{
    {Agent: usResearcher,  Role: "research"},  // index 0
    {Agent: euResearcher,  Role: "research"},  // index 1
    {Agent: writer,        Role: "write"},     // index 2
}, reactor.Options{
    Coordinator: reactor.NewRoleRoundRobin(1, "research", "write"),
})

// Step 1: RoleRoundRobin selects role "research"
//         → Both researchers (indices 0,1) run CONCURRENTLY
//         → Both replies appended to shared chat
// Step 2: RoleRoundRobin selects role "write"
//         → Writer (index 2) runs, sees both research results
//         → Writer produces final article
result, err := r.Run(ctx)
```

### 15.9 Nested Reactors (Hierarchical Teams)

```go
// Inner reactor: research team (2 researchers + 1 synthesizer)
researchTeam, _ := reactor.New("research-team", chat.New(), []reactor.TeamMember{
    {Agent: researcher1, Role: "research"},
    {Agent: researcher2, Role: "research"},
    {Agent: synthesizer, Role: "synthesize"},
}, reactor.Options{
    Coordinator: reactor.NewRoleRoundRobin(1, "research", "synthesize"),
})

// Outer reactor: research team + writer
// researchTeam implements NamedAgent, so it can be a member!
outerShared := chat.New(
    message.NewText("user", role.User, "Write a comprehensive report on quantum computing"),
)

pipeline, _ := reactor.New("full-pipeline", outerShared, []reactor.TeamMember{
    {Agent: researchTeam, Role: "research"},  // Entire team as one member
    {Agent: writer,       Role: "write"},
}, reactor.Options{
    Coordinator: reactor.NewSequence(),
})

result, err := pipeline.Run(ctx)
```

### 15.10 MCP Tool Integration

```go
// Connect to an external MCP tool server
mcpClient, _ := mcpclient.New(ctx, "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")

// Fetch its tools and register them
tools, _ := mcpClient.ListTools(ctx)
tb := toolbox.New()
tb.Register(tools...)

// Use in an agent — the agent doesn't know these are remote tools
llm := anthropic.New(baseURL, apiKey, model)
llm.Tools = tb.Tools()

agent := react.New(
    agents.NewAgentBase("file-agent", llm, chat.New(
        message.NewText("", role.System, "You manage files using the available tools."),
        message.NewText("user", role.User, "List files in /tmp"),
    ), tb),
    react.Options{MaxIterations: 10},
)

result, err := agent.Run(ctx)
mcpClient.Close()
```

---

## 16. Interface Reference

### Core Interfaces

```go
// Agent — the universal contract for all agent types
type Agent interface {
    Run(ctx context.Context) (message.Message, error)
}

// NamedAgent — extends Agent with identity and chat access
type NamedAgent interface {
    Agent
    AgentName() string
    AgentChat() *chat.Chat
}

// Completer — LLM completion
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}

// Coordinator — orchestration strategy
type Coordinator interface {
    Next(ctx context.Context, shared *chat.Chat, members []TeamMember) (Selection, error)
}

// Part — content within a message
type Part interface {
    PartKind() string
}

// Handler — tool execution
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Middleware — agent wrapper
type Middleware func(next agents.Agent) agents.Agent

// AgentFactory — builds agents for handoff groups
type AgentFactory func(shared *chat.Chat, transferTools *toolbox.ToolBox) reactor.NamedAgent
```

### Who Implements What

| Type | Implements |
|------|-----------|
| `AgentBase` | `AgentName()`, `AgentChat()` (but not `Agent` — no `Run` method) |
| `react.ReActAgent` | `Agent`, `NamedAgent` (via embedded `AgentBase`) |
| `reactor.Reactor` | `Agent`, `NamedAgent` |
| `handoff.HandoffAgent` | `Agent`, `NamedAgent` |
| Middleware wrappers | `Agent`, `NamedAgent` (delegated) |
| `anthropic.Adapter` | `Completer` |
| `openai.Adapter` | `Completer` |
| `grok.GrokAdapter` | `Completer` |
| `SequenceCoordinator` | `Coordinator` |
| `LoopCoordinator` | `Coordinator` |
| `RoundRobinUntilCoordinator` | `Coordinator` |
| `RoleRoundRobin` | `Coordinator` |
| `LLMCoordinator` | `Coordinator` |

---

## 17. External Dependencies

The project maintains a minimal dependency footprint:

| Dependency | Purpose | Used By |
|-----------|---------|--------|
| `github.com/modelcontextprotocol/go-sdk` | MCP protocol implementation | `mcpclient/`, `mcpserver/` |
| `github.com/coder/websocket` | WebSocket client (indirect via modeladapter) | `modeladapter/` |
| `github.com/stretchr/testify` | Test assertions | All `*_test.go` files |
| `rsc.io/quote` | Legacy (placeholder) | `cmd/shelly/` |

Everything else uses Go's standard library (`net/http`, `encoding/json`, `sync`, `context`, `log/slog`).
