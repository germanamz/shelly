# agent

Package `agent` provides a unified agent type that runs a ReAct (Reason + Act) loop, supports dynamic delegation to other agents via a registry, and can learn procedures from skills.

## Overview

This package implements a single `Agent` type that:

- Runs a **ReAct loop**: iterates between LLM completion and tool execution until a final answer is produced or the iteration limit is reached.
- Can **discover and delegate** to other agents at runtime via a `Registry`.
- Learns **procedures from Skills** (folder-based definitions with step-by-step processes), split into inline skills (embedded in system prompt) and on-demand skills (loaded via `load_skill` tool).
- Supports **middleware** for cross-cutting concerns (timeout, recovery, logging, output guardrails).
- Supports **effects** -- pluggable, per-iteration hooks for dynamic behaviours (context compaction, tool result trimming, loop detection, failure reflection, progress tracking).
- Emits **fine-grained events** (`tool_call_start`, `tool_call_end`, `message_added`) via an optional `EventFunc` callback.
- Publishes **sub-agent lifecycle events** (`agent_start`, `agent_end`) via an optional `EventNotifier` callback.

## Exported Types and Interfaces

### Agent

The core type. Created via `New(name, description, instructions, completer, opts)`.

```go
type Agent struct { /* unexported fields */ }
```

**Constructor:**

- `New(name, description, instructions string, completer modeladapter.Completer, opts Options) *Agent`

**Methods:**

| Method | Description |
|--------|-------------|
| `Run(ctx) (message.Message, error)` | Executes the ReAct loop with middleware applied. |
| `Init()` | Builds and sets the system prompt. Called automatically by `Run()`, but can be called manually after `SetRegistry` and `AddToolBoxes`. Safe to call multiple times. |
| `SetRegistry(r *Registry)` | Enables dynamic delegation by setting the agent's registry. |
| `AddToolBoxes(tbs ...*toolbox.ToolBox)` | Adds user-provided toolboxes, deduplicating by pointer equality. |
| `Name() string` | Returns the agent's instance name (unique per spawned agent). |
| `ConfigName() string` | Returns the agent's config/template name (registry key). Equals `Name()` for session agents. |
| `Description() string` | Returns the agent's description. |
| `Prefix() string` | Returns the display prefix, defaulting to the robot emoji if unset. |
| `Chat() *chat.Chat` | Returns the agent's chat. |
| `Completer() modeladapter.Completer` | Returns the agent's completer. |
| `CompletionResult() *CompletionResult` | Returns structured completion data set by `task_complete`, or nil. |

### Options

Configures an Agent at construction time.

```go
type Options struct {
    MaxIterations          int           // ReAct loop limit (0 = unlimited).
    MaxDelegationDepth     int           // Max tree depth for delegation (0 = cannot delegate).
    Skills                 []skill.Skill // Procedures the agent knows.
    Middleware             []Middleware   // Applied around Run().
    Effects                []Effect      // Per-iteration hooks run inside the ReAct loop.
    Context                string        // Project context injected into the system prompt.
    EventNotifier          EventNotifier // Publishes sub-agent lifecycle events.
    Prefix                 string        // Display prefix (emoji + label) for the TUI.
    TaskBoard              TaskBoard     // Optional task board for automatic task lifecycle during delegation.
    ReflectionDir          string        // Directory for failure reflection notes (empty = disabled).
    DisableBehavioralHints bool          // When true, omits the <behavioral_constraints> section.
    EventFunc              EventFunc     // Optional callback for fine-grained loop events.
}
```

### CompletionResult

Carries structured completion data from a sub-agent. Set by the `task_complete` tool, read by delegation tools after `Run()` returns.

```go
type CompletionResult struct {
    Status        string   `json:"status"`                   // "completed" or "failed"
    Summary       string   `json:"summary"`                  // What was done or why it failed.
    FilesModified []string `json:"files_modified,omitempty"` // Files changed.
    TestsRun      []string `json:"tests_run,omitempty"`      // Tests executed.
    Caveats       string   `json:"caveats,omitempty"`        // Known limitations.
}
```

### TaskBoard

Interface for optional task lifecycle management during delegation. When set on `Options.TaskBoard`, the `delegate` tool automatically claims tasks and updates their status based on the child's `CompletionResult`.

```go
type TaskBoard interface {
    ClaimTask(id, agent string) error
    UpdateTaskStatus(id, status string) error
}
```

### EventNotifier

Called by orchestration tools to publish sub-agent lifecycle events.

```go
type EventNotifier func(ctx context.Context, kind string, agentName string, data any)
```

### EventFunc

Called by the agent to publish fine-grained loop events.

