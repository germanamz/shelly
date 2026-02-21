# Shelly Multi-Agent Coordination: Evaluation & Next Steps Plan

## Table of Contents

1. [Current Architecture Evaluation](#1-current-architecture-evaluation)
2. [Gap Analysis](#2-gap-analysis)
3. [Design Principles](#3-design-principles)
4. [Proposed Architecture](#4-proposed-architecture)
5. [Implementation Plan](#5-implementation-plan)
6. [Phase Details](#6-phase-details)
7. [Open Questions](#7-open-questions)

---

## 1. Current Architecture Evaluation

### 1.1 What We Have

The codebase has five well-layered packages with clean dependency boundaries:

```
chats/          Foundation: role, content, message, chat (thread-safe)
modeladapter/   LLM abstraction: Completer interface, HTTP/WS helpers, usage tracking
tools/          Tool execution: toolbox, mcpclient, mcpserver (MCP integration)
agents/         Agent base + ReAct loop
reactor/        Multi-agent orchestration with coordinators
```

### 1.2 Strengths for Multi-Agent Coordination

| Feature | Where | Why It Matters |
|---------|-------|----------------|
| Thread-safe `Chat` with `Wait()` signaling | `pkg/chats/chat/` | Async agent communication foundation |
| `Sender` field on every `Message` | `pkg/chats/message/` | Agent identity tracking in conversations |
| `Metadata map[string]any` on messages | `pkg/chats/message/` | Extensible coordination signals without schema changes |
| `content.Part` interface (extensible) | `pkg/chats/content/` | Custom part types for coordination without breaking existing code |
| `Agent` interface (single method: `Run`) | `pkg/agents/` | Simple, composable, any orchestration pattern can wrap it |
| `NamedAgent` with identity + private chat | `pkg/reactor/` | Private reasoning with shared output -- essential for multi-agent |
| Reactor with coordinator pattern | `pkg/reactor/` | Pluggable orchestration strategies |
| Private/shared chat split + cursor sync | `pkg/reactor/` | Agents see others' outputs as user input; internal reasoning stays private |
| Concurrent execution with `sync.WaitGroup` | `pkg/reactor/` | Parallel agent execution with deterministic output ordering |
| Nestable (Reactor implements NamedAgent) | `pkg/reactor/` | Hierarchical teams: a reactor of reactors |
| MCP integration | `pkg/tools/` | Standard protocol for tool interop |
| Multiple ToolBoxes per agent | `pkg/agents/` | Isolated + shared tool access already supported |

### 1.3 Verdict

**The current structure is a solid foundation.** The Reactor already supports the core multi-agent orchestration patterns (sequential, concurrent, role-based round-robin, nested hierarchies). The thread-safe Chat with cursor-based sync is a clean substrate for agent communication. The content.Part interface and message Metadata provide extension points without breaking changes.

**However**, the current system can only coordinate agents in a **centralized, deterministic** way. All coordination flows through the Reactor's coordinator, which has no LLM reasoning capability. Agents cannot communicate directly, delegate to each other, or dynamically form teams. The system supports "structured workflows" but not "emergent collaboration."

---

## 2. Gap Analysis

### 2.1 Critical Gaps (Block Real Use Cases)

| Gap | Impact | Current Workaround |
|-----|--------|--------------------|
| **No LLM-driven coordinator** | Can't build supervisor/orchestrator that reasons about which agent to run | Must hardcode coordination logic in deterministic coordinators |
| **No agent-as-tool (delegation)** | Agent A can't call Agent B as a tool during its ReAct loop | Would need manual wiring outside the framework |
| **No handoff mechanism** | Agent A can't transfer control to Agent B mid-conversation | Must pre-define the full sequence in a coordinator |
| **No concrete LLM provider** | Can't actually run anything end-to-end | `Completer` interface exists but no concrete implementation |

### 2.2 Important Gaps (Limit Flexibility)

| Gap | Impact | Current Workaround |
|-----|--------|--------------------|
| **No shared state/blackboard** | Agents can only share through natural language in chat | Abuse message Metadata for structured data passing |
| **No structured coordination messages** | Task assignment, status reports are unstructured text | Free-text in shared chat, hope the LLM parses it |
| **No guardrails/hooks** | Can't validate or transform agent inputs/outputs | Wrap agent with custom code before/after |
| **No dynamic team membership** | Members fixed at Reactor creation time | Create a new Reactor to change membership |
| **No error recovery / retry** | Single agent failure stops the whole reactor | Catch and handle manually in coordinator |

### 2.3 Nice-to-Have Gaps (Future Enhancement)

| Gap | Impact |
|-----|--------|
| No agent memory/persistence across sessions | Agents lose all context between runs |
| No streaming support in agent output | Must wait for full completion |
| No observability/tracing | Hard to debug multi-agent interactions |
| No resource budgeting (token limits per agent) | One agent can consume the entire budget |

---

## 3. Design Principles

Given the user's constraint of **in-house development with minimal external dependencies** (limited to MCP-standard-like things), these principles guide the design:

1. **Build on what exists.** Extend `Chat`, `content.Part`, `Message.Metadata`, `Coordinator`, and the `Agent` interface rather than replacing them.

2. **Go-native concurrency.** Use goroutines, channels, `context.Context`, `sync` primitives. No external messaging or actor frameworks.

3. **MCP as the interop boundary.** External tool integration uses MCP. Agent-to-agent communication within the process uses Go interfaces and channels.

4. **Chat as the communication substrate.** All agent communication flows through `Chat` instances (shared or direct). This keeps the architecture simple and debuggable.

5. **Composition over inheritance.** New capabilities (delegation, handoff, blackboard) should be composable add-ons, not changes to the core `Agent` or `AgentBase` types.

6. **Incremental delivery.** Each phase produces a working, testable system. Later phases build on earlier ones without requiring rewrites.

---

## 4. Proposed Architecture

### 4.1 High-Level View

```
                    ┌──────────────────────────────┐
                    │          Application          │
                    └──────────────┬───────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
        ┌─────▼─────┐     ┌───────▼──────┐     ┌──────▼─────┐
        │  Reactor   │     │  Delegator   │     │ Handoff    │
        │ (teams)    │     │ (agent→agent │     │ (transfer  │
        │            │     │  as tool)    │     │  control)  │
        └─────┬──────┘     └───────┬──────┘     └──────┬─────┘
              │                    │                    │
              └────────────────────┼────────────────────┘
                                   │
                           ┌───────▼───────┐
                           │   AgentBase   │
                           │ + middleware  │
                           └───────┬───────┘
                                   │
                    ┌──────────────┼──────────────┐
                    │              │              │
              ┌─────▼────┐  ┌─────▼────┐  ┌─────▼─────┐
              │Completer │  │  Chat    │  │ ToolBoxes │
              │ (LLM)    │  │          │  │           │
              └──────────┘  └──────────┘  └───────────┘
```

### 4.2 New Coordination Patterns

The plan introduces three complementary coordination patterns, each as a standalone package that composes with the existing system:

| Pattern | Package | Description | Real-World Analogy |
|---------|---------|-------------|-------------------|
| **Reactor (exists)** | `pkg/reactor/` | Coordinator picks who runs next over shared chat | A meeting moderator calling on speakers |
| **Delegation** | `pkg/agents/delegate/` | Agent A wraps Agent B as a callable tool | A manager asking a specialist for help |
| **Handoff** | `pkg/agents/handoff/` | Agent A transfers its conversation to Agent B | A phone call transfer |

### 4.3 New Components Summary

| Component | Package | Purpose |
|-----------|---------|---------|
| **LLM Coordinator** | `pkg/reactor/` | Coordinator that uses an LLM to pick the next agent(s) |
| **AgentTool** | `pkg/agents/delegate/` | Wraps any Agent as a `toolbox.Tool` for use in ReAct loops |
| **Handoff Agent** | `pkg/agents/handoff/` | Runs multiple agents with handoff transfer logic |
| **Middleware** | `pkg/agents/middleware/` | Composable hooks for agent input/output processing |
| **State Store** | `pkg/state/` | Thread-safe key-value store for structured inter-agent data |
| **Anthropic Adapter** | `pkg/providers/anthropic/` | Concrete Completer for Claude API |
| **OpenAI Adapter** | `pkg/providers/openai/` | Concrete Completer for OpenAI API |

---

## 5. Implementation Plan

### Phase Overview

```
Phase 0: Provider Implementation        [Foundation - unblocks everything]
Phase 1: LLM-Driven Coordinator         [Smart orchestration]
Phase 2: Delegation (Agent-as-Tool)     [Agent calling agent]
Phase 3: Handoff Pattern                [Transfer control between agents]
Phase 4: Middleware & Guardrails        [Agent lifecycle hooks]
Phase 5: Shared State Store             [Structured inter-agent data]
Phase 6: Integration & CLI              [Wire everything into shelly CLI]
```

### Dependency Graph

```
Phase 0 ──┬── Phase 1 (LLM Coordinator needs a Completer)
           ├── Phase 2 (Delegation tests need a real or mock Completer)
           ├── Phase 3 (Handoff tests need a real or mock Completer)
           └── Phase 6 (CLI needs providers)

Phase 1 ──── Phase 6

Phase 2 ──── Phase 6

Phase 3 ──── Phase 6

Phase 4 ──── (independent, can run in parallel with 1-3)

Phase 5 ──── (independent, can run in parallel with 1-3)
```

**Phases 1, 2, 3, 4, and 5 can be developed in parallel** after Phase 0. Phase 6 integrates them all.

---

## 6. Phase Details

### Phase 0: Concrete LLM Provider(s)

**Goal:** Implement at least one real `Completer` so the entire system can run end-to-end.

**Why first:** Every other phase needs an LLM to test. The mock providers in tests are useful for unit testing but don't validate real behavior.

#### 6.0.1 Anthropic Adapter (`pkg/providers/anthropic/`)

**Files to create:**
- `pkg/providers/anthropic/anthropic.go` — Claude API adapter
- `pkg/providers/anthropic/anthropic_test.go` — tests
- `pkg/providers/anthropic/README.md`

**Implementation details:**

```go
// pkg/providers/anthropic/anthropic.go

type Anthropic struct {
    modeladapter.ModelAdapter
    Model       string   // e.g. "claude-sonnet-4-20250514"
    SystemHint  string   // Optional system prompt handling
}

func New(apiKey, model string) *Anthropic {
    return &Anthropic{
        ModelAdapter: modeladapter.New(
            "https://api.anthropic.com",
            modeladapter.Auth{Key: apiKey, Header: "x-api-key", Scheme: ""},
            nil,
        ),
        Model: model,
    }
}

// Complete implements modeladapter.Completer.
// Maps chat.Chat → Anthropic Messages API request.
// Maps Anthropic response → message.Message with content.Parts.
func (a *Anthropic) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
    // 1. Extract system prompt from chat (first system message)
    // 2. Convert remaining messages to Anthropic format:
    //    - role.User → "user"
    //    - role.Assistant → "assistant"
    //    - content.Text → {"type":"text","text":"..."}
    //    - content.ToolCall → {"type":"tool_use","id":"...","name":"...","input":{...}}
    //    - content.ToolResult → {"type":"tool_result","tool_use_id":"...","content":"..."}
    //    - content.Image → {"type":"image","source":{...}}
    // 3. Build request with model, max_tokens, tools (from chat metadata or separate)
    // 4. POST to /v1/messages
    // 5. Parse response stop_reason:
    //    - "end_turn" → message with Text parts
    //    - "tool_use" → message with ToolCall parts
    // 6. Track usage via a.Usage.Add()
    // 7. Return message.Message with appropriate parts
}
```

**Key mapping decisions:**
- `role.Tool` messages → group consecutive tool results into a single `"user"` message with `tool_result` content blocks (Anthropic API requirement)
- System messages → extracted and sent as top-level `system` parameter
- Tool definitions → sent as `tools` array when agent has ToolBoxes

**External dependencies:** None beyond `net/http` (already used by `ModelAdapter`). The Anthropic API is a simple JSON REST API.

#### 6.0.2 OpenAI Adapter (`pkg/providers/openai/`)

**Files to create:**
- `pkg/providers/openai/openai.go`
- `pkg/providers/openai/openai_test.go`
- `pkg/providers/openai/README.md`

**Implementation details:**

```go
type OpenAI struct {
    modeladapter.ModelAdapter
    Model string // e.g. "gpt-4o"
}

func New(apiKey, model string) *OpenAI { ... }

// Complete maps chat → OpenAI Chat Completions API → message.
// Similar structure to Anthropic but different wire format.
func (o *OpenAI) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) { ... }
```

**Key differences from Anthropic:**
- Tool results are `role: "tool"` messages (not nested in user messages)
- System prompt is a `role: "system"` message (not a separate parameter)
- Tool calls use `function_call` / `tool_calls` response format

**Both adapters must pass a common integration test suite** that verifies the `Completer` contract:
- Single text completion
- Multi-turn conversation
- Tool call + tool result round-trip
- Error handling (rate limits, invalid input)

---

### Phase 1: LLM-Driven Coordinator

**Goal:** A `Coordinator` that uses an LLM to decide which agent(s) should act next, enabling dynamic orchestration.

**Package:** `pkg/reactor/` (add to existing package)

**Files to modify/create:**
- `pkg/reactor/coordinators.go` — add `LLMCoordinator`
- `pkg/reactor/coordinators_test.go` — tests

#### 6.1.1 LLMCoordinator

```go
// LLMCoordinator uses an LLM to decide which team member(s) should act next.
// On each step it presents the shared conversation and team roster to the LLM,
// which returns a structured decision.
type LLMCoordinator struct {
    completer  modeladapter.Completer
    maxRounds  int
    step       int
}

func NewLLMCoordinator(completer modeladapter.Completer, maxRounds int) *LLMCoordinator {
    return &LLMCoordinator{
        completer: completer,
        maxRounds: maxRounds,
    }
}
```

**How it works:**

1. Build a system prompt describing the coordinator's job:
   ```
   You are a team coordinator. Given the conversation so far and the available
   team members, decide which member(s) should act next.

   Team members:
   - [0] "researcher" (role: research) — researches topics using tools
   - [1] "writer" (role: write) — writes content based on research
   - [2] "critic" (role: review) — reviews and provides feedback

   Respond with a JSON object:
   {"members": [0, 1], "done": false, "reasoning": "..."}

   Set "done": true when the task is complete.
   ```

2. The coordinator's own chat accumulates the conversation summary and decisions.

3. On each `Next()` call:
   - Append recent shared chat messages as context
   - Call `completer.Complete()` to get the LLM's decision
   - Parse the structured JSON response
   - Return `Selection{Members: [...], Done: ...}`

4. If the LLM returns invalid JSON or out-of-range indices, retry once with an error correction prompt. On second failure, return an error.

**Why this design:**
- Uses the existing `Coordinator` interface — no changes to `Reactor`
- The coordinator has its own private chat (not exposed to team members)
- The LLM sees a high-level summary, not raw tool calls from agents
- `maxRounds` provides a safety limit

#### 6.1.2 Tool-Equipped LLM Coordinator (Stretch)

An advanced variant where the coordinator itself has tools:
- `get_team_status` — returns each member's last message and role
- `get_shared_summary` — returns a summary of the shared chat
- `assign_task` — returns a Selection with specific members and injects a task description into the shared chat

This makes the coordinator a ReAct agent itself, but that's a future enhancement.

---

### Phase 2: Delegation (Agent-as-Tool)

**Goal:** Allow any agent to call another agent as a tool during its ReAct loop. The called agent runs its own ReAct loop and returns a result.

**Package:** `pkg/agents/delegate/`

**Files to create:**
- `pkg/agents/delegate/delegate.go`
- `pkg/agents/delegate/delegate_test.go`
- `pkg/agents/delegate/README.md`

#### 6.2.1 AgentTool

```go
// AgentTool wraps a NamedAgent as a toolbox.Tool. When called, it injects
// the tool call arguments as a user message into the agent's chat, runs the
// agent, and returns the final text reply as the tool result.
type AgentTool struct {
    Agent       reactor.NamedAgent
    Description string
    InputSchema json.RawMessage // JSON Schema for the "task" input
}

// Tool returns a toolbox.Tool that delegates to the wrapped agent.
func (at *AgentTool) Tool() toolbox.Tool {
    return toolbox.Tool{
        Name:        at.Agent.AgentName(),
        Description: at.Description,
        InputSchema: at.InputSchema,
        Handler:     at.handle,
    }
}

func (at *AgentTool) handle(ctx context.Context, input json.RawMessage) (string, error) {
    // 1. Parse input to extract the task/query
    // 2. Append it as a user message to the agent's private chat
    // 3. Run the agent: reply, err := at.Agent.Run(ctx)
    // 4. Return reply.TextContent() as the tool result
}
```

**Usage example:**

```go
// Create a researcher agent
researcher := react.New(
    agents.NewAgentBase("researcher", llm, chat.New(), searchTools),
    react.Options{MaxIterations: 5},
)

// Wrap it as a tool
researchTool := delegate.AgentTool{
    Agent:       researcher,
    Description: "Delegates a research task to a specialist researcher agent",
    InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "task": {"type": "string", "description": "The research task to perform"}
        },
        "required": ["task"]
    }`),
}

// Give the tool to an orchestrator agent
orchestratorTB := toolbox.New()
orchestratorTB.Register(researchTool.Tool())

orchestrator := react.New(
    agents.NewAgentBase("orchestrator", llm, mainChat, orchestratorTB),
    react.Options{MaxIterations: 10},
)

reply, err := orchestrator.Run(ctx)
```

**Key design decisions:**
- Each delegation call creates a fresh user message in the sub-agent's chat, preserving the sub-agent's full conversation history across multiple calls
- The sub-agent's private reasoning (tool calls, intermediate steps) stays in its own chat
- Only the final text reply surfaces as the tool result
- Context cancellation propagates: if the parent is cancelled, the sub-agent stops too
- The sub-agent can itself have AgentTools, enabling recursive delegation (with depth limits via `MaxIterations`)

#### 6.2.2 AgentToolFactory (Convenience)

```go
// NewAgentTool creates an AgentTool with a default input schema (single "task" string field).
func NewAgentTool(agent reactor.NamedAgent, description string) toolbox.Tool {
    return AgentTool{
        Agent:       agent,
        Description: description,
        InputSchema: defaultTaskSchema,
    }.Tool()
}
```

---

### Phase 3: Handoff Pattern

**Goal:** Allow an agent to transfer control of the conversation to another agent mid-run, similar to OpenAI Swarm's handoff pattern.

**Package:** `pkg/agents/handoff/`

**Files to create:**
- `pkg/agents/handoff/handoff.go`
- `pkg/agents/handoff/handoff_test.go`
- `pkg/agents/handoff/README.md`

#### 6.3.1 Design

A `HandoffAgent` manages a set of agents and a shared chat. One agent is "active" at a time. The active agent runs its ReAct loop. If it calls a special `transfer_to_<name>` tool, control transfers to the named agent.

```go
type HandoffAgent struct {
    name    string
    members map[string]reactor.NamedAgent
    active  string        // name of currently active agent
    chat    *chat.Chat    // shared conversation (all agents share this one)
}

func New(name string, c *chat.Chat, initial string, members ...reactor.NamedAgent) *HandoffAgent {
    // Register each member, set initial as active
}

// Run executes the handoff loop. The active agent runs until it either:
// - Returns a final answer (no tool calls) → HandoffAgent returns it
// - Calls transfer_to_<name> → switch active agent, continue loop
func (h *HandoffAgent) Run(ctx context.Context) (message.Message, error) {
    for {
        agent := h.members[h.active]
        reply, err := agent.Run(ctx)
        if err != nil {
            // Check if it's a handoff signal (via a special error or metadata)
            // If so, switch active agent and continue
            // Otherwise, return the error
        }
        // Check reply for handoff signal
        // If no handoff, return reply as final answer
    }
}
```

**Handoff mechanism options:**

**Option A: Transfer tools injected automatically**
- When creating a HandoffAgent, automatically register `transfer_to_<name>` tools in each member's ToolBox
- The tool handler returns a special sentinel error or sets metadata on the reply
- The HandoffAgent's `Run` loop detects this and switches

**Option B: Handoff via message metadata**
- The agent sets `msg.SetMeta("handoff_to", "agent-name")` on its reply
- The HandoffAgent checks metadata after each agent.Run()

**Recommendation: Option A** — it's more natural for the LLM (tools are part of its schema) and doesn't require the LLM to know about metadata conventions.

#### 6.3.2 Transfer Tool Implementation

```go
// handoffTool creates a transfer tool for a specific target agent.
func handoffTool(targetName string, targetDescription string) toolbox.Tool {
    return toolbox.Tool{
        Name:        "transfer_to_" + targetName,
        Description: fmt.Sprintf("Transfer the conversation to %s. %s", targetName, targetDescription),
        InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
        Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
            return "", &HandoffError{Target: targetName}
        },
    }
}

// HandoffError signals that a handoff should occur.
type HandoffError struct {
    Target string
}

func (e *HandoffError) Error() string {
    return fmt.Sprintf("handoff to %s", e.Target)
}
```

**Key difference from Reactor:** In a Reactor, a coordinator decides who runs. In Handoff, the agents themselves decide by calling transfer tools. This is more dynamic and allows the LLM to reason about when to transfer.

**Key difference from Delegation:** In Delegation, Agent A calls Agent B and gets a result back (Agent A stays in control). In Handoff, Agent A gives up control entirely — Agent B takes over the conversation.

---

### Phase 4: Middleware & Guardrails

**Goal:** Composable hooks that wrap agent execution to add cross-cutting concerns (logging, validation, rate limiting, output formatting).

**Package:** `pkg/agents/middleware/`

**Files to create:**
- `pkg/agents/middleware/middleware.go`
- `pkg/agents/middleware/middleware_test.go`
- `pkg/agents/middleware/README.md`

#### 6.4.1 Design

```go
// Middleware wraps an Agent's Run method.
type Middleware func(next agents.Agent) agents.Agent

// Chain composes multiple middleware into a single middleware.
// Middleware are applied in order: first middleware is outermost.
func Chain(mws ...Middleware) Middleware {
    return func(agent agents.Agent) agents.Agent {
        for i := len(mws) - 1; i >= 0; i-- {
            agent = mws[i](agent)
        }
        return agent
    }
}

// Apply wraps an agent with the given middleware chain.
func Apply(agent agents.Agent, mws ...Middleware) agents.Agent {
    return Chain(mws...)(agent)
}
```

#### 6.4.2 Built-in Middleware

**MaxTokens** — Limits total token usage per agent run:
```go
func MaxTokens(limit int, tracker *usage.Tracker) Middleware
```

**Timeout** — Adds a deadline to the agent's context:
```go
func Timeout(d time.Duration) Middleware
```

**Recovery** — Catches panics and converts to errors:
```go
func Recovery() Middleware
```

**Logger** — Logs agent start/stop/error with timing:
```go
func Logger(log *slog.Logger) Middleware
```

**OutputGuardrail** — Validates/transforms the agent's final output:
```go
func OutputGuardrail(check func(message.Message) error) Middleware
```

**Usage example:**
```go
agent := react.New(base, opts)

guarded := middleware.Apply(agent,
    middleware.Logger(slog.Default()),
    middleware.Timeout(30 * time.Second),
    middleware.Recovery(),
    middleware.OutputGuardrail(func(msg message.Message) error {
        if strings.Contains(msg.TextContent(), "UNSAFE") {
            return errors.New("output contains unsafe content")
        }
        return nil
    }),
)

reply, err := guarded.Run(ctx)
```

---

### Phase 5: Shared State Store

**Goal:** A thread-safe key-value store that agents can read/write structured data to, enabling coordination beyond natural language chat.

**Package:** `pkg/state/`

**Files to create:**
- `pkg/state/store.go`
- `pkg/state/store_test.go`
- `pkg/state/README.md`

#### 6.5.1 Design

```go
// Store is a thread-safe key-value store for inter-agent state.
// Keys are strings, values are any type. The zero value is ready to use.
type Store struct {
    mu   sync.RWMutex
    data map[string]any
}

func New() *Store { ... }

// Get returns a value by key.
func (s *Store) Get(key string) (any, bool) { ... }

// Set stores a value.
func (s *Store) Set(key string, value any) { ... }

// Delete removes a key.
func (s *Store) Delete(key string) { ... }

// Keys returns all keys.
func (s *Store) Keys() []string { ... }

// Snapshot returns a shallow copy of the entire store.
func (s *Store) Snapshot() map[string]any { ... }

// Watch blocks until the given key is set or ctx is done.
// Returns the value and nil, or zero and ctx.Err().
func (s *Store) Watch(ctx context.Context, key string) (any, error) { ... }
```

#### 6.5.2 State as Tools

To make the state store usable by agents (via their ReAct loop), expose it as tools:

```go
// Tools returns a ToolBox with get/set/list tools backed by the store.
func (s *Store) Tools(namespace string) *toolbox.ToolBox {
    tb := toolbox.New()
    tb.Register(
        toolbox.Tool{
            Name:        namespace + "_state_get",
            Description: "Read a value from the shared state store",
            Handler:     s.getHandler,
            InputSchema: ...,
        },
        toolbox.Tool{
            Name:        namespace + "_state_set",
            Description: "Write a value to the shared state store",
            Handler:     s.setHandler,
            InputSchema: ...,
        },
        toolbox.Tool{
            Name:        namespace + "_state_list",
            Description: "List all keys in the shared state store",
            Handler:     s.listHandler,
            InputSchema: ...,
        },
    )
    return tb
}
```

**Usage example:**
```go
shared := state.New()
stateTools := shared.Tools("team")

researcher := react.New(
    agents.NewAgentBase("researcher", llm, chat.New(), searchTools, stateTools),
    react.Options{},
)

writer := react.New(
    agents.NewAgentBase("writer", llm, chat.New(), stateTools),
    react.Options{},
)

// Researcher writes: shared.Set("findings", "...")
// Writer reads: shared.Get("findings") → "..."
```

This is the **blackboard pattern** from multi-agent research — agents coordinate through a shared data structure rather than direct messaging.

---

### Phase 6: Integration & CLI

**Goal:** Wire everything together in `cmd/shelly/` to demonstrate the full system working end-to-end.

**Files to modify:**
- `cmd/shelly/shelly.go` — main entry point

**What to build:**
1. A CLI that accepts a task description
2. Creates a team of agents (configurable via flags or config file)
3. Runs the team using the appropriate coordination pattern
4. Streams output to the terminal

**This phase is detailed last because it depends on all previous phases. The specific CLI design should be driven by the capabilities implemented in Phases 0-5.**

---

## 7. Open Questions

### 7.1 Agent Chat Reset Between Delegation Calls

When Agent A delegates to Agent B multiple times, should Agent B:
- **(a)** Keep its full conversation history (accumulating context)?
- **(b)** Start fresh each time (stateless delegation)?
- **(c)** Configurable per delegation?

**Recommendation:** (c) — default to (a) for conversational agents, (b) for stateless workers. Implement via an option on `AgentTool`.

### 7.2 Handoff with Shared vs. Separate Chats

Should agents in a handoff group share a single chat (seeing each other's full reasoning) or have private chats with sync (like Reactor)?

**Recommendation:** Shared single chat by default (simpler, agents can see context). Offer private-chat mode as an option for cases where agents shouldn't see each other's reasoning.

### 7.3 LLM Coordinator Context Management

As the shared chat grows, the LLM coordinator's context window fills up. Should the coordinator:
- **(a)** See the full shared chat?
- **(b)** See only the last N messages?
- **(c)** See a summary generated by another LLM call?

**Recommendation:** Start with (b) — a sliding window. Add (c) as an optimization later.

### 7.4 Provider Priority

Should we implement Anthropic first, OpenAI first, or both in parallel?

**Recommendation:** Anthropic first (Shelly's primary LLM target based on the project context), then OpenAI. Both use simple REST APIs and the `ModelAdapter` base handles the HTTP boilerplate.

### 7.5 Reactor Package Location

The `reactor` package currently lives at `pkg/reactor/`. With the addition of delegation and handoff patterns, should we:
- **(a)** Keep reactor separate, add delegate/handoff under `pkg/agents/`?
- **(b)** Move reactor under `pkg/agents/reactor/`?
- **(c)** Create a new `pkg/orchestration/` package?

**Recommendation:** (a) — keep reactor where it is. Delegation and handoff are agent-level concerns (they extend how a single agent behaves), while reactor is an orchestration concern (it manages multiple agents). The separation is clean.

---

## Summary

The current architecture is surprisingly well-suited for multi-agent coordination. The main gaps are:

1. **No LLM-powered decision-making in coordination** → Phase 1 (LLM Coordinator)
2. **No agent-to-agent delegation** → Phase 2 (AgentTool)
3. **No dynamic control transfer** → Phase 3 (Handoff)
4. **No lifecycle hooks** → Phase 4 (Middleware)
5. **No structured shared state** → Phase 5 (State Store)
6. **No concrete LLM provider** → Phase 0 (Anthropic/OpenAI adapters)

All of these build on the existing foundations (`Chat`, `Agent`, `Coordinator`, `ToolBox`) without requiring architectural rewrites. The approach is fully in-house with zero new external dependencies beyond what's already used (MCP SDK for tool interop, standard library for everything else).
