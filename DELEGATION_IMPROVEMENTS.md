# Delegation Improvements — Implementation Plan

Implementation plan for Section 1 (Delegation Improvements) from `BETTER_DELEGATION.md`. Covers peer handoff, streaming delegation, rich agent cards, and bidirectional child-to-parent communication.

## Phase 1: Rich Agent Capability Discovery (Agent Cards)

**Priority:** P1 | **Complexity:** Low | **No breaking changes**

This is the foundation — richer metadata enables better decisions in all subsequent phases.

### 1.1 Extend `Entry` in `pkg/agent/registry.go`

```go
type Entry struct {
    Name           string            `json:"name"`
    Description    string            `json:"description"`
    Skills         []string          `json:"skills,omitempty"`          // capability tags
    InputSchema    json.RawMessage   `json:"input_schema,omitempty"`    // expected input shape (JSON Schema)
    OutputSchema   json.RawMessage   `json:"output_schema,omitempty"`   // expected output shape (JSON Schema)
    EstimatedCost  string            `json:"estimated_cost,omitempty"`  // "cheap" | "medium" | "expensive"
    MaxConcurrency int               `json:"max_concurrency,omitempty"` // 0 = unlimited
}
```

### 1.2 Update `Registry.Register()` signature

Add a new `RegisterEntry(entry Entry, factory Factory)` method that accepts a full `Entry`. Keep the existing `Register(name, desc, factory)` as a convenience that builds a minimal `Entry`.

### 1.3 Add YAML config fields in `pkg/engine/config.go`

New fields on `AgentConfig`:

```go
type AgentConfig struct {
    // ... existing fields ...
    SkillsTags     []string       `yaml:"skills_tags,omitempty"`
    EstimatedCost  string         `yaml:"estimated_cost,omitempty"`
    MaxConcurrency int            `yaml:"max_concurrency,omitempty"`
    InputSchema    map[string]any `yaml:"input_schema,omitempty"`
    OutputSchema   map[string]any `yaml:"output_schema,omitempty"`
}
```

`InputSchema` and `OutputSchema` are typed as `map[string]any` because YAML natively unmarshals JSON Schema objects into nested maps. This avoids needing a dedicated schema struct — arbitrary JSON Schema is supported.

Example YAML:

```yaml
agents:
  - name: coder
    description: A coding expert
    skills_tags: [coding, testing, refactoring]
    estimated_cost: medium
    max_concurrency: 3
    input_schema:
      type: object
      properties:
        task: { type: string }
        files: { type: array, items: { type: string } }
    output_schema:
      type: object
      properties:
        files_modified: { type: array, items: { type: string } }
        summary: { type: string }
```

Add validation in `Config.Validate()`:

- `estimated_cost` must be one of `""`, `"cheap"`, `"medium"`, `"expensive"`.
- `input_schema` and `output_schema`, when present, must contain a `"type"` key (basic sanity check — not full JSON Schema validation).
- `max_concurrency` must be >= 0.

### 1.4 Wire schemas through `pkg/engine/registration.go`

The `registrationContext` gains new fields from `AgentConfig`. In `registerFactory`, the engine marshals the `map[string]any` schema values to `json.RawMessage` before passing them to `RegisterEntry`:

```go
// In buildRegistrationContext or registerFactory:
var inputSchema, outputSchema json.RawMessage
if ac.InputSchema != nil {
    inputSchema, err = json.Marshal(ac.InputSchema)
    if err != nil {
        return fmt.Errorf("engine: agent %q: invalid input_schema: %w", ac.Name, err)
    }
}
if ac.OutputSchema != nil {
    outputSchema, err = json.Marshal(ac.OutputSchema)
    if err != nil {
        return fmt.Errorf("engine: agent %q: invalid output_schema: %w", ac.Name, err)
    }
}

e.registry.RegisterEntry(agent.Entry{
    Name:           ac.Name,
    Description:    ac.Description,
    Skills:         ac.SkillsTags,
    InputSchema:    inputSchema,
    OutputSchema:   outputSchema,
    EstimatedCost:  ac.EstimatedCost,
    MaxConcurrency: ac.MaxConcurrency,
}, factory)
```

This keeps `pkg/agent` free of YAML concerns — it only sees `json.RawMessage`. The engine handles the YAML-to-JSON boundary.

### 1.5 Update `list_agents` tool in `pkg/agent/delegation.go`

`listAgentsTool` already serializes `[]Entry` via `json.Marshal` — the new fields (including `InputSchema`/`OutputSchema` as `json.RawMessage`) will be included automatically via JSON tags. No handler change needed.

The LLM receives the full schema JSON when it calls `list_agents`, enabling it to understand what each agent expects and produces.

