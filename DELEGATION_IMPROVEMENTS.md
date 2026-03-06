# Delegation Improvements — Implementation Plan

Implementation plan for Section 1 (Delegation Improvements) from `BETTER_DELEGATION.md`. Covers peer handoff, streaming delegation, rich agent cards, and bidirectional child-to-parent communication.

## Phase 1: Rich Agent Capability Discovery (Agent Cards) ✅ COMPLETE

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
    SkillsTags     []string `yaml:"skills_tags,omitempty"`
    EstimatedCost  string   `yaml:"estimated_cost,omitempty"`
    MaxConcurrency int      `yaml:"max_concurrency,omitempty"`
}
```

> **Note:** `InputSchema` and `OutputSchema` have been removed from `Entry`. Delegation uses freeform text (`task` + `context` strings) and a uniform `CompletionResult` output — there is no structured input/output contract between agents. These fields can be added back if the delegation protocol evolves to support structured schemas.

Example YAML:

```yaml
agents:
  - name: coder
    description: A coding expert
    skills_tags: [coding, testing, refactoring]
    estimated_cost: medium
    max_concurrency: 3
```

Add validation in `Config.Validate()`:

- `estimated_cost` must be one of `""`, `"cheap"`, `"medium"`, `"expensive"`.
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

## Phase 2: Task Cancellation ✅ COMPLETE

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

## Phase 3: Streaming Delegation Results ✅ COMPLETE

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

## Phase 4: Peer Handoff ✅ COMPLETE

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

## Phase 5: Bidirectional Child-to-Parent Communication ✅ COMPLETE

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

## Phase 6: Interactive Delegation (Solution B) ✅ COMPLETE

**Priority:** P2 | **Complexity:** High | **Depends on Phase 5 (Child-to-Parent)**

Replaces the `autoAnswer` mechanism with a protocol where the `delegate` tool returns early when children ask questions, letting the parent LLM reason about them and respond via `answer_delegation_questions`. Questions are bundled through a shared queue and answered in batches for token efficiency.

### Core Design

Two tools alternate in a simple loop:

```
delegate(interactive, [A, B, C])
  -> [{d-1, question}, {completed B}, {d-2, question}]

answer_delegation_questions([{d-1, "REST"}, {d-2, "JSON"}])
  -> [{d-1, question}, {d-2, completed}]

answer_delegation_questions([{d-1, "v2"}])
  -> [{d-1, completed}]
```

Each call fans out concurrently, blocks until every child has either asked a question or completed, then returns a bundled result. Minimal round-trips — one tool call per "wave."

### 6.1 Shared `QuestionQueue` in `DelegationRegistry`

All children push questions to a single intake channel on the parent's registry. Each child keeps its own `answerCh` for 1:1 response routing.

```go
// PendingQuestion pairs a question with its delegation handle.
type PendingQuestion struct {
    DelegationID string
    Question     Question
}

// DelegationRegistry tracks active interactive delegations for a parent agent.
type DelegationRegistry struct {
    mu        sync.Mutex
    pending   map[string]*PendingDelegation
    questions chan PendingQuestion // single intake for ALL children
    counter   atomic.Int64
}

