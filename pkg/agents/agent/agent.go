// Package agent defines the Agent type that orchestrates a ModelAdapter, ToolBoxes,
// and a Chat into a cohesive unit for LLM interactions.
package agent

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/content"
	"github.com/germanamz/shelly/pkg/chatty/message"
	"github.com/germanamz/shelly/pkg/chatty/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Agent orchestrates a ModelAdapter, ToolBoxes, and a Chat. It sends conversations
// to the model adapter for completion and executes tool calls across its toolboxes.
// Agent is not safe for concurrent use; callers must synchronize externally.
type Agent struct {
	Name         string
	ModelAdapter modeladapter.Completer
	ToolBoxes    []*toolbox.ToolBox
	Chat         *chat.Chat
}

// New creates an Agent with the given name, model adapter, chat, and optional toolboxes.
func New(name string, a modeladapter.Completer, c *chat.Chat, tbs ...*toolbox.ToolBox) *Agent {
	return &Agent{
		Name:         name,
		ModelAdapter: a,
		ToolBoxes:    tbs,
		Chat:         c,
	}
}

// Complete sends the chat to the model adapter and appends the reply to the
// conversation. The reply's Sender is set to the agent's Name.
func (a *Agent) Complete(ctx context.Context) (message.Message, error) {
	reply, err := a.ModelAdapter.Complete(ctx, a.Chat)
	if err != nil {
		return message.Message{}, err
	}

	reply.Sender = a.Name
	a.Chat.Append(reply)

	return reply, nil
}

// CallTools executes all tool calls in the given message and appends the
// results to the chat. It searches each ToolBox in order for the named tool.
// Returns nil if the message contains no tool calls.
func (a *Agent) CallTools(ctx context.Context, msg message.Message) []content.ToolResult {
	calls := msg.ToolCalls()
	if len(calls) == 0 {
		return nil
	}

	results := make([]content.ToolResult, 0, len(calls))

	for _, tc := range calls {
		result := a.callTool(ctx, tc)
		results = append(results, result)
		a.Chat.Append(message.New(a.Name, role.Tool, result))
	}

	return results
}

// Tools returns all tools from all registered ToolBoxes.
func (a *Agent) Tools() []toolbox.Tool {
	var tools []toolbox.Tool

	for _, tb := range a.ToolBoxes {
		tools = append(tools, tb.Tools()...)
	}

	return tools
}

// callTool searches all ToolBoxes for the named tool and executes it.
func (a *Agent) callTool(ctx context.Context, tc content.ToolCall) content.ToolResult {
	for _, tb := range a.ToolBoxes {
		if _, ok := tb.Get(tc.Name); ok {
			return tb.Call(ctx, tc)
		}
	}

	return content.ToolResult{
		ToolCallID: tc.ID,
		Content:    fmt.Sprintf("tool not found: %s", tc.Name),
		IsError:    true,
	}
}