### 1.6 Update system prompt in `pkg/agent/prompt.go`

The `<available_agents>` section currently renders `- **name**: description`. Extend it to include skill tags, cost, and schema summaries when present:

```
- **coder**: A coding expert [skills: coding, testing] [cost: medium]
  Input: {task: string, files: string[]}
  Output: {files_modified: string[], summary: string}
```

Schema rendering in the prompt should be compact. Rather than dumping the full JSON Schema, extract a simplified type signature from the schema's `properties` map. If the schema is too complex (deeply nested or >5 properties), show only the top-level property names and append `...`. This keeps the system prompt concise while still giving the orchestrator agent enough information to construct well-shaped delegation contexts.

Full schemas remain available via the `list_agents` tool for the LLM to inspect on demand.

### 1.7 Tests

- `registry_test.go`: `RegisterEntry` stores and retrieves full entries including schemas. `List()` returns entries with `json.RawMessage` schemas intact.
- `delegation_test.go`: `list_agents` returns enriched entries. Verify JSON output contains `input_schema`/`output_schema` keys when set, and omits them when nil.
- `prompt_test.go`: system prompt renders skill tags, cost, and compact schema summaries. Test with schemas present, absent, and with complex (>5 property) schemas that trigger truncation.
- `config_test.go`: YAML parsing of new fields. Schema round-trip: YAML `map[string]any` -> `json.Marshal` -> `json.RawMessage` -> `json.Unmarshal` back to map, verify equality. Validation rejects invalid `estimated_cost` values and schemas missing `"type"`.
- `registration_test.go`: Engine wires `AgentConfig` schemas through to registry entries correctly. Nil schemas in config produce nil `json.RawMessage` in entries.

### 1.8 Update READMEs

- `pkg/agent/README.md`: document new `Entry` fields, `RegisterEntry`, explain that `InputSchema`/`OutputSchema` are `json.RawMessage` containing JSON Schema.
- `pkg/engine/README.md`: document new YAML config fields. Include examples of `input_schema`/`output_schema` in YAML. Explain the `map[string]any` -> `json.RawMessage` conversion.

---

## Phase 2: Task Cancellation

**Priority:** P0 | **Complexity:** Low | **Prerequisite for streaming (Phase 3)**

### 2.1 Add `StatusCanceled` to `pkg/tasks/store.go`

```go
const StatusCanceled Status = "canceled"
```

Update lifecycle: `pending -> in_progress -> completed | failed | canceled`.

### 2.2 Add `Cancel(id string) error` to `Store`

- Sets status to `canceled` if the task is in `pending` or `in_progress`.
- Returns error if already in a terminal state (`completed`, `failed`, `canceled`).
- Broadcasts to `WatchCompleted` watchers (treat `canceled` as terminal alongside `completed`/`failed`).

### 2.3 Add `cancel` tool to `Store.Tools()`

New tool `{ns}_tasks_cancel` that calls `Cancel(id)`.

### 2.4 Propagate context cancellation in `pkg/agent/delegation.go`

Currently `runDelegateTask` creates `childCtx, childCancel := context.WithCancel(ctx)`. Add a mechanism where task cancellation triggers `childCancel()`:

- Extend `TaskBoard` interface with `WatchCanceled(id string) <-chan struct{}` (optional, via interface assertion).
- In `runDelegateTask`, when `task_id` is provided, start a goroutine that watches for cancellation and calls `childCancel()`.

### 2.5 Handle `canceled` in delegation result

When `child.Run()` returns `context.Canceled` due to task cancellation, produce a `delegateResult` with `Error: "task canceled"` and update the task status to `canceled`.

### 2.6 Tests

- `store_test.go`: Cancel from pending, in_progress, terminal states; WatchCompleted unblocks on cancel.
- `delegation_test.go`: task cancellation propagates to child context.
- Tool integration tests for `{ns}_tasks_cancel`.

### 2.7 Update READMEs

- `pkg/tasks/README.md`: document `canceled` status, `Cancel` method, cancel tool.
- `pkg/agent/README.md`: document cancellation propagation.

---

## Phase 3: Streaming Delegation Results

**Priority:** P1 | **Complexity:** Medium | **Depends on Phase 2 (cancellation)**

### 3.1 Define `DelegationEvent` types in `pkg/agent/delegation.go`

```go
type DelegationEventKind string

const (
    DelegationStatus   DelegationEventKind = "status"   // lifecycle transitions
    DelegationProgress DelegationEventKind = "progress" // partial outputs
    DelegationResult   DelegationEventKind = "result"   // final result
)

type DelegationEvent struct {
    Kind    DelegationEventKind `json:"kind"`
    Agent   string              `json:"agent"`
    Message string              `json:"message,omitempty"`
    Result  *delegateResult     `json:"result,omitempty"`
}
```

