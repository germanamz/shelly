# Agent System

The `pkg/agent/` package implements a unified agent type running a ReAct (Reason + Act) loop with dynamic delegation, middleware, and an effects system for conversation management.

## Core Types

### Agent Struct

```go
type Agent struct {
    name, configName   string                // instance name + template/kind name
    description        string
    instructions       string
    completer          modeladapter.Completer
    chat               *chat.Chat
    toolboxes          []*toolbox.ToolBox
    registry           *Registry             // factory registry for delegation
    middleware         []Middleware
    effects            []Effect
    delegation         delegationConfig       // maxDepth, maxHandoffs, taskBoard, reflectionDir
    events             eventConfig            // notifier, eventFunc, cancelRegistrar
    completion         completionHandler      // tracks task_complete calls
    handoff            handoffHandler         // tracks handoff calls
    interaction        *InteractionChannel    // parent↔child communication
    depth              int                    // current delegation depth
    maxIterations      int
    warnIterations     int                    // inject wrap-up message at this iteration
}
```

Created via `New(name, description, instructions, completer, opts)`.

### Options

Key fields in `Options`: `MaxIterations`, `WarnIterations`, `MaxDelegationDepth`, `MaxHandoffs`, `Skills []skill.Skill`, `Middleware []Middleware`, `Effects []Effect`, `Context string`, `EventNotifier`, `EventFunc`, `TaskBoard`, `ReflectionDir`, `InteractionMode` (""|"auto"|"interactive"|"blocking"), `QuestionTimeout`, `ProviderLabel`, `Prefix`.

### Sentinel Errors

- `ErrMaxIterations` — loop exceeded MaxIterations without final answer
- `ErrTokenBudgetExhausted` — cumulative tokens exceeded budget
- `ErrTimeBudgetExhausted` — cumulative LLM time exceeded budget
- `ErrStallDetected` — agent not making progress

## ReAct Loop (`Run` / `run`)

`Run(ctx)` wraps the internal `run` method with middleware, then executes:

```
1. Init() — ensure system prompt exists
2. Collect all toolboxes (user + orchestration + completion/handoff + effect-provided)
3. Deduplicate tools → tool declarations + handler map
4. Reset effects that implement Resetter
5. Cache toolTokens estimate
6. LOOP (until maxIterations or final answer):
   a. Inject wrap-up warning if i >= warnIterations (once)
   b. Estimate tokens
   c. Run effects at PhaseBeforeComplete
   d. Filter tools via ToolFilter effects
   e. Call completer.Complete(ctx, chat, tools) → reply
   f. Append reply to chat, emit "message_added"
   g. Run effects at PhaseAfterComplete
   h. Extract tool calls from reply
   i. If no tool calls → return reply (final answer)
   j. Execute all tool calls concurrently (sync.WaitGroup)
   k. Emit tool_call_start/tool_call_end events
   l. Cap tool results at 32K runes, append to chat
   m. If completion.IsComplete() or handoff.IsHandoff() → return reply
7. Return ErrMaxIterations
```

Tool results are capped at `maxToolResultRunes` (32000). Error results are never capped.

## Delegation System

### Registry (`registry.go`)

Factory-based agent creation. Thread-safe via `sync.RWMutex`.

```go
type Factory func() *Agent              // creates fresh agent with clean chat
type Entry struct {                      // agent directory entry
    Name, Description string
    Skills []string
}

type Registry struct {
    factories map[string]Factory
    entries   map[string]Entry
}
```

Key methods: `Register(name, factory, entry)`, `Spawn(name, depth) (*Agent, bool)`, `List() []Entry`, `NextID(name) int` (auto-increment for unique instance names).

### Delegation Tool (`delegation.go`)

When `canDelegate()` is true (registry set, depth < maxDepth), the agent gets a `delegate` tool. The tool accepts a `DelegateInput` with `Tasks []delegateTask` (each has `agent`, `task`, `context`, optional `mode`, `task_id`).

**Delegation flow:**
1. Parse input, resolve agent names from registry
2. If any task has `mode: "interactive"` → `runInteractiveDelegate`
3. Otherwise → run all tasks concurrently via `runDelegateTask`

**`runDelegateTask`**: Spawns child via `buildDelegateChild` → `spawnChild`, claims task on TaskBoard, runs with `runChildWithHandoff`.

**`propagateParentConfig`**: Copies registry, event notifier, reflection dir, task board from parent to child. Generates unique instance name: `{configName}-{taskSlug}-{seqID}`.

**Handoffs**: A child can call `handoff` tool to transfer to a peer agent. `runChildWithHandoff` handles the chain with `maxHandoffDefault=3` limit.

### Delegation Streaming (`delegation_stream.go`)

`DelegationEvent` with kinds: `DelegationStatus`, `DelegationProgress`. The `delegationProgressFunc` wraps a parent's EventFunc to forward child events with rune-count tracking.

### Completion & Handoff

- **`completion.go`**: `CompletionResult{Status, Summary, FilesModified, TestsRun, Caveats}`. Registered as `task_complete` tool for sub-agents (depth > 0).
- **`handoff.go`**: `HandoffResult{TargetAgent, Reason, Context}`. Registered as `handoff` tool when MaxHandoffs > 0.

### Interaction Channel (`interaction.go`)

Bidirectional parent↔child communication via `InteractionChannel`:

```go
type Question struct { ID, Agent, Content string }
type InteractionChannel struct {
    answerCh    chan string          // parent sends answer
    questionCh  chan PendingQuestion // child sends question  
    sharedQueue chan PendingQuestion // nil for per-child, set for interactive mode
}
```

- **Auto mode**: Parent auto-answers from delegation context
- **Interactive mode**: Questions routed to shared `DelegationRegistry` queue, parent LLM answers
- **Blocking mode**: Questions block until external answer

