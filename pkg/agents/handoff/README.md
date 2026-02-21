# handoff

Package `handoff` enables agents to transfer control to each other mid-conversation. A `HandoffAgent` manages a set of named agents that share a single chat. Each agent receives `transfer_to_<name>` tools; calling one switches the active agent. The loop continues until an agent returns a final answer without triggering a handoff.

## Architecture

### Core Types

- **`HandoffAgent`** — orchestrates a group of agents over a shared chat. Implements `agents.Agent` and `reactor.NamedAgent`, making it composable with `Reactor`.
- **`HandoffError`** — sentinel error type used by transfer tool handlers to signal a handoff. The `Run` loop catches it to switch agents.
- **`AgentFactory`** — `func(shared *chat.Chat, transferTools *toolbox.ToolBox) reactor.NamedAgent`. Factory function that builds an agent with the shared chat and transfer tools injected.
- **`Member`** — pairs a name with an `AgentFactory`.
- **`Options`** — configuration including `MaxHandoffs` to prevent infinite loops.

### How It Works

1. `New` creates a `HandoffAgent`, builds transfer tools for all members, and calls each `AgentFactory` with the shared chat and transfer `ToolBox`.
2. The first member in the list becomes the initially active agent.
3. `Run` loops: the active agent runs its full reasoning loop.
4. If the agent calls a `transfer_to_<name>` tool, the handler returns a `HandoffError`.
5. The `Run` loop catches the `HandoffError`, switches the active agent, and continues.
6. When an agent returns normally (no handoff), its reply becomes the final result.
7. If `MaxHandoffs` is exceeded, `ErrMaxHandoffs` is returned.

### Shared Chat

Unlike `Reactor` (which gives each agent a private chat and syncs messages), `HandoffAgent` gives all agents the **same chat**. This means:
- Agents see each other's messages directly.
- Transfer tools inject naturally into the conversation flow.
- The `AgentFactory` pattern ensures each agent is constructed with the shared chat.

## Usage

```go
shared := chat.New(message.NewText("user", role.User, "Help me book a flight"))

h, err := handoff.New("travel-agent", shared, []handoff.Member{
    {Name: "triage", Factory: func(c *chat.Chat, tb *toolbox.ToolBox) reactor.NamedAgent {
        base := agents.NewAgentBase("triage", llm, c, tb, triageTools)
        return react.New(base, react.Options{})
    }},
    {Name: "booking", Factory: func(c *chat.Chat, tb *toolbox.ToolBox) reactor.NamedAgent {
        base := agents.NewAgentBase("booking", llm, c, tb, bookingTools)
        return react.New(base, react.Options{})
    }},
}, handoff.Options{MaxHandoffs: 10})

result, err := h.Run(ctx)
```

## Dependencies

- `pkg/agents` — `Agent` interface
- `pkg/reactor` — `NamedAgent` interface
- `pkg/tools/toolbox` — `ToolBox`, `Tool`, and `Handler` types
- `pkg/chats/` — chat, message, and content types
