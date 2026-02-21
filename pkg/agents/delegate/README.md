# delegate

Package `delegate` wraps a `NamedAgent` as a `toolbox.Tool`, enabling the **delegation (agent-as-tool)** pattern. A parent agent can invoke a sub-agent through its normal tool-calling mechanism; the sub-agent runs its full reasoning loop privately and only the final text reply surfaces as the tool result.

## Architecture

### Core Types

- **`AgentTool`** — holds a `NamedAgent`, a description, and an input schema. Its `Tool()` method returns a `toolbox.Tool` that can be registered in any `ToolBox`.
- **`NewAgentTool(agent, description)`** — convenience constructor with a default input schema accepting a single `"task"` string field.

### How It Works

1. The parent agent calls the delegation tool with a JSON input containing a `"task"` string.
2. The `AgentTool` handler appends the task as a user message to the sub-agent's private chat.
3. The sub-agent's `Run(ctx)` method executes its full loop (e.g., ReAct reasoning + tool calls).
4. Only the final text reply (`reply.TextContent()`) is returned as the tool result string.
5. All intermediate tool calls and reasoning stay in the sub-agent's private chat.

### Key Behaviors

- **Stateful by default** — the sub-agent retains its chat history across multiple calls, enabling multi-turn delegation.
- **Context propagation** — the parent's context flows through to the sub-agent, enabling cancellation.
- **Error passthrough** — if the sub-agent returns an error, it propagates to the parent as a tool error.

## Usage

```go
researcher := react.New(agents.NewAgentBase("researcher", llm, chat.New(), webTools), react.Options{})
delegateTool := delegate.NewAgentTool(researcher, "Delegate research tasks to a specialist")

parentToolBox := toolbox.New()
parentToolBox.Register(delegateTool.Tool())

parent := react.New(agents.NewAgentBase("coordinator", llm, chat.New(), parentToolBox), react.Options{})
result, err := parent.Run(ctx)
```

## Dependencies

- `pkg/reactor` — `NamedAgent` interface
- `pkg/tools/toolbox` — `Tool` and `Handler` types
- `pkg/chats/` — chat, message, role, and content types
