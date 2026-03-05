# Better Delegation — Research Findings

Research into multi-agent communication patterns across the industry (A2A protocol, OpenAI Agents SDK, CrewAI, LangGraph, AutoGen, Anthropic's own multi-agent research system) surfaced several concrete improvements for Shelly's agent-to-agent communication.

## 1. Delegation Improvements

### 1.1 Peer Handoff

**Inspiration:** OpenAI Agents SDK handoff pattern

Shelly currently only supports orchestrator-to-worker delegation. A complementary pattern is peer handoff — where an agent transfers control of the conversation to a sibling agent without returning to a parent. This is useful when Agent A realizes mid-task that Agent B is better suited.

Currently Shelly would require bouncing back to the orchestrator, adding a round-trip and wasting tokens. A `handoff` tool that transfers the current conversation context directly to a peer would reduce latency and token cost for lateral transfers.

**Implementation sketch:**
- New `handoff` orchestration tool alongside `delegate`
- Transfers the current chat (or a summary) to the target agent
- The current agent's loop exits, the peer continues
- The parent receives the peer's result instead

### 1.2 Streaming Delegation Results

**Inspiration:** Google A2A Protocol (SSE streaming, `TaskStatusUpdateEvent`, `TaskArtifactUpdateEvent`)

Shelly's `delegate` tool blocks until all children complete. Adding streaming progress from child to parent would let the orchestrator:

- React to partial results and replan mid-flight
- Cancel children early if a result invalidates remaining work
- Display real-time progress to the user

A2A uses Server-Sent Events with two event types: status updates (lifecycle transitions) and artifact updates (partial outputs). A similar pattern could use Go channels internally.

**Implementation sketch:**
- Children write progress events to a channel
- Parent's delegate tool can optionally stream these
- Add a `cancel` mechanism to abort children when no longer needed

### 1.3 Rich Agent Capability Discovery (Agent Cards)

**Inspiration:** Google A2A Agent Cards

Shelly's `list_agents` only returns `{name, description}`. A2A introduces Agent Cards — JSON metadata describing an agent's skills, accepted input types, output schemas, and resource requirements.

Richer capability metadata would help orchestrators make better delegation decisions, especially as the agent count grows. The LLM could match task requirements to agent capabilities more precisely.

**Proposed additions to registry entries:**
- `skills []string` — high-level capability tags
- `input_schema` / `output_schema` — expected data shapes
- `estimated_cost` — relative cost indicator (cheap/medium/expensive)
- `max_concurrency` — how many instances can run in parallel

### 1.4 Bidirectional Child-to-Parent Communication

**Inspiration:** A2A `input-required` task state, Anthropic multi-agent research system

Currently children can only return results at the end via `task_complete`. Anthropic's own research system found that agents need to ask clarifying questions back to the parent. A2A has an explicit `input-required` task state for this.

Adding a `request_input` tool for sub-agents would enable interactive delegation without waiting for full completion.

**Implementation sketch:**
- Sub-agent calls `request_input` with a question
- Parent's delegate tool receives the question via channel
- Parent LLM answers, response is injected into child's chat
- Child continues with the new information

**Complexity:** High — requires the parent's ReAct loop to handle mid-delegation interactions.

## 2. Task Board Improvements

### 2.1 Task Lifecycle Events / Subscriptions

**Inspiration:** A2A task lifecycle (`submitted -> working -> input-required -> completed/failed/canceled`)

The current `WatchCompleted` only blocks on terminal states. Adding a `Subscribe(id) <-chan TaskEvent` that emits on any state change (status transitions, metadata updates, reassignments) would enable reactive orchestration rather than poll-or-block.

### 2.2 Task Cancellation

**Inspiration:** A2A `canceled` state

There is no way to cancel an in-progress task. Adding:
- `Cancel(id) error` on the Store
- A `canceled` Status constant
- Context cancellation propagation to the running agent

This would let orchestrators abort work that is no longer needed (e.g., a parallel search where one branch already found the answer).

### 2.3 Task Priorities

The current store has no priority concept. When multiple agents compete for pending tasks, there is no way to signal urgency. A simple `Priority int` field plus ordered `List` results would improve scheduling in fan-out scenarios.

### 2.4 Typed Task Artifacts / Results

**Inspiration:** A2A Artifacts

Tasks carry only `Metadata map[string]any`. A2A has dedicated Artifacts — typed outputs with MIME types and streaming support. Adding a structured `Result` field (or an `Artifacts []Artifact` slice) to tasks would let workers attach rich outputs that downstream dependents can consume without relying on the state store as a side channel.

```go
type Artifact struct {
    Name     string          `json:"name"`
    MimeType string          `json:"mime_type"`
    Data     json.RawMessage `json:"data"`
}
```

## 3. State Store Improvements

### 3.1 Namespaced / Scoped State

The current store is a flat keyspace. As agent count grows, collisions become likely. Adding namespace prefixes (per-agent or per-task-group) or hierarchical keys would improve isolation and discoverability.

Options:
- Convention-based: `{agent}/{key}` naming
- API-level: `store.Scoped(namespace)` returns a prefixed view

### 3.2 Conditional Watch

`Watch(ctx, key)` only waits for key existence. Supporting conditional watches — `WatchUntil(ctx, key, func(json.RawMessage) bool)` — would eliminate busy-polling patterns where an agent watches for a key then checks if the value meets some condition.

### 3.3 TTL / Expiry

Long-running sessions accumulate stale state. Adding optional TTL on keys would allow automatic cleanup:

```go
store.SetWithTTL("key", value, 5*time.Minute)
```

## 4. Cross-Cutting Improvements

### 4.1 OpenTelemetry Tracing

**Inspiration:** OTel GenAI Semantic Conventions (v1.37+), AG2 OpenTelemetry integration

This is the biggest gap. Shelly has `EventFunc` and `EventNotifier` but no structured tracing. Adding OpenTelemetry spans would enable:

- A trace ID per top-level request, propagated through delegation chains
- Spans for each agent run, tool call, LLM completion
- Standard attributes: `gen_ai.request.model`, token counts, agent name, task ID
- Visualization of the full delegation tree
- Latency analysis and cost attribution per sub-agent
- Debugging multi-agent failures with distributed tracing tools (Jaeger, Grafana Tempo)

**Implementation approach:**
- Wrap `agent.Run()` in a span
- Wrap each tool call in a child span
- Propagate trace context through delegation via `context.Context`
- Use OTel's GenAI semantic conventions for LLM-specific attributes

### 4.2 Token Budget Propagation

**Inspiration:** Anthropic multi-agent research blog (agents spawning 50 sub-agents, searching endlessly)

Shelly's `MaxIterations` limits depth but not cost. Propagating a token/cost budget from parent to child (and tracking against it in effects) would prevent runaway spending in deep delegation trees.

**Implementation sketch:**
- `Options.TokenBudget int` — max tokens this agent (and its children) may consume
- Parent splits its remaining budget across children
- An effect checks cumulative usage against budget each iteration
- Children inherit a fraction of the parent's remaining budget

### 4.3 Direct Message Channels Between Peers

**Inspiration:** CrewAI Flows, LangGraph graph edges

Currently agents can only communicate through: delegation results, shared state store, task board metadata, or notes files. There is no direct message passing between peer agents.

A lightweight typed message channel between agents in the same session could reduce coupling to the state store for real-time coordination:

```go
type MessageChannel struct {
    Send(ctx context.Context, to string, msg json.RawMessage) error
    Receive(ctx context.Context) (from string, msg json.RawMessage, err error)
}
```

Exposed as `send_message` / `receive_message` tools.

### 4.4 Graph-Based Orchestration Mode

**Inspiration:** LangGraph

LangGraph's key insight is that many multi-agent workflows are better expressed as directed graphs than as free-form delegation. Adding an optional graph/pipeline mode alongside the current dynamic delegation would support deterministic workflows where the orchestrator does not need LLM reasoning to decide routing.

```yaml
pipeline:
  - agent: researcher
    next:
      - agent: coder
        condition: "research.status == 'completed'"
      - agent: fallback
        condition: "research.status == 'failed'"
```

This would complement (not replace) the existing dynamic delegation for cases where the workflow is known in advance.

### 4.5 Retry / Circuit Breaker for Delegation

When a child agent fails, the parent currently gets a failed `CompletionResult` and must decide what to do. Built-in retry with backoff (configurable per-agent) and circuit breaker (stop delegating to an agent that keeps failing) would improve resilience without burdening the orchestrator LLM with retry logic.

**Implementation sketch:**
- `RetryPolicy` on registry entries: max retries, backoff strategy
- Circuit breaker state per agent config name: open after N consecutive failures, half-open after cooldown
- Transparent to the orchestrator — retries happen inside the `delegate` tool

## Priority Assessment

| Improvement | Impact | Complexity | Priority |
|---|---|---|---|
| Task cancellation | High | Low | P0 |
| OpenTelemetry tracing | High | Medium | P0 |
| Streaming delegation | High | Medium | P1 |
| Token budget propagation | High | Medium | P1 |
| Rich agent cards | Medium | Low | P1 |
| Task lifecycle events | Medium | Medium | P1 |
| Retry / circuit breaker | Medium | Low | P2 |
| Namespaced state | Medium | Low | P2 |
| Typed task artifacts | Medium | Medium | P2 |
| Peer handoff | Medium | High | P2 |
| Conditional watch | Low | Low | P3 |
| State TTL | Low | Low | P3 |
| Child-to-parent questions | Medium | High | P3 |
| Direct message channels | Medium | High | P3 |
| Graph-based orchestration | Medium | High | P3 |
| Task priorities | Low | Low | P3 |

## Sources

- [How Anthropic built their multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system)
- [Google A2A Protocol Specification](https://a2a-protocol.org/latest/specification/)
- [A2A Streaming & Async Operations](https://a2a-protocol.org/latest/topics/streaming-and-async/)
- [OpenAI Agents SDK — Multi-agent orchestration](https://openai.github.io/openai-agents-python/multi_agent/)
- [OpenTelemetry AI Agent Observability](https://opentelemetry.io/blog/2025/ai-agent-observability/)
- [OTel Semantic Conventions for Agentic Systems](https://github.com/open-telemetry/semantic-conventions/issues/2664)
- [Google ADK multi-agent patterns](https://developers.googleblog.com/developers-guide-to-multi-agent-patterns-in-adk/)
- [CrewAI vs LangGraph vs AutoGen comparison](https://www.datacamp.com/tutorial/crewai-vs-langgraph-vs-autogen)
- [AG2 OpenTelemetry Tracing](https://docs.ag2.ai/latest/docs/blog/2026/02/08/AG2-OpenTelemetry-Tracing/)
- [Multi-Agent Systems 2026 Guide](https://dev.to/eira-wexford/how-to-build-multi-agent-systems-complete-2026-guide-1io6)