### 3.2 Add `DelegationStream` channel type

```go
type DelegationStream struct {
    Events <-chan DelegationEvent
    cancel context.CancelFunc
}
```

### 3.3 Create a streaming-aware child `EventFunc`

When `runDelegateTask` sets up the child agent, inject a custom `EventFunc` that intercepts `message_added` events (specifically tool results and assistant messages) and forwards summaries as `DelegationProgress` events on the stream channel.

### 3.4 Add `delegateStreaming` tool

A new orchestration tool alongside `delegate` (or a `stream: true` option on `delegate`):

- Instead of blocking until all children complete, immediately returns a delegation ID.
- Children run in background goroutines, writing `DelegationEvent`s to a channel.
- Parent can poll results via a `check_delegation` tool or receive them via the event system.

**Alternative (simpler, recommended first):** Keep `delegate` blocking but emit `DelegationEvent`s through the existing `EventNotifier`, allowing the engine/TUI to display real-time progress without changing the parent agent's ReAct loop. This approach:

- Adds a new event kind `EventDelegationProgress` to the `EventBus`.
- The child's `EventFunc` publishes progress events upward.
- No new tools needed — the parent still blocks.

### 3.5 Add cancellation support to streaming delegation

Expose a `cancel_delegation` tool (or reuse task cancellation from Phase 2) that cancels specific children mid-flight:

```go
func cancelDelegationTool(a *Agent) toolbox.Tool
```

This calls the child's `childCancel()` from the cancel registry.

### 3.6 Engine event wiring

In `pkg/engine/registration.go`, wire the new delegation progress events to the `EventBus` via the existing `buildAgentEventNotifier()`.

### 3.7 Tests

- Unit tests for `DelegationEvent` serialization.
- Integration test: parent delegates, child emits progress, events arrive at engine EventBus.
- Integration test: parent cancels child mid-delegation.

### 3.8 Update READMEs

- `pkg/agent/README.md`: document streaming delegation events, cancellation tool.
- `pkg/engine/README.md`: document new event kinds.

---

## Phase 4: Peer Handoff

**Priority:** P2 | **Complexity:** High | **Depends on Phases 1 & 2**

### 4.1 Design the handoff protocol

A peer handoff transfers control from the current agent to a sibling, without returning to the parent. The key challenge: the current agent's ReAct loop must exit, and the peer must produce the result that the parent's `delegate` call is waiting for.

**Approach:** Handoff as a special completion signal.

- When an agent calls `handoff`, its loop stops (like `task_complete`).
- The delegation machinery in the parent detects the handoff and starts the peer agent.
- The peer receives the accumulated context and continues.

### 4.2 Add `HandoffResult` to `pkg/agent/completion.go`

```go
type HandoffResult struct {
    TargetAgent string `json:"target_agent"`
    Reason      string `json:"reason"`
    Context     string `json:"context"`  // context to pass to peer
}
```

Store it on the `Agent` alongside `CompletionResult`.

### 4.3 Add `handoff` tool

New tool available to sub-agents (depth > 0) alongside `task_complete`:

```go
func (hh *handoffHandler) tool() toolbox.Tool
```

Input: `{target_agent, reason, context}`. Calling it sets the `HandoffResult` and stops the loop (same mechanism as `task_complete` — checked via `IsComplete()`).

### 4.4 Modify `runDelegateTask` to handle handoffs

After `child.Run()` returns, check for `HandoffResult`:

```go
if hr := child.HandoffResult(); hr != nil {
    // Spawn the target peer agent
    peer, ok := a.registry.Spawn(hr.TargetAgent, a.depth+1)
    // Transfer context: child's chat summary + handoff context
    // Run peer, return peer's result as the delegation result
}
```

**Handoff chain limit:** Add a max handoff count (e.g., 3) to prevent infinite peer-to-peer bouncing.

### 4.5 Context transfer strategy

The peer needs enough context to continue without re-exploring. Options:

1. **Summary transfer** (recommended): The handoff tool requires a `context` field where the agent summarizes relevant state. This is prepended to the peer as `<handoff_context>`.
2. **Chat transfer**: Copy the full chat (too expensive, breaks toolbox isolation).

### 4.6 Update system prompt

Add a `<handoff_protocol>` section for sub-agents explaining when and how to use `handoff` vs `task_complete`.

### 4.7 Add YAML config

```yaml
agents:
  - name: coder
    options:
      max_handoffs: 3  # max peer-to-peer transfers (0 = disabled)
```

### 4.8 Tests

- `completion_test.go`: handoff tool sets `HandoffResult`, stops loop.
- `delegation_test.go`: handoff triggers peer spawn, peer result returned to parent.
- Handoff chain limit enforcement.
- Self-handoff rejection.
- Handoff to nonexistent agent.

