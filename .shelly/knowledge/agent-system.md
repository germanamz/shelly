# Agent System

The `pkg/agent/` package implements a unified agent type running a ReAct (Reason + Act) loop with dynamic delegation, middleware, an effects system for conversation management, and interactive parent↔child communication.

## Core Types

### Agent Struct

```go
type Agent struct {
    name, configName   string                // instance name + template/kind name
    description        string
    instructions       string
    chat               *chat.Chat
    completer          modeladapter.Completer
    toolbox            *toolbox.ToolBox
    effects            []Effect
    middleware         []Middleware
    skills             []skill.Skill
    delegation         delegationConfig       // maxDepth, maxHandoffs, taskBoard, reflectionDir, questionTimeout
    interaction        *InteractionChannel    // parent↔child communication
    handoff            handoffHandler         // tracks handoff calls
    interactiveDelegations *DelegationRegistry // tracks active interactive children
    inbox              chan message.Message    // receives routed user messages
}
```

Key fields in `Options`: `MaxIterations`, `WarnIterations`, `MaxDelegationDepth`, `MaxHandoffs`, `Skills []skill.Skill`, `Middleware []Middleware`, `Effects []Effect`, `Context string`, `EventNotifier`, `EventFunc`, `TaskBoard`, `ReflectionDir`, `InteractionMode` (""|"auto"|"interactive"|"blocking"), `QuestionTimeout`, `ProviderLabel`, `Prefix`, `DisableBehavioralHints`, `InboxRegistrar`, `InboxUnregistrar`.

### Error Sentinels

```go
var ErrMaxIterations     = errors.New("agent: max iterations reached")
var ErrTokenBudgetExhausted = errors.New("agent: token budget exhausted")
var ErrTimeBudgetExhausted  = errors.New("agent: time budget exhausted")
```

### EventNotifier

```go
type EventNotifier func(ctx context.Context, kind string, agentName string, data any)
```

Published event kinds: `"agent_started"`, `"agent_completed"`, `"agent_failed"`, `"delegation_started"`, `"delegation_completed"`, `"delegation_failed"`.

### InboxRegistrar / InboxUnregistrar

```go
type InboxRegistrar func(name string, inbox chan message.Message)
type InboxUnregistrar func(name string)
```

Used to register and unregister inbox channels for child agents, enabling user message routing to specific named agents. The parent wires these during delegation so that external user messages can be forwarded to the appropriate child.

### TaskBoard Interface

```go
type TaskBoard interface {
    Create(ctx context.Context, task tasks.Task) (tasks.Task, error)
    Get(ctx context.Context, id string) (tasks.Task, error)
    Update(ctx context.Context, id string, updates map[string]any) (tasks.Task, error)
}
```

### TaskCancelWatcher Interface

```go
type TaskCancelWatcher interface {
    WatchCancel(ctx context.Context, taskID string) <-chan struct{}
}
```

Optional interface that TaskBoard implementations can provide to support cancellation propagation. When a task is canceled on the board, the returned channel is closed, allowing the delegation handler to cancel the child agent's context.

---

## ReAct Loop (`agent.go` — `Run()`)

The `Run()` method drives the agent's main loop:

1. **Reset effects** — calls `Resetter.Reset()` on each effect that implements it
2. **Build system prompt** — assembles `<identity>`, `<instructions>`, `<project_context>`, `<behavioral_constraints>` (unless `DisableBehavioralHints`), `<available_skills>`, `<available_agents>`, tool formatting hints
3. **Iteration loop** (up to `MaxIterations`):
   a. Estimate token count via `TokenEstimator` (if completer implements it)
   b. **Pre-complete effects** — run `PhaseBeforeComplete` effects in order
   c. **Filter tools** — apply `ToolFilter` effects to remove/restrict tools
   d. **Collect provided tools** — gather tools from `ToolProvider` effects
   e. **LLM completion** — call `completer.Complete(ctx, chat, tools)`
   f. **Post-complete effects** — run `PhaseAfterComplete` effects
   g. **Tool dispatch** — if message has tool calls, execute them sequentially; if no tool calls, loop ends
   h. **Check inbox** — non-blocking check for user messages in the inbox channel; appends them to chat
4. **Return** final assistant message or error

### Token Estimation

If the completer implements `modeladapter.TokenEstimator`, the agent estimates tokens before each LLM call and passes the count to effects via `IterationContext.EstimatedTokens`.

---

## Delegation System (`delegation.go`)

### Agent Registry

```go
type RegistryFactory func(opts Options) *Agent
```

