# Why Coder Sub-Agents Produce Unfocused or Incomplete Work

Analysis of the multi-agent orchestration in Shelly, focused on the dev-team pattern where an orchestrator delegates to planner/coder agents.

## The Chain Reaction

The issues compound: minimal orchestrator instructions lead to underspecified task strings, which means the coder starts with almost no context, explores randomly to figure out what to do, burns iterations, and either hits `max_iterations` or produces a vague "done" response that the orchestrator can't verify.

---

## ~~Issue 1: Sub-Agents Receive Only a Bare Text String~~ (FIXED)

Fixed: Both `delegate_to_agent` and `spawn_agents` now have a required `context` field. The parent LLM provides background information (file contents, decisions, constraints) which is prepended as a `<delegation_context>` user message before the task message. See `pkg/agent/tools.go`.

## Issue 2: Orchestrator and Coder Instructions Are Too Minimal

**`.shelly/config.yaml`**

The orchestrator instruction is just *"Break down user requests into planning and coding tasks, then delegate."* The coder instruction is just *"Implement changes based on plans, write clean code, and run tests."*

Neither instructs agents on:
- What information must be included in a task handoff
- How to verify coder completion
- What the coder should read/check before starting (notes, task board)
- What format the coder should report results in
- How to handle delegation failure or iteration exhaustion

## Issue 3: No Structured Completion Signal

**`pkg/agent/agent.go:172-175`**

The ReAct loop terminates whenever the LLM produces a response with no tool calls:

```go
calls := reply.ToolCalls()
if len(calls) == 0 {
    return reply, nil
}
```

There is no distinction between "I genuinely completed the task" and "I ran out of ideas." The orchestrator receives only `reply.TextContent()` — a freeform string — with no structured indication of files modified, tests run, or whether the task actually succeeded.

For `spawn_agents`, results are collected as `[]spawnResult{Agent, Result, Error}`. If a coder "gives up" and writes a text reply without completing work, that goes in `Result` indistinguishably from genuine success.

## Issue 4: Compact Effect Is Silently Inert

**`.shelly/config.yaml`** — No `context_window` is set on the provider.

**`pkg/agent/effects/compact.go:76-93`** — `shouldCompact()` returns `false` when `context_window <= 0`:

```go
func (e *CompactEffect) shouldCompact(completer modeladapter.Completer) bool {
    if e.cfg.ContextWindow <= 0 || e.cfg.Threshold <= 0 {
        return false
    }
}
```

The coder declares effects (`trim_tool_results`, `compact`) but they never fire because `context_window` is unset. The coder accumulates unbounded context until it hits the provider's hard token limit, at which point responses become truncated or incoherent.

## Issue 5: Task Board Exists But Isn't Wired Into the Workflow

**`pkg/tasks/store.go`**

A full task lifecycle (`pending → in_progress → completed/failed`) and tools (`shared_tasks_*`) are available to both orchestrator and coder. But no agent instructions require using them:

- The orchestrator doesn't create tasks before delegating
- The coder doesn't claim or mark tasks complete
- The orchestrator doesn't watch tasks for completion status

The task board is a coordination mechanism that goes unused.

## Issue 6: Notes Are the Only Durable Cross-Agent State, But Their Use Isn't Enforced

The planner can write notes, but there's no instruction forcing the coder to `read_note` or `list_notes` before starting work. The planner→orchestrator→coder context chain depends entirely on the LLM deciding to use notes, which is unreliable.

## Issue 7: No Recovery Path for Iteration Exhaustion

**`pkg/agent/tools.go:106-110`**

When a coder hits `max_iterations: 20`, the tool returns `ErrMaxIterations` as an error:

```go
if err != nil {
    return "", fmt.Errorf("delegate_to_agent: agent %q: %w", di.Agent, err)
}
```

The orchestrator receives `content.ToolResult{IsError: true, Content: "...max iterations reached"}` with no guidance on recovery. It may re-delegate (burning another 20 iterations), give up, or hallucinate success.

## Issue 8: Concurrent Spawns Can Clobber Each Other

**`pkg/agent/tools.go:162-205`**

`spawn_agents` runs children concurrently with shared `*toolbox.ToolBox` pointers:

```go
for i, t := range si.Tasks {
    go func() {
        child.AddToolBoxes(a.toolboxes...)
        child.chat.Append(message.NewText("user", role.User, t.Task))
        reply, err := child.Run(ctx)
    }()
}
```

Two coders writing to the same files have no file-level coordination. The shared `permissions.Store` pointer means permission grants by one concurrent agent propagate immediately to siblings.

## Issue 9: `agentctx` Name Mismatch With Task Board

**`pkg/agent/agent.go:132`**

Each agent overwrites the context's agent name: `ctx = agentctx.WithAgentName(ctx, a.name)`. The task board's `handleClaim` uses this to auto-assign tasks. If the orchestrator claims a task before delegating, the assignee is `"orchestrator"` — not `"coder"` — causing a mismatch between the assignee and actual executor.

---

## Summary Table

| # | Problem | Impact | Key Location |
|---|---------|--------|-------------|
| ~~1~~ | ~~Bare text handoff~~ | ~~FIXED — `context` field added~~ | `tools.go` |
| 2 | Minimal agent instructions | No handoff protocol or verification | `config.yaml` |
| 3 | No structured completion | Can't distinguish success from giving up | `agent.go:172-175` |
| 4 | Compact effect inert | Unbounded context until provider hard limit | `compact.go:76-93`, `config.yaml` |
| 5 | Task board unused | No lifecycle coordination between agents | `tasks/store.go` |
| 6 | Notes not enforced | Durable state exists but isn't used reliably | `config.yaml` |
| 7 | No iteration exhaustion recovery | Orchestrator can't handle coder failure | `tools.go:106-110` |
| 8 | Concurrent file clobbering | Spawned coders overwrite each other | `tools.go:162-205` |
| 9 | Agent name mismatch in tasks | Task assignee doesn't match executor | `agent.go:132` |

---

## Fix Categories

### Configuration / Instructions (no code changes)
- Richer orchestrator instructions: require structured task specs, mandate note usage, define verification steps
- Richer coder instructions: read notes first, report files changed, use task board lifecycle
- Set `context_window` on the provider so compact/trim effects actually fire

### Code Changes
- ~~**Structured handoff**: Pass a structured task spec (not just free text) to child agents, optionally including relevant parent context snippets~~ (DONE)
- **Structured results**: Return structured data from sub-agents (files modified, tests run, status) instead of just `TextContent()`
- **File-level locking**: Coordinate concurrent spawned agents to prevent clobbering
- **Completion protocol**: Add an explicit "task complete" tool that sub-agents must call, distinguishing intentional completion from the loop simply ending
- ~~**Context propagation**: Optionally forward a summary of the parent's accumulated context to the child at spawn time~~ (DONE)