```go
type EventFunc func(ctx context.Context, kind string, data any)
```

### ToolCallEventData / MessageAddedEventData

Event payloads for `tool_call_start`/`tool_call_end` and `message_added` events respectively.

```go
type ToolCallEventData struct {
    ToolName string `json:"tool_name"`
    CallID   string `json:"call_id"`
}

type MessageAddedEventData struct {
    Role    string          `json:"role"`
    Message message.Message `json:"message"`
}
```

### AgentEventData

Metadata carried by `agent_start` and `agent_end` lifecycle events.

```go
type AgentEventData struct {
    Prefix string // Display prefix (e.g. robot emoji, pencil emoji).
    Parent string // Name of the parent agent (empty for top-level).
}
```

### Registry

Thread-safe directory of agent factories for dynamic discovery and delegation.

```go
type Registry struct { /* unexported fields */ }
```

| Method | Description |
|--------|-------------|
| `NewRegistry() *Registry` | Creates an empty Registry. |
| `Register(name, description string, factory Factory)` | Registers an agent factory. Replaces existing entries with the same name. |
| `Get(name string) (Factory, bool)` | Returns the factory for the named agent. |
| `List() []Entry` | Returns all entries sorted by name. |
| `Spawn(name string, depth int) (*Agent, bool)` | Creates a fresh agent instance with the given delegation depth. Sets `configName` to the registry key. |
| `NextID(configName string) int` | Returns a monotonically increasing counter for the given config name, used for unique instance name generation. |

### Factory

Creates a fresh Agent instance for delegation. Each call should return a new agent with a clean chat.

```go
type Factory func() *Agent
```

### Entry

Describes a registered agent in the directory.

```go
type Entry struct {
    Name        string
    Description string
}
```

### ErrMaxIterations

Sentinel error returned when the ReAct loop exceeds `MaxIterations` without producing a final answer.

```go
var ErrMaxIterations = errors.New("agent: max iterations reached")
```

## Effects System

Effects are pluggable, per-iteration hooks that run inside the ReAct loop at two phases: **before** the LLM call (`PhaseBeforeComplete`) and **after** the LLM reply (`PhaseAfterComplete`). They enable configuration-driven behaviours without modifying the core loop.

### Effect Interface

```go
type Effect interface {
    Eval(ctx context.Context, ic IterationContext) error
}
```

Effects receive an `IterationContext` containing the current phase, iteration number, chat, completer, and agent name. Returning an error aborts the loop. Effects run synchronously in registration order.

### IterationPhase

```go
const (
    PhaseBeforeComplete IterationPhase = iota // Runs before the LLM call.
    PhaseAfterComplete                        // Runs after the LLM reply, before tool dispatch.
)
```

### IterationContext

```go
type IterationContext struct {
    Phase     IterationPhase
    Iteration int
    Chat      *chat.Chat
    Completer modeladapter.Completer
    AgentName string
}
```

### Resetter

Optional interface that effects can implement to reset internal state between agent runs. Effects that track per-run state (e.g. injection guards, counters) should implement this. The agent calls `Reset()` on all effects that implement `Resetter` at the beginning of each `Run()`.

```go
type Resetter interface {
    Reset()
}
```

### EffectFunc

Adapter that lets ordinary functions implement `Effect`.

```go
type EffectFunc func(ctx context.Context, ic IterationContext) error
```

### Configuration

Add effects to `Options.Effects`:

```go
opts := agent.Options{
    Effects: []agent.Effect{
        effects.NewCompactEffect(effects.CompactConfig{
            ContextWindow: 200000,
            Threshold:     0.8,
        }),
    },
}
```

Or via YAML configuration through the engine:

```yaml
agents:
  - name: coder
    effects:
      - kind: compact
        params:
          threshold: 0.8
```

See `pkg/agent/effects/` for available implementations.

## Middleware System

Middleware wraps the agent's `Run` method, enabling cross-cutting concerns. Middleware is applied in the order listed in `Options.Middleware`, with the first middleware being the outermost wrapper.

### Runner / RunnerFunc

```go
type Runner interface {
    Run(ctx context.Context) (message.Message, error)
}

type RunnerFunc func(ctx context.Context) (message.Message, error)
```

### Middleware Type

```go
type Middleware func(next Runner) Runner
```

### Built-in Middleware

| Middleware | Description |
|-----------|-------------|
| `Timeout(d time.Duration)` | Wraps the runner's context with a deadline. |
| `Recovery()` | Catches panics and converts them to errors. |
| `Logger(log *slog.Logger, name string)` | Structured logging of start, finish, duration, and errors. |
| `OutputGuardrail(check func(message.Message) error)` | Validates the final message; returns the check error if validation fails. Skipped when the runner itself returns an error. |