The agent builds `delegate` and `handoff` tools from its `Options.Registry` map. Delegation uses a factory pattern: each registered agent kind has a factory that creates fresh agent instances.

### Delegation Modes

Three delegation modes controlled by `InteractionMode`:

1. **Blocking** (default / `"blocking"`): Parent spawns child, waits for completion, returns result as tool output.
2. **Auto** (`"auto"`): Uses blocking for single delegations. For concurrent delegations, uses blocking with parallel execution.
3. **Interactive** (`"interactive"`): Child runs asynchronously. If child calls `request_input`, parent receives a `PendingQuestion` and can answer via `answer_delegation_questions` tool.

### Delegation Flow

1. Create child agent via registry factory with depth-decremented options
2. Wire `InteractionChannel` for parent↔child communication
3. Register inbox channel via `InboxRegistrar` (if provided)
4. Create/update task on `TaskBoard` (if provided)
5. Set up `TaskCancelWatcher` (if TaskBoard implements it)
6. Run child agent (`child.Run(ctx)`)
7. Collect `CompletionResult` and return to parent
8. Unregister inbox via `InboxUnregistrar` on completion

### AgentEventData

```go
type AgentEventData struct {
    Agent       string
    Task        string
    Prefix      string
    DelegateOf  string
    Provider    string
}
```

Published with `delegation_started`/`delegation_completed`/`delegation_failed` events.

### Delegation Streaming (`delegation_stream.go`)

Real-time streaming of delegation progress to parent:

```go
type DelegationEventKind string // "status" | "progress"

type DelegationEvent struct {
    Kind      DelegationEventKind
    AgentName string
    Status    string          // for status events
    Text      string          // for progress events
    TextDelta string          // incremental text for progress
}

type DelegationStreamCallback func(event DelegationEvent)
```

### Handoffs

Peer-to-peer agent handoff via the `handoff` tool. Limited by `MaxHandoffs`. The `HandoffResult` carries the target agent name, reason, and context.

```go
type HandoffResult struct {
    Agent   string `json:"agent"`
    Reason  string `json:"reason"`
    Context string `json:"context"`
}
```

### DelegationRegistry (`interaction_registry.go`)

Tracks active interactive delegations for a parent agent:

```go
type PendingDelegation struct {
    ID         string
    Agent      string
    Task       string
    QuestionCh chan PendingQuestion  // per-delegation question intake
    AnswerCh   chan string           // route answers back
    DoneCh     <-chan delegateResult // final result
    Cancel     context.CancelFunc
}
```

Provides `Register()`, `Get()`, `Remove()`, `Close()` methods. The `answer_delegation_questions` tool batches answers to multiple children and waits for follow-up questions or completion.

---

## InteractionChannel

Bidirectional parent↔child communication:

```go
type InteractionChannel struct {
    QuestionCh chan<- PendingQuestion // child sends questions
    AnswerCh   <-chan string          // child receives answers
}

type Question struct {
    Question string   `json:"question"`
    Options  []string `json:"options,omitempty"`
}
```

- **Interactive mode**: Questions are routed to parent via `DelegationRegistry`
- **Blocking mode**: The `request_input` tool auto-answers from delegation context
- **Timeout**: Configurable via `QuestionTimeout` option

---

## Completion (`completion.go`)

### CompletionResult

```go
type CompletionResult struct {
    Status        string   `json:"status"`        // "completed" or "failed"
    Summary       string   `json:"summary"`
    KeyDecisions  []string `json:"key_decisions"`
    FilesModified []string `json:"files_modified"`
    ErrorMessage  string   `json:"error_message,omitempty"`
}
```

Set by the `task_complete` tool, read by delegation tools after `Run()` returns. The `task_complete` tool is injected into every delegated agent.

---

## Effect System (`effect.go`)

### Core Interfaces

```go
type IterationPhase int
const (
    PhaseBeforeComplete IterationPhase = iota  // before LLM call
    PhaseAfterComplete                         // after LLM reply, before tool dispatch
)

type IterationContext struct {
    Phase           IterationPhase
    Iteration       int
    Chat            *chat.Chat
    Completer       modeladapter.Completer
    AgentName       string
    EstimatedTokens int  // pre-call estimate (0 = not computed)
    ToolTokens      int  // token cost of tool definitions (0 = not computed)
}

type Effect interface {
    Eval(ctx context.Context, ic IterationContext) error
}

type Resetter interface { Reset() }

type ToolFilter interface {
    FilterTools(ctx context.Context, ic IterationContext, tools []toolbox.Tool) []toolbox.Tool
}

type ToolProvider interface {
    ProvidedTools() *toolbox.ToolBox
}

type EffectFunc func(ctx context.Context, ic IterationContext) error
```

