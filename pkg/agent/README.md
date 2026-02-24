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
- `Name()`, `Description()`, `Chat()`, `Prefix()` — accessors.

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
| `delegate_to_agent` | Delegates a task to another agent, returns its response. Requires `agent`, `task`, and `context` fields. |
| `spawn_agents` | Runs multiple agents concurrently, returns collected results. Each task requires `agent`, `task`, and `context` fields. |

Both `delegate_to_agent` and `spawn_agents` require a `context` field — background information (file contents, decisions, constraints) that the child agent needs. The context is prepended as a `<delegation_context>`-tagged user message before the task message, so the child sees: system prompt -> context -> task.

Safety guards: self-delegation rejected, `MaxDelegationDepth` enforced, concurrent spawn uses cancel-on-first-error.

### Sub-Agent Event Notifications

When `Options.EventNotifier` is set, orchestration tools publish lifecycle events for child agents:

- `agent_start` — emitted before `child.Run(ctx)` with `AgentEventData{Prefix}`.
- `agent_end` — emitted after `child.Run(ctx)` completes (success or error).

The notifier is automatically propagated to children so that nested delegation chains publish events at every level. The engine wires this to the `EventBus` so frontends can observe sub-agent activity.

### Display Prefix

`Options.Prefix` sets a configurable emoji/label for the agent (e.g. `"🤖"`, `"📝"`, `"🦾"`). It defaults to `"🤖"` when empty. Frontends read the prefix via `Agent.Prefix()` or from `AgentEventData` in lifecycle events to render agent output with the appropriate visual treatment.

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

| Effect | Kind | Phase | Description |
|--------|------|-------|-------------|
| `CompactEffect` | `compact` | BeforeComplete | Graduated context compaction: first trims old tool results (lightweight), then falls back to full summarisation when approaching context window limit |
| `TrimToolResultsEffect` | `trim_tool_results` | AfterComplete | Trims old tool result content to a configurable length, preserving the most recent N tool messages |

### Toolbox Inheritance

When an agent delegates to or spawns a child agent, the child receives a **union** of its own configured toolboxes and the parent's toolboxes. The sequence is:

1. The child's factory is called, producing a fresh agent with only its config-defined toolboxes.
2. The parent calls `child.AddToolBoxes(a.toolboxes...)`, which adds only toolboxes not already present (pointer-based deduplication).
3. The child's registry is set to the parent's registry, enabling further delegation.

`AddToolBoxes` deduplicates by pointer equality — if the parent and child share the same `*ToolBox` (e.g., both configured with `filesystem`), it will not be added twice. This prevents duplicate tool declarations from being sent to the LLM.

**Implication**: a child agent may end up with tools beyond what its YAML config specifies. For example, if `code_reviewer` is configured with `[filesystem, search, git]` but is delegated from an `assistant` with `[filesystem, exec, search, git, http, state, tasks]`, the child will also have access to `exec`, `http`, `state`, and `tasks` via inheritance — but shared toolboxes like `filesystem`, `search`, and `git` will not be duplicated.

## System Prompt Structure

The system prompt is built by `buildSystemPrompt()` using XML tags for clear section boundaries. Sections are ordered for prompt-cache friendliness (static content first, dynamic content last):

1. `<identity>` — Agent name and description (static, cacheable prefix)
2. `<instructions>` — Agent-specific instructions (static)
3. `<project_context>` — Project context loaded at startup (semi-static)
4. `<skills>` — Inline skill content (semi-static)
5. `<available_skills>` — On-demand skill descriptions (semi-static)
6. `<available_agents>` — Agent directory from registry (dynamic, last)

This ordering ensures LLM provider prompt caching can cache the stable prefix across iterations, and the XML tags help LLMs attend to section boundaries without relying on prose structure.

The `Skills` slice in `Options` controls which skills appear in sections 4 and 5. The engine can filter engine-level skills per agent via the `skills` config field — see `pkg/engine/README.md` for details.

## Architecture

```
agent.go        — Agent struct, New(), Run() ReAct loop, system prompt building, EventNotifier, Prefix
effect.go       — Effect interface, EffectFunc, IterationPhase, IterationContext
effects/        — Reusable Effect implementations (compact, etc.)
registry.go     — Registry for dynamic agent discovery + Factory pattern
tools.go        — Built-in orchestration tools, AgentEventData, sub-agent event publishing
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
    Prefix:        "🤖",
})
a.AddToolBoxes(myTools)
reply, err := a.Run(ctx)

// Agent with delegation and sub-agent event notifications.
reg := agent.NewRegistry()
reg.Register("researcher", "Finds information", researcherFactory)
reg.Register("coder", "Writes code", coderFactory)

orch := agent.New("orchestrator", "Coordinates work", "Break tasks into subtasks.", completer, agent.Options{
    MaxDelegationDepth: 3,
    Skills:             skills,
    Prefix:             "🧠",
    EventNotifier: func(ctx context.Context, kind, name string, data any) {
        fmt.Printf("sub-agent event: %s %s\n", kind, name)
    },
})
orch.SetRegistry(reg)
reply, err := orch.Run(ctx)
```
