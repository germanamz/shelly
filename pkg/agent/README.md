# agent

Package `agent` provides a unified agent type that runs a ReAct (Reason + Act) loop, supports dynamic delegation to other agents via a registry, and can learn procedures from skills.

## Overview

This package replaces the previous `pkg/agents/` and `pkg/reactor/` hierarchies with a single `Agent` type. An agent:

- Runs a **ReAct loop**: iterates between LLM completion and tool execution until a final answer is produced.
- Can **discover and delegate** to other agents at runtime via a `Registry`.
- Learns **procedures from Skills** (folder-based definitions with step-by-step processes).
- Supports **middleware** for cross-cutting concerns (timeout, recovery, logging, guardrails).
- Supports **effects** — pluggable, per-iteration hooks for dynamic behaviours (context compaction, cost limits, guardrails).

## Types

### Agent

The core type. Created via `New(name, description, instructions, completer, opts)`.

- `Run(ctx) (message.Message, error)` — executes the ReAct loop with middleware.
- `SetRegistry(r)` — enables dynamic delegation.
- `AddToolBoxes(tbs...)` — adds user-provided tool registries.
- `Name()`, `Description()`, `Chat()` — accessors.

### Registry

Thread-safe directory of agent factories for dynamic discovery and delegation.

- `Register(name, description, factory)` — registers an agent factory.
- `List() []Entry` — returns all entries sorted by name.
- `Spawn(name, depth) (*Agent, bool)` — creates a fresh agent instance.

### Middleware

Composable wrappers around the agent's `Run` method.

- `Timeout(d)` — context deadline.
- `Recovery()` — panic-to-error conversion.
- `Logger(log, name)` — structured logging of start/finish/errors.
- `OutputGuardrail(check)` — validates the final message.

## Built-in Orchestration Tools

When a `Registry` is set, three tools are automatically injected:

| Tool | Description |
|------|-------------|
| `list_agents` | Lists all available agents (excluding self) |
| `delegate_to_agent` | Delegates a task to another agent, returns its response |
| `spawn_agents` | Runs multiple agents concurrently, returns collected results |

Safety guards: self-delegation rejected, `MaxDelegationDepth` enforced, concurrent spawn uses cancel-on-first-error.

## Effects System

Effects are pluggable, per-iteration hooks that run inside the ReAct loop at two phases: **before** the LLM call (`PhaseBeforeComplete`) and **after** the LLM reply (`PhaseAfterComplete`). They enable configuration-driven behaviours without modifying the core loop.

### Interface

```go
type Effect interface {
    Eval(ctx context.Context, ic IterationContext) error
}
```

Effects receive an `IterationContext` containing the current phase, iteration number, chat, completer, and agent name. Returning an error aborts the loop. Effects run synchronously in registration order.

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

### Built-in Effects

See `pkg/agent/effects/` for available implementations:

| Effect | Kind | Description |
|--------|------|-------------|
| `CompactEffect` | `compact` | Summarises the conversation when token usage approaches the context window limit |

### Toolbox Inheritance

When an agent delegates to or spawns a child agent, the child receives a **union** of its own configured toolboxes and the parent's toolboxes. The sequence is:

1. The child's factory is called, producing a fresh agent with only its config-defined toolboxes.
2. The parent calls `child.AddToolBoxes(a.toolboxes...)`, appending all of the parent's toolboxes.
3. The child's registry is set to the parent's registry, enabling further delegation.

Since `AddToolBoxes` appends parent toolboxes **after** the child's own, and `callTool` uses first-match, the child's config-defined tools take precedence on name collisions.

**Implication**: a child agent may end up with tools beyond what its YAML config specifies. For example, if `code_reviewer` is configured with `[filesystem, search, git]` but is delegated from an `assistant` with `[filesystem, exec, search, git, http, state, tasks]`, the child will also have access to `exec`, `http`, `state`, and `tasks` via inheritance.

## Architecture

```
agent.go        — Agent struct, New(), Run() ReAct loop, system prompt building
effect.go       — Effect interface, EffectFunc, IterationPhase, IterationContext
effects/        — Reusable Effect implementations (compact, etc.)
registry.go     — Registry for dynamic agent discovery + Factory pattern
tools.go        — Built-in orchestration tools
middleware.go   — Runner interface, Middleware type, built-in middleware
```

## Dependencies

- `pkg/agentctx/` — shared context key helpers (agent name propagation)
- `pkg/chats/` — chat, message, content, role types
- `pkg/modeladapter/` — Completer interface
- `pkg/tools/toolbox/` — ToolBox, Tool types
- `pkg/skill/` — Skill type for procedure loading

## Usage

```go
// Simple agent with tools.
a := agent.New("assistant", "Helpful bot", "Be helpful.", completer, agent.Options{
    MaxIterations: 20,
})
a.AddToolBoxes(myTools)
reply, err := a.Run(ctx)

// Agent with delegation.
reg := agent.NewRegistry()
reg.Register("researcher", "Finds information", researcherFactory)
reg.Register("coder", "Writes code", coderFactory)

orch := agent.New("orchestrator", "Coordinates work", "Break tasks into subtasks.", completer, agent.Options{
    MaxDelegationDepth: 3,
    Skills:             skills,
})
orch.SetRegistry(reg)
reply, err := orch.Run(ctx)
```