## Built-in Orchestration Tools

When a `Registry` is set and `MaxDelegationDepth > 0`, two tools are automatically injected:

| Tool | Description |
|------|-------------|
| `list_agents` | Lists all available agents (excluding self). Case-insensitive self-exclusion. |
| `delegate` | Delegates one or more tasks to other agents. All tasks run concurrently. Each task requires `agent`, `task`, and `context` fields. Optional `task_id` per task for automatic task lifecycle. |

When depth > 0 (sub-agent), an additional tool is injected:

| Tool | Description |
|------|-------------|
| `task_complete` | Signals task completion with structured metadata (status, summary, files modified, tests run, caveats). Only callable once per agent run (duplicates are silently ignored). |

### Delegation Mechanics

The `delegate` tool:

1. Validates input (rejects self-delegation, enforces `MaxDelegationDepth`).
2. Spawns child agents from the registry with `depth + 1` and generates a unique instance name (`<configName>-<taskSlug>-<counter>`).
3. Propagates the parent's registry, `EventNotifier`, `EventFunc`, `ReflectionDir`, and `TaskBoard` to each child. Each child keeps its own `MaxDelegationDepth` from the factory.
4. Child uses only its own configured toolboxes (no inheritance from parent).
5. Prepends a `<delegation_context>` user message with the provided context.
6. Searches for relevant prior reflections and prepends them as `<prior_reflections>`.
7. Appends the task as a user message.
8. Runs all tasks concurrently and collects results.

Safety guards: self-delegation rejected (case-insensitive, compared against `configName`), `MaxDelegationDepth` enforced.

### Instance Names vs Config Names

Agents have two name concepts:

- **Config name** (`ConfigName()`): The template/kind name used for registry lookups and self-exclusion. For session agents, this equals the instance name. Set by `Spawn()` to the registry key.
- **Instance name** (`Name()`): A unique identifier per agent instance. For session agents, this equals the config name. For spawned sub-agents, it is generated as `<configName>-<taskSlug>-<counter>` (e.g., `coder-refactor-1`, `coder-parsing-2`).

This separation allows multiple children of the same type to be spawned in parallel while remaining distinguishable in events, task boards, and logs. The config name is used for registry lookups, self-exclusion in `list_agents`/`delegate`, and system prompt filtering. The instance name is used for identity, context propagation, event notifications, and task board claims.

### Automatic Task Lifecycle

When `Options.TaskBoard` is set, the `delegate` tool supports an optional `task_id` parameter per task:

1. **Before `child.Run()`**: `TaskBoard.ClaimTask(taskID, childName)` is called. Claim errors cause the task to fail immediately.
2. **After `child.Run()`**: if the child produced a `CompletionResult`, `TaskBoard.UpdateTaskStatus(taskID, cr.Status)` is called automatically. If the child exhausts iterations, a synthetic `CompletionResult` with status "failed" is created and the task is updated accordingly.
3. **On child error**: if `child.Run()` returns a non-`ErrMaxIterations` error (e.g. context cancellation, completer failure), the task is rolled back to `"failed"` so it doesn't stay stuck in `in_progress`.
4. **No completion fallback**: if the child finishes without error but never called `task_complete`, the task is automatically updated to `"completed"` since the child ran to natural conclusion.

### Structured Completion Protocol

Sub-agents (depth > 0) receive a `task_complete` tool and a `<completion_protocol>` section in their system prompt instructing them to always call it. When called, the ReAct loop stops immediately. The result is stored on the agent and included in the `delegate` tool's response. If the sub-agent exhausts its iteration limit without calling `task_complete`, a synthetic failed `CompletionResult` is generated and a reflection note is written.

Duplicate `task_complete` calls are safely ignored via `sync.Once`.

### Failure Reflections

When `Options.ReflectionDir` is set, the `delegate` tool writes markdown reflection notes when sub-agents fail. Before delegating, it searches existing reflections for keyword matches against the task description and prepends relevant ones as `<prior_reflections>`. This enables agents to learn from past failures.

Reflections are capped at 5 files and 32KB total to avoid excessive context.

### Notes Protocol

When an agent has notes tools (detected by the presence of `list_notes` in its toolboxes), the system prompt includes a `<notes_protocol>` section informing the agent about the shared notes system for durable cross-agent communication. The protocol does not preload any notes content.

### Sub-Agent Event Notifications

When `Options.EventNotifier` is set, the `delegate` tool publishes lifecycle events:

- `agent_start` -- emitted before `child.Run(ctx)` with `AgentEventData{Prefix, Parent}`.
- `agent_end` -- emitted after `child.Run(ctx)` completes (success or error).