// PendingDelegation represents one in-flight interactive child.
type PendingDelegation struct {
    ID          string
    Agent       string
    Task        string
    AnswerCh    chan string              // route answers back to this child
    DoneCh      <-chan delegateResult    // receives final result
    Cancel      context.CancelFunc      // cancel the child
}
```

Queue depth is bounded by the number of active children — `request_input` is blocking, so each child can have at most one pending question.

**File:** `pkg/agent/interaction_registry.go` (new)

### 6.2 Modify `InteractionChannel` to use shared queue

Replace the per-child `questionCh` with a reference to the parent's shared queue. The child's `request_input` tool pushes a `PendingQuestion` (tagged with `delegation_id`) onto the shared channel, then blocks on its own `answerCh` as before.

```go
type InteractionChannel struct {
    delegationID string
    sharedQueue  chan<- PendingQuestion  // write-only ref to parent's queue
    answerCh     chan string             // per-child, read by request_input
    idCounter    atomic.Int64
}
```

`request_input` handler becomes:

```go
select {
case ic.sharedQueue <- PendingQuestion{DelegationID: ic.delegationID, Question: q}:
case <-ctx.Done():
    return "", ctx.Err()
}
// block for answer (unchanged)
select {
case answer := <-ic.answerCh:
    return answer, nil
case <-ctx.Done():
    return "", ctx.Err()
}
```

**File:** `pkg/agent/interaction.go` (modify)

### 6.3 Add `mode` field to `delegateTask`

```go
type delegateTask struct {
    Agent   string `json:"agent"`
    Task    string `json:"task"`
    Context string `json:"context"`
    TaskID  string `json:"task_id"`
    Mode    string `json:"mode"` // "" | "blocking" | "interactive"
}
```

Update the `delegate` tool's JSON schema to include `mode`.

**Behavior by mode:**

| Mode | Behavior |
|------|----------|
| `""` / `"blocking"` | Current behavior. `autoAnswer` handles questions. Tool blocks until all children complete. |
| `"interactive"` | Children run concurrently. Tool returns as soon as every child has either completed or asked a question. Pending children are registered in `DelegationRegistry`. |

When `mode` is `"interactive"`, the `delegate` handler:

1. Spawns all children with `InteractionChannel`s wired to the shared queue.
2. Runs each child in a goroutine, sending its final `delegateResult` on a per-child `doneCh`.
3. For each child, selects between `doneCh` and a question arriving on the shared queue (filtered by delegation ID).
4. Returns a mixed result array — completed results inline, pending questions as handles.

**Return type for interactive mode:**

```go
type interactiveDelegateResult struct {
    // Always present.
    Agent string `json:"agent"`

    // Set for completed children (mutually exclusive with DelegationID).
    Result     string            `json:"result,omitempty"`
    Completion *CompletionResult `json:"completion,omitempty"`
    Error      string            `json:"error,omitempty"`
    Warning    string            `json:"warning,omitempty"`

    // Set for children with pending questions (mutually exclusive with Result).
    DelegationID    string    `json:"delegation_id,omitempty"`
    PendingQuestion *Question `json:"pending_question,omitempty"`
}
```

**File:** `pkg/agent/delegation.go` (modify)

### 6.4 New tool: `answer_delegation_questions`

Single tool for batched question answering. Questions always come back inline from `delegate` or `answer_delegation_questions` — no separate polling tool needed.

```go
func answerDelegationQuestionsTool(a *Agent) toolbox.Tool
```

- **Name:** `answer_delegation_questions`
- **Input:**
  ```json
  {
    "answers": [
      {"delegation_id": "string", "answer": "string"}
    ]
  }
  ```
- **Handler:**
  1. Validates all delegation IDs exist in the registry.
  2. Sends all answers concurrently (each answer goes to the child's `answerCh`).
  3. For each answered child, selects between:
     - Another question on the shared queue (filtered by delegation ID) -> return as `pending_question`.
     - Child completion on `doneCh` -> return as completed result.
     - Timeout -> cancel child, return timeout error.
  4. Returns bundled array — same `[]interactiveDelegateResult` shape as the interactive `delegate` return.

The tool **blocks** until every answered child has either asked a follow-up or completed. This reduces token usage — the parent gets all results in one round-trip.

**File:** `pkg/agent/interaction_registry.go`

### 6.5 Configurable question timeout

Add to `AgentOptions`:

```go
type AgentOptions struct {
    // ... existing ...
    InteractionMode string `yaml:"interaction_mode"`    // "auto" | "interactive" | "blocking"
    QuestionTimeout string `yaml:"question_timeout"`    // Duration string, e.g. "5m". "" = no timeout.
}
```

Parsed to `time.Duration` in `delegationConfig`:

```go
type delegationConfig struct {
    // ... existing ...
    interactionMode string
    questionTimeout time.Duration
}
```

Applied in two places:
1. **`delegate` interactive handler** — while waiting for each child's initial question-or-completion.
2. **`answer_delegation_questions` handler** — while waiting for next question-or-completion after sending an answer.

```go
var timer <-chan time.Time
if a.delegation.questionTimeout > 0 {
    t := time.NewTimer(a.delegation.questionTimeout)
    defer t.Stop()
    timer = t.C
}