`request_input` tool registered for sub-agents with an InteractionChannel.

## Middleware

```go
type Runner interface { Run(ctx context.Context) (message.Message, error) }
type RunnerFunc func(ctx context.Context) (message.Message, error)
type Middleware func(next Runner) Runner
```

Applied in reverse order so first middleware in the slice is outermost. Wraps `Run()` — can observe/modify before and after the entire ReAct loop.

Usage: logging, metrics, timing, error wrapping.

## Effects System

Effects are per-iteration hooks that run inside the ReAct loop. **The agent package defines interfaces; implementations live in `pkg/agent/effects/`** (clean separation — agent never imports effects).

### Core Interfaces (`effect.go`)

```go
type IterationPhase int // PhaseBeforeComplete | PhaseAfterComplete

type IterationContext struct {
    Phase           IterationPhase
    Iteration       int
    Chat            *chat.Chat
    Completer       modeladapter.Completer
    AgentName       string
    EstimatedTokens int
    ToolTokens      int
}

type Effect interface {
    Eval(ctx context.Context, ic IterationContext) error
}

type Resetter interface { Reset() }       // reset per-run state
type ToolFilter interface {               // filter tools per iteration
    FilterTools(ctx context.Context, ic IterationContext, tools []toolbox.Tool) []toolbox.Tool
}
type ToolProvider interface {             // inject extra tools
    ProvidedTools() *toolbox.ToolBox
}
```

### Effect Implementations (`pkg/agent/effects/`)

#### 1. Compact (`compact.go`)
**Phase**: BeforeComplete. Summarizes old messages when token count exceeds threshold.

- Configurable via `CompactConfig{Threshold float64, Model modeladapter.Completer}`
- Default threshold: 80% of context window
- Splits chat into kept prefix (system) + summarizable body + recent suffix
- Calls the LLM with a summarization prompt to compress the body
- Replaces body with a single summary message
- Thread-safe via mutex

#### 2. Loop Detection (`loopdetect.go`)
**Phase**: AfterComplete. Detects when the agent is stuck in repetitive patterns.

- `LoopDetectConfig{Threshold int, WindowSize int}` (defaults: 3, 10)
- Compares recent assistant messages in a sliding window
- If same tool call pattern repeats `Threshold` times → injects a user message telling the agent to try a different approach
- Implements `Resetter` to clear state between runs

#### 3. Observation Masking (`observation_mask.go`)
**Phase**: BeforeComplete. Replaces large tool results with placeholders to save tokens.

- `ObservationMaskConfig{RecentWindow int, Threshold float64}` (defaults: 10, 0.6)
- Keeps recent N messages unmasked
- Masks older tool results exceeding the threshold fraction of context
- Uses `obs_masked` metadata key to avoid re-masking
- Non-destructive: only modifies the Content field of tool results

#### 4. Context Offload (`offload.go`)
**Phase**: BeforeComplete. Offloads large tool results to disk files and provides a `recall` tool to retrieve them.

- `OffloadConfig{Threshold int, RecentWindow int, BaseDir string}`
- Default threshold: 4000 runes, recent window: 6 messages
- Writes offloaded content to `{BaseDir}/{agentName}/{hash}.md`
- Replaces in-chat content with pointer: `[content offloaded → {path}] Use recall tool to retrieve`
- Implements `ToolProvider` to register the `recall` tool
- Thread-safe via mutex, implements `Resetter`

#### 5. Trim Tool Results (`trim_tool_results.go`)
**Phase**: BeforeComplete. Truncates older tool results to a max character length.

- `TrimToolResultsConfig{MaxResultLength int, PreserveRecent int}` (defaults: 500, 4)
- Preserves the N most recent messages untouched
- Truncates older tool results and appends `"… [trimmed]"` suffix
- Uses `trimmed` metadata key to avoid re-trimming

## Event System

Two levels of event notification:

### EventNotifier (orchestration-level)
```go
type EventNotifier func(ctx context.Context, kind string, agentName string, data any)
```
Published by delegation tools for agent lifecycle: `"agent_start"`, `"agent_end"`. Carries `AgentEventData{Prefix, Parent, ProviderLabel, Task}`.

### EventFunc (loop-level)
```go
type EventFunc func(ctx context.Context, kind string, data any)
```
Published by the ReAct loop: `"tool_call_start"`, `"tool_call_end"` (`ToolCallEventData`), `"message_added"` (`MessageAddedEventData`).

### Cancel Registration
`CancelRegistrar`/`CancelUnregistrar` functions allow the TUI/engine to cancel individual sub-agents by name.

## Prompt Construction (`prompt.go`)

`promptBuilder` assembles the system prompt from: name, description, instructions, project context, skills (with YAML frontmatter), behavioral hints, delegation registry listing, interaction/handoff protocol descriptions. Pure value type, no side effects.

## Reflection (`reflection.go`)

On delegation failure (iteration exhaustion), the agent writes a reflection note to `reflectionDir` containing: task description, summary, and caveats. Filename uses `taskSlug()` for readability. Helps future agents learn from failures.

## Key Patterns

1. **Single unified type** — one `Agent` handles all patterns (root, child, interactive)
2. **Depth-based behavior** — depth=0 is root (no task_complete tool), depth>0 is sub-agent
3. **Clean effect separation** — agent defines interfaces, `effects/` implements them
4. **Concurrent tool execution** — all tool calls in a reply run in parallel via `sync.WaitGroup`
5. **Factory pattern** — Registry holds factories, Spawn creates fresh instances per delegation
6. **Config propagation** — `propagateParentConfig` copies registry/events/taskboard to children
7. **Graceful degradation** — wrap-up warnings, reflection notes, auto-answering of questions