### 4.9 Update READMEs

- `pkg/agent/README.md`: document handoff tool, protocol, chain limits.
- `pkg/engine/README.md`: document `max_handoffs` config.

---

## Phase 5: Bidirectional Child-to-Parent Communication

**Priority:** P3 | **Complexity:** High | **Depends on Phase 3 (streaming)**

### 5.1 Design the interaction protocol

This is the most complex feature. A child agent needs to ask a question, pause its ReAct loop, wait for the parent to answer, then continue.

**Approach:** Channel-based question/answer with parent loop integration.

### 5.2 Add `InteractionChannel` type in `pkg/agent/`

```go
type InteractionChannel struct {
    questionCh chan Question
    answerCh   chan string
}

type Question struct {
    ID      string `json:"id"`
    Agent   string `json:"agent"`
    Content string `json:"content"`
}
```

### 5.3 Add `request_input` tool for sub-agents

Available when depth > 0 and an `InteractionChannel` is wired:

```go
func requestInputTool(a *Agent, ic *InteractionChannel) toolbox.Tool
```

The handler:

1. Sends a `Question` on `questionCh`.
2. Blocks on `answerCh` (with context cancellation).
3. Returns the answer as the tool result.

From the child's perspective, it is just another blocking tool call.

### 5.4 Parent-side: intercept questions during delegation

In `runDelegateTask`, when the child has an `InteractionChannel`:

1. Run the child in a goroutine.
2. Select on the question channel and child completion.
3. When a question arrives, inject it as a special event into the parent's context.

**Challenge:** The parent's ReAct loop is blocked inside `delegate`'s tool handler. The parent cannot reason about the question while `delegate` is executing.

**Solution A — Auto-answer from context:** The `delegate` tool itself answers the question using the delegation context, without involving the parent LLM. Simple but limited.

**Solution B — Bubble up to parent:** The `delegate` tool returns a partial result indicating a question is pending. The parent answers in its next ReAct iteration, then calls a `respond_to_child` tool. This requires:

- A `pending_questions` tool to list unanswered questions.
- A `respond_to_child` tool to send answers.
- The child's loop remains paused (blocked on `answerCh`).
- The `delegate` tool becomes non-blocking for this mode.

**Recommended:** Start with Solution A for simplicity, upgrade to Solution B later.

### 5.5 Solution A implementation

```go
func autoAnswer(parentContext string, question Question) string {
    // Simple heuristic: return the delegation context as the answer
    // with the question echoed for clarity
    return fmt.Sprintf("Based on the delegation context: %s", parentContext)
}
```

This is a placeholder — in practice the parent's delegation context often contains the answer.

### 5.6 Solution B implementation (future)

- Modify `delegate` to support `mode: "interactive"`.
- Interactive delegation returns immediately with a delegation handle.
- Add `check_delegation_questions` and `answer_delegation_question` tools.
- The parent's ReAct loop can interleave answering questions with other work.

### 5.7 Tests

- `request_input` tool blocks and returns answer.
- Auto-answer mode works with delegation context.
- Context cancellation unblocks `request_input`.
- Timeout on unanswered questions.

### 5.8 Update READMEs

- `pkg/agent/README.md`: document `request_input` tool, interaction protocol.
- `pkg/engine/README.md`: document interactive delegation config.

---

## Dependency Graph

```
Phase 1 (Agent Cards) --------------------------+
                                                 +-- Phase 4 (Peer Handoff)
Phase 2 (Task Cancellation) --- Phase 3 (Streaming) --- Phase 5 (Child-to-Parent)
```

## Files Modified Per Phase

| Phase | New Files | Modified Files |
|-------|-----------|----------------|
| 1 | -- | `registry.go`, `delegation.go`, `prompt.go`, `config.go`, `registration.go` |
| 2 | -- | `store.go`, `delegation.go`, `agent.go` |
| 3 | `delegation_stream.go` | `delegation.go`, `agent.go`, `event.go`, `registration.go` |
| 4 | `handoff.go` | `completion.go`, `delegation.go`, `prompt.go`, `agent.go`, `config.go` |
| 5 | `interaction.go` | `delegation.go`, `agent.go`, `completion.go` |

## Estimated Scope

| Phase | Changes | Tests |
|-------|---------|-------|
| 1 — Agent Cards | ~200 LOC | ~150 LOC |
| 2 — Task Cancellation | ~150 LOC | ~200 LOC |
| 3 — Streaming Delegation | ~400 LOC | ~300 LOC |
| 4 — Peer Handoff | ~500 LOC | ~400 LOC |
| 5 — Child-to-Parent Comm | ~600 LOC | ~400 LOC |