select {
case q := <-filteredQuestion:
    // next question
case result := <-pd.DoneCh:
    // completed
case <-timer:
    pd.Cancel()
    // return timeout error for this child
case <-ctx.Done():
    return "", ctx.Err()
}
```

**Validation in `config.go`:**

```go
if a.Options.QuestionTimeout != "" {
    if _, err := time.ParseDuration(a.Options.QuestionTimeout); err != nil {
        return nil, fmt.Errorf("engine: config: agent %q: question_timeout: %w", a.Name, err)
    }
}
if a.Options.InteractionMode != "" &&
    a.Options.InteractionMode != "auto" &&
    a.Options.InteractionMode != "interactive" &&
    a.Options.InteractionMode != "blocking" {
    return nil, fmt.Errorf(
        "engine: config: agent %q: interaction_mode must be \"auto\", \"interactive\", or \"blocking\"",
        a.Name,
    )
}
```

Example YAML:

```yaml
agents:
  - name: orchestrator
    options:
      max_delegation_depth: 3
      interaction_mode: interactive
      question_timeout: 5m
```

**File:** `pkg/engine/config.go` (modify)

### 6.6 Wire tools into `orchestrationToolBox`

Register `answer_delegation_questions` when the agent has a `DelegationRegistry`:

```go
func orchestrationToolBox(a *Agent) *toolbox.ToolBox {
    tb := toolbox.New()
    tb.Register(
        listAgentsTool(a),
        delegateTool(a),
    )
    if a.interactiveDelegations != nil {
        tb.Register(answerDelegationQuestionsTool(a))
    }
    return tb
}
```

**File:** `pkg/agent/delegation.go` (modify)

### 6.7 Agent struct changes

```go
type Agent struct {
    // ... existing ...
    interactiveDelegations *DelegationRegistry // nil when interaction_mode != "interactive"
}
```

Cleanup: `DelegationRegistry.Close()` cancels all pending children. Called via `defer` in `run()`:

```go
func (a *Agent) run(ctx context.Context) (message.Message, error) {
    if a.interactiveDelegations != nil {
        defer a.interactiveDelegations.Close()
    }
    // ... existing loop ...
}
```

**File:** `pkg/agent/agent.go` (modify)

### 6.8 Update system prompt

Add `HasInteractiveDelegation` to `promptBuilder`. Render when true:

```
<interactive_delegation_protocol>
You can delegate tasks with mode "interactive" when children may need your input.
Interactive delegation returns immediately with any pending questions from children.
Use answer_delegation_questions to respond to all pending questions in a single call.
Each answer call blocks until every answered child either asks a follow-up or completes.
Use "interactive" mode when the task may need clarification you can provide.
Use default mode (no mode field) for self-contained tasks where children have all context.
</interactive_delegation_protocol>
```

**File:** `pkg/agent/prompt.go` (modify)

### 6.9 Engine wiring

In `pkg/engine/registration.go`, parse config and wire into agent options:

```go
// In registerFactory or buildAgent:
if ac.Options.InteractionMode == "interactive" {
    opts.InteractionMode = "interactive"
}
if ac.Options.QuestionTimeout != "" {
    d, _ := time.ParseDuration(ac.Options.QuestionTimeout) // already validated
    opts.QuestionTimeout = d
}
```

In agent construction (`New()`), create the `DelegationRegistry` when interactive:

```go
if opts.InteractionMode == "interactive" {
    a.interactiveDelegations = NewDelegationRegistry()
}
```

**File:** `pkg/engine/registration.go` (modify)

### 6.10 Backward compatibility

| Scenario | Behavior |
|----------|----------|
| No `interaction_mode` in config | Default `"auto"` — `autoAnswer` as today |
| `interaction_mode: blocking` | No `InteractionChannel`, no `request_input` tool on children |
| `interaction_mode: interactive` | `DelegationRegistry` created, `answer_delegation_questions` tool available |
| `delegate` called without `mode` field | Blocking behavior regardless of `interaction_mode` config |
| `delegate` called with `mode: "interactive"` | Interactive behavior. Only works when `DelegationRegistry` exists (error otherwise) |

The `autoAnswer` goroutine is only started for blocking-mode delegations. Interactive-mode delegations skip it entirely — questions flow through the registry.

### 6.11 Tests

- **Unit: `DelegationRegistry`** — register, lookup by ID, `Close()` cancels all children, double-register rejection.
- **Unit: `answerDelegationQuestionsTool`** — sends answers concurrently, blocks for next wave, returns mixed results. Invalid delegation ID returns error. Partial answers (answer some, not all) rejected.
- **Unit: shared `QuestionQueue`** — multiple children push concurrently, questions arrive in order, bounded by child count.
- **Integration: single child interactive flow** — `delegate(interactive, [A])` -> question -> `answer(...)` -> completion.
- **Integration: multi-child parallel questions** — `delegate(interactive, [A, B, C])` -> B completes, A+C ask -> `answer([A, C])` -> A asks again, C completes -> `answer([A])` -> A completes.
- **Integration: child completes without asking** — interactive mode, child never calls `request_input`, appears as normal completed result in `delegate` return.
- **Integration: question timeout** — child asks, parent doesn't answer within timeout, child is canceled, timeout error returned.
- **Integration: parent context canceled** — `DelegationRegistry.Close()` cancels all pending children, blocked `answer_delegation_questions` returns `context.Canceled`.
- **Config: validation** — `interaction_mode` accepts `auto`/`interactive`/`blocking`, rejects others. `question_timeout` parses valid durations, rejects invalid.
- **Backward compat** — default config produces identical behavior to current `autoAnswer`. Blocking-mode `delegate` calls work alongside interactive ones on the same agent.

### 6.12 Update READMEs

- `pkg/agent/README.md`: document `DelegationRegistry`, shared queue, `answer_delegation_questions` tool, interactive delegation flow.
- `pkg/engine/README.md`: document `interaction_mode` and `question_timeout` config options.

### Sequence Diagram (parallel multi-child)

```
Parent LLM              delegate handler           Registry Queue        Children
    |                        |                          |               A    B    C
    |-- delegate(int,[A,B,C])|                          |               |    |    |
    |                        |-- spawn all 3 + wire --->|-------------->|    |    |
    |                        |                          |               |    |    |
    |                        |                          |  A: request_input("API?")
    |                        |                          |<----q(d-1)----|    |    |
    |                        |                          |               |    |    |
    |                        |                          |         B: task_complete
    |                        |<--- done(d-2) -----------|-------result--+----|    |
    |                        |                          |               |         |
    |                        |                          |  C: request_input("fmt?")
    |                        |                          |<----q(d-3)----+---------|
    |                        |                          |               |         |
    |<-[{d-1,q},{B,done},{d-3,q}]                       |               |         |
    |                        |                          |               |         |
    |-- answer([{d-1,"REST"},{d-3,"JSON"}]) ----------->|               |         |
    |                        |-- send answers --------->|--ans(d-1)---->|         |
    |                        |                          |--ans(d-3)---->+---------|
    |                        |                          |               |         |
    |                        |                          |  A: request_input("v?")
    |                        |                          |<----q(d-1)----|         |
    |                        |                          |         C: task_complete
    |                        |<--- done(d-3) -----------|-------result--+---------|
    |                        |                          |               |
    |<-[{d-1,q},{d-3,done}]--|                          |               |
    |                        |                          |               |
    |-- answer([{d-1,"v2"}])-|                          |               |
    |                        |-- send answer ---------->|--ans(d-1)---->|
    |                        |                          |         A: task_complete
    |                        |<--- done(d-1) -----------|-------result--|
    |                        |                          |
    |<-[{d-1,done}]----------|
