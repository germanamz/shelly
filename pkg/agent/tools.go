package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// orchestrationToolBox builds a ToolBox containing the built-in orchestration
// tools (list_agents, delegate_to_agent, spawn_agents) for the given agent.
func orchestrationToolBox(a *Agent) *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(
		listAgentsTool(a),
		delegateTool(a),
		spawnTool(a),
	)

	return tb
}

// --- list_agents ---

func listAgentsTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "list_agents",
		Description: "List all available agents that can be delegated to",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			entries := a.registry.List()

			// Filter out self.
			var filtered []Entry
			for _, e := range entries {
				if e.Name != a.name {
					filtered = append(filtered, e)
				}
			}

			data, err := json.Marshal(filtered)
			if err != nil {
				return "", fmt.Errorf("list_agents: %w", err)
			}

			return string(data), nil
		},
	}
}

// --- delegate_to_agent ---

type delegateInput struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

func delegateTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "delegate_to_agent",
		Description: "Delegate a task to another agent and get its response",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"agent":{"type":"string","description":"Name of the agent to delegate to"},"task":{"type":"string","description":"The task to delegate"}},"required":["agent","task"]}`),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var di delegateInput
			if err := json.Unmarshal(input, &di); err != nil {
				return "", fmt.Errorf("delegate_to_agent: invalid input: %w", err)
			}

			if di.Agent == a.name {
				return "", fmt.Errorf("delegate_to_agent: self-delegation is not allowed")
			}

			if a.options.MaxDelegationDepth > 0 && a.depth >= a.options.MaxDelegationDepth {
				return "", fmt.Errorf("delegate_to_agent: max delegation depth %d reached", a.options.MaxDelegationDepth)
			}

			child, ok := a.registry.Spawn(di.Agent, a.depth+1)
			if !ok {
				return "", fmt.Errorf("delegate_to_agent: agent %q not found", di.Agent)
			}

			child.registry = a.registry
			child.AddToolBoxes(a.toolboxes...)
			child.chat.Append(message.NewText("user", role.User, di.Task))

			reply, err := child.Run(ctx)
			if err != nil {
				return "", fmt.Errorf("delegate_to_agent: agent %q: %w", di.Agent, err)
			}

			return reply.TextContent(), nil
		},
	}
}

// --- spawn_agents ---

type spawnTask struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

type spawnInput struct {
	Tasks []spawnTask `json:"tasks"`
}

type spawnResult struct {
	Agent  string `json:"agent"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func spawnTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "spawn_agents",
		Description: "Spawn multiple agents concurrently and collect their results",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"agent":{"type":"string","description":"Name of the agent"},"task":{"type":"string","description":"The task to delegate"}},"required":["agent","task"]},"description":"List of agent tasks to run concurrently"}},"required":["tasks"]}`),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var si spawnInput
			if err := json.Unmarshal(input, &si); err != nil {
				return "", fmt.Errorf("spawn_agents: invalid input: %w", err)
			}

			if len(si.Tasks) == 0 {
				return "[]", nil
			}

			for _, t := range si.Tasks {
				if t.Agent == a.name {
					return "", fmt.Errorf("spawn_agents: self-delegation is not allowed")
				}
			}

			if a.options.MaxDelegationDepth > 0 && a.depth >= a.options.MaxDelegationDepth {
				return "", fmt.Errorf("spawn_agents: max delegation depth %d reached", a.options.MaxDelegationDepth)
			}

			results := make([]spawnResult, len(si.Tasks))

			var wg sync.WaitGroup
			wg.Add(len(si.Tasks))

			for i, t := range si.Tasks {
				go func() {
					defer wg.Done()

					child, ok := a.registry.Spawn(t.Agent, a.depth+1)
					if !ok {
						results[i] = spawnResult{
							Agent: t.Agent,
							Error: fmt.Sprintf("agent %q not found", t.Agent),
						}
						return
					}

					child.registry = a.registry
					child.AddToolBoxes(a.toolboxes...)
					child.chat.Append(message.NewText("user", role.User, t.Task))

					reply, err := child.Run(ctx)
					if err != nil {
						results[i] = spawnResult{
							Agent: t.Agent,
							Error: err.Error(),
						}
						return
					}

					results[i] = spawnResult{
						Agent:  t.Agent,
						Result: reply.TextContent(),
					}
				}()
			}

			wg.Wait()

			data, err := json.Marshal(results)
			if err != nil {
				return "", fmt.Errorf("spawn_agents: %w", err)
			}

			return string(data), nil
		},
	}
}
