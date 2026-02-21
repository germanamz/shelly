// Package delegate wraps a NamedAgent as a toolbox.Tool, enabling the
// "delegation" (agent-as-tool) pattern. A parent agent can invoke a sub-agent
// through its normal tool-calling mechanism; the sub-agent runs its full
// reasoning loop privately and only the final text reply surfaces as the tool
// result.
package delegate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/reactor"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// defaultInputSchema is the JSON Schema used by NewAgentTool. It accepts a
// single required "task" string field describing what to delegate.
var defaultInputSchema = json.RawMessage(`{"type":"object","properties":{"task":{"type":"string","description":"The task to delegate"}},"required":["task"]}`)

// taskInput is the expected JSON structure for the default input schema.
type taskInput struct {
	Task string `json:"task"`
}

// AgentTool wraps a NamedAgent so it can be registered in a ToolBox and called
// like any other tool. Each invocation appends the caller's input as a user
// message into the sub-agent's private chat, runs the agent, and returns the
// final text reply.
type AgentTool struct {
	agent       reactor.NamedAgent
	description string
	inputSchema json.RawMessage
}

// NewAgentTool creates an AgentTool with a default input schema that expects a
// JSON object with a single "task" string field. The tool name is derived from
// the agent's name.
func NewAgentTool(agent reactor.NamedAgent, description string) *AgentTool {
	return &AgentTool{
		agent:       agent,
		description: description,
		inputSchema: defaultInputSchema,
	}
}

// Tool returns a toolbox.Tool that delegates work to the wrapped agent. The
// tool name matches the agent's name.
func (at *AgentTool) Tool() toolbox.Tool {
	return toolbox.Tool{
		Name:        at.agent.AgentName(),
		Description: at.description,
		InputSchema: at.inputSchema,
		Handler:     at.handle,
	}
}

// handle is the tool handler. It parses the JSON input, appends a user message
// to the sub-agent's chat, runs the agent, and returns the final text reply.
func (at *AgentTool) handle(ctx context.Context, input json.RawMessage) (string, error) {
	var ti taskInput
	if err := json.Unmarshal(input, &ti); err != nil {
		return "", fmt.Errorf("delegate: invalid input: %w", err)
	}

	at.agent.AgentChat().Append(message.NewText("user", role.User, ti.Task))

	reply, err := at.agent.Run(ctx)
	if err != nil {
		return "", err
	}

	return reply.TextContent(), nil
}
