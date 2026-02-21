# reactor

Package `reactor` orchestrates one or more agents over a shared conversation as a **team**. Each agent is wrapped in a `TeamMember` with a `TeamRole`, and a `Coordinator` decides which members to run on each step — including concurrent execution when multiple members are selected.

## Architecture

### Core Types

- **`NamedAgent`** — interface extending `agents.Agent` with `AgentName()` and `AgentChat()` accessors. Any type embedding `agents.AgentBase` satisfies this automatically.
- **`TeamRole`** — typed string identifying an agent's function (e.g. `"research"`, `"write"`). No predefined constants — users define their own.
- **`TeamMember`** — pairs a `NamedAgent` with a `TeamRole`.
- **`Selection`** — the coordinator's decision: which member indices to run, and whether orchestration is done.
- **`Coordinator`** — interface with `Next(ctx, *chat.Chat, []TeamMember) (Selection, error)` that decides which members act next.
- **`Reactor`** — the orchestrator. Implements both `agents.Agent` and `NamedAgent`, making reactors composable and nestable.

### Message Sync Algorithm

Before each agent turn:
1. Fetch new messages from shared chat since the agent's cursor position.
2. Skip messages where `Sender == agent.AgentName()` (already in private chat).
3. Remap role to `User`, preserving `Sender`, `Parts`, and `Metadata`.
4. Append to the agent's private chat and advance the cursor.

After the agent runs, its final reply is appended to the shared chat. Intermediate tool calls and reasoning stay in the agent's private chat.

### Concurrent Execution

When a coordinator returns multiple member indices in a single `Selection`, the reactor runs them concurrently:
1. All selected agents are synced before any goroutine launches.
2. Each agent runs in its own goroutine.
3. If any agent errors, the context is cancelled for the rest.
4. Replies are appended to the shared chat in selection order (deterministic).

## Built-in Coordinators

- **`NewSequence()`** — runs each member exactly once in order, then signals done.
- **`NewLoop(maxRounds)`** — round-robin cycling. Returns `ErrMaxRounds` when the limit is hit. Zero means unlimited.
- **`NewRoundRobinUntil(maxRounds, predicate)`** — like Loop but stops early when `predicate(chat)` returns true.
- **`NewRoleRoundRobin(maxRounds, roles...)`** — cycles through an ordered list of roles. Each step selects all members matching the current role, enabling concurrent execution for shared roles.

## Usage

```go
shared := chat.New(message.NewText("user", role.User, "Summarize this document"))

researcher := react.New(agents.NewAgentBase("researcher", llm, chat.New(), tools...), react.Options{})
writer := react.New(agents.NewAgentBase("writer", llm, chat.New()), react.Options{})

r, err := reactor.New("pipeline", shared, []reactor.TeamMember{
    {Agent: researcher, Role: "research"},
    {Agent: writer, Role: "write"},
}, reactor.Options{
    Coordinator: reactor.NewRoleRoundRobin(1, "research", "write"),
})

result, err := r.Run(ctx)
```

## Dependencies

- `pkg/agents` — `Agent` interface, `AgentBase` (provides `AgentName()`/`AgentChat()`)
- `pkg/chats/chat` — shared and private conversation containers
- `pkg/chats/message` — message types
- `pkg/chats/role` — role constants for remapping