```

---

## Dependency Graph

```
Phase 1 (Agent Cards) --------------------------+
                                                 +-- Phase 4 (Peer Handoff)
Phase 2 (Task Cancellation) --- Phase 3 (Streaming) --- Phase 5 (Child-to-Parent)
                                                                    |
                                                         Phase 6 (Interactive Delegation)
```

## Files Modified Per Phase

| Phase | New Files | Modified Files |
|-------|-----------|----------------|
| 1 | -- | `registry.go`, `delegation.go`, `prompt.go`, `config.go`, `registration.go` |
| 2 | -- | `store.go`, `delegation.go`, `agent.go` |
| 3 | `delegation_stream.go` | `delegation.go`, `agent.go`, `event.go`, `registration.go` |
| 4 | `handoff.go` | `completion.go`, `delegation.go`, `prompt.go`, `agent.go`, `config.go` |
| 5 | `interaction.go` | `delegation.go`, `agent.go`, `completion.go` |
| 6 | `interaction_registry.go` | `interaction.go`, `delegation.go`, `agent.go`, `prompt.go`, `config.go`, `registration.go` |

## Estimated Scope

| Phase | Changes | Tests |
|-------|---------|-------|
| 1 — Agent Cards | ~200 LOC | ~150 LOC |
| 2 — Task Cancellation | ~150 LOC | ~200 LOC |
| 3 — Streaming Delegation | ~400 LOC | ~300 LOC |
| 4 — Peer Handoff | ~500 LOC | ~400 LOC |
| 5 — Child-to-Parent Comm | ~600 LOC | ~400 LOC |
| 6 — Interactive Delegation | ~620 LOC | ~500 LOC |