The notifier is automatically propagated to children so nested delegation chains publish events at every level.

### Toolbox Isolation

Children use only their own configured toolboxes. Parent toolboxes are **not** inherited during delegation. This enforces least-privilege: a child configured with `[filesystem, search]` only has access to those tools, regardless of what the parent has.

### Display Prefix

`Options.Prefix` sets a configurable emoji/label for the agent. It defaults to a robot emoji when empty. Frontends read the prefix via `Agent.Prefix()` or from `AgentEventData` in lifecycle events.

## System Prompt Structure

The system prompt is built by `buildSystemPrompt()` using XML tags for clear section boundaries. Sections are ordered for prompt-cache friendliness (static content first, dynamic content last):

1. `<identity>` -- Agent name and description (static, cacheable prefix)
2. `<completion_protocol>` -- Sub-agent completion instructions (static, depth > 0 only)
3. `<notes_protocol>` -- Cross-agent notes awareness (static, only when notes tools are present)
4. `<instructions>` -- Agent-specific instructions (static)
5. `<behavioral_constraints>` -- Heuristic behavioural hints (static, can be disabled via `DisableBehavioralHints`)
6. `<project_context>` -- Project context loaded at startup (semi-static)
7. `<skills>` -- Inline skill content, skills without a description (semi-static)
8. `<available_skills>` -- On-demand skill descriptions with `load_skill` instruction, skills with a description (semi-static)
9. `<available_agents>` -- Agent directory from registry, excluding self (dynamic, last)

## ReAct Loop Details

The internal `run()` method:

1. Sets the agent name in the context via `agentctx.WithAgentName`.
2. Calls `Init()` to ensure the system prompt exists.
3. Collects all toolboxes (user + orchestration + completion) and deduplicates tool declarations by name.
4. Resets all effects that implement `Resetter`.
5. Enters the iteration loop (bounded by `MaxIterations` or unlimited if 0).
6. Each iteration:
   - Evaluates effects at `PhaseBeforeComplete`.
   - Calls `completer.Complete()` with the chat and tools.
   - Appends the reply to the chat, emits `message_added` event.
   - Evaluates effects at `PhaseAfterComplete`.
   - If no tool calls in the reply, returns the reply as the final answer.
   - Executes all tool calls concurrently using `sync.WaitGroup.Go()`, collecting results in order.
   - Appends tool results to the chat, emits `message_added` events.
   - If `completionResult` is set (from `task_complete`), returns immediately.
7. If the loop exhausts iterations, returns `ErrMaxIterations`.

## File Structure

```
agent.go        -- Agent struct, New(), Run(), Init(), system prompt building, event emission
effect.go       -- Effect interface, EffectFunc, Resetter, IterationPhase, IterationContext
effects/        -- Reusable Effect implementations (see pkg/agent/effects/README.md)
registry.go     -- Registry, Factory, Entry for dynamic agent discovery and spawning
tools.go        -- Built-in orchestration tools (list_agents, delegate, task_complete),
                   AgentEventData, delegation context, reflection helpers
middleware.go   -- Runner interface, Middleware type, built-in middleware
                   (Timeout, Recovery, Logger, OutputGuardrail)
```

## Dependencies

- `pkg/agentctx/` -- shared context key helpers (agent name propagation)
- `pkg/chats/` -- chat, message, content, role types
- `pkg/modeladapter/` -- `Completer` interface, `UsageReporter` (used by effects)
- `pkg/tools/toolbox/` -- `ToolBox`, `Tool` types
- `pkg/skill/` -- `Skill` type for procedure loading

## Usage

```go
// Simple agent with tools.
a := agent.New("assistant", "Helpful bot", "Be helpful.", completer, agent.Options{
    MaxIterations: 20,
})
a.AddToolBoxes(myTools)
reply, err := a.Run(ctx)

// Agent with delegation, middleware, and effects.
reg := agent.NewRegistry()
reg.Register("researcher", "Finds information", researcherFactory)
reg.Register("coder", "Writes code", coderFactory)

orch := agent.New("orchestrator", "Coordinates work", "Break tasks into subtasks.", completer, agent.Options{
    MaxIterations:      100,
    MaxDelegationDepth: 3,
    Skills:             skills,
    Middleware:         []agent.Middleware{agent.Recovery(), agent.Logger(logger, "orchestrator")},
    Effects:            []agent.Effect{compactEffect, trimEffect},
    EventNotifier: func(ctx context.Context, kind, name string, data any) {
        fmt.Printf("sub-agent event: %s %s\n", kind, name)
    },
})
orch.SetRegistry(reg)
reply, err := orch.Run(ctx)
```