### Built-in Effects (`effects/` subpackage)

The `effects/` subpackage provides the concrete implementations. The agent package defines the interfaces; effects never import agent directly.

| Effect | Phase | Purpose |
|--------|-------|---------|
| **Compact** | BeforeComplete | Summarizes old messages when context exceeds threshold. Replaces them with a compact summary. |
| **LoopDetect** | AfterComplete | Detects repetitive tool-call patterns via fingerprinting. Injects a hint to break the loop. |
| **ObservationMask** | BeforeComplete | Truncates long tool results in older messages to reduce token usage while preserving recent messages. |
| **Offload** | BeforeComplete | Writes large tool results to disk and replaces them with file references when context gets large. |
| **TrimToolResults** | BeforeComplete | Trims tool results exceeding a character limit, keeping head+tail with a truncation marker. |
| **Progress** | AfterComplete | Injects periodic progress prompts (every N iterations) reminding agent to assess progress and use notes. |
| **Reflection** | AfterComplete | After consecutive tool failures (≥ threshold), injects a reflection prompt asking the agent to reassess. |
| **SlidingWindow** | BeforeComplete | Token-aware context management: summarizes old messages in a "far zone" while preserving recent messages in a "recent zone". |
| **StallDetect** | AfterComplete | Detects stalled agents via message fingerprinting (hash-based similarity). Injects hints to change approach. |
| **TimeBudget** | AfterComplete | Enforces a maximum cumulative LLM inference time. Warns at threshold, terminates with `ErrTimeBudgetExhausted`. |
| **TokenBudget** | BeforeComplete | Enforces a maximum token budget. Warns at threshold, terminates with `ErrTokenBudgetExhausted`. |
| **ToolScope** | (ToolFilter) | Filters which tools the LLM sees by excluding named tools (blacklist). |
| **Render** | (helper) | Utility for rendering message transcripts in compact text form (used by other effects). |
| **Threshold** | (helper) | Shared utility for checking if estimated tokens exceed a context window threshold. |

---

## Middleware System (`middleware.go`)

```go
type Runner interface {
    Run(ctx context.Context) (message.Message, error)
}

type Middleware func(next Runner) Runner
```

Middleware wraps the agent's `Run()` call. Applied in registration order (outermost first). Built-in middleware includes:

- **TimeoutMiddleware**: Enforces a maximum wall-clock duration
- **LoggingMiddleware**: Logs agent start/complete/error with slog

---

## System Prompt Assembly

The system prompt is built from sections (in order):
1. `<identity>` — Agent name and description
2. `<instructions>` — Agent instructions text
3. `<project_context>` — Project context from `Options.Context`
4. `<behavioral_constraints>` — Static behavioral hints (omitted if `DisableBehavioralHints` is true)
5. `<available_skills>` — Loaded skills summary
6. `<available_agents>` — Registry agent descriptions for delegation
7. Tool formatting hints — JSON examples for tool use

---

## File Layout

```
pkg/agent/
├── agent.go                  # Agent struct, Options, Run() loop, inbox handling
├── completion.go             # CompletionResult, task_complete tool
├── delegation.go             # Delegation tools, registry, spawn logic
├── delegation_stream.go      # DelegationEvent streaming types
├── effect.go                 # Effect/Resetter/ToolFilter/ToolProvider interfaces
├── interaction_registry.go   # DelegationRegistry, interactive Q&A flow
├── middleware.go              # Runner/Middleware types, TimeoutMiddleware, LoggingMiddleware
├── effects/                   # Concrete effect implementations
│   ├── compact.go             # Context compaction via summarization
│   ├── loopdetect.go          # Repetitive tool-call pattern detection
│   ├── observation_mask.go    # Old tool result truncation
│   ├── offload.go             # Large result offloading to disk
│   ├── progress.go            # Periodic progress prompts
│   ├── reflection.go          # Failure-triggered reflection prompts
│   ├── render.go              # Message transcript rendering utility
│   ├── sliding_window.go      # Token-aware sliding window context management
│   ├── stall_detect.go        # Stall detection via fingerprinting
│   ├── threshold.go           # Token threshold checking utility
│   ├── time_budget.go         # Time budget enforcement
│   ├── token_budget.go        # Token budget enforcement
│   ├── tool_scope.go          # Tool filtering by name
│   └── trim_tool_results.go   # Tool result size trimming
└── *_test.go                  # Comprehensive test files
```
