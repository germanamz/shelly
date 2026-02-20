package agents

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Agent is the interface implemented by all agent types. It provides a single
// Run method that drives the agent's execution loop and returns the final message.
type Agent interface {
	Run(ctx context.Context) (message.Message, error)
}

// AgentBase provides shared functionality for agent types. It orchestrates a
// ModelAdapter, ToolBoxes, and a Chat. Embed AgentBase in concrete agent structs
// to inherit Complete, CallTools, and Tools methods.
// AgentBase is not safe for concurrent use; callers must synchronize externally.
type AgentBase struct {
	Name         string
	ModelAdapter modeladapter.Completer
	ToolBoxes    []*toolbox.ToolBox
	Chat         *chat.Chat
}

// NewAgentBase creates an AgentBase with the given name, model adapter, chat, and optional toolboxes.
func NewAgentBase(name string, a modeladapter.Completer, c *chat.Chat, tbs ...*toolbox.ToolBox) AgentBase {
	return AgentBase{
		Name:         name,
		ModelAdapter: a,
		ToolBoxes:    tbs,
		Chat:         c,
	}
}

// Complete sends the chat to the model adapter and appends the reply to the
// conversation. The reply's Sender is set to the base's Name.
func (b *AgentBase) Complete(ctx context.Context) (message.Message, error) {
	reply, err := b.ModelAdapter.Complete(ctx, b.Chat)
	if err != nil {
		return message.Message{}, err
	}

	reply.Sender = b.Name
	b.Chat.Append(reply)

	return reply, nil
}

// CallTools executes all tool calls in the given message and appends the
// results to the chat. It searches each ToolBox in order for the named tool.
// Returns nil if the message contains no tool calls.
func (b *AgentBase) CallTools(ctx context.Context, msg message.Message) []content.ToolResult {
	calls := msg.ToolCalls()
	if len(calls) == 0 {
		return nil
	}

	results := make([]content.ToolResult, 0, len(calls))

	for _, tc := range calls {
		result := b.callTool(ctx, tc)
		results = append(results, result)
		b.Chat.Append(message.New(b.Name, role.Tool, result))
	}

	return results
}

// Tools returns all tools from all registered ToolBoxes.
func (b *AgentBase) Tools() []toolbox.Tool {
	var tools []toolbox.Tool

	for _, tb := range b.ToolBoxes {
		tools = append(tools, tb.Tools()...)
	}

	return tools
}

// callTool searches all ToolBoxes for the named tool and executes it.
func (b *AgentBase) callTool(ctx context.Context, tc content.ToolCall) content.ToolResult {
	for _, tb := range b.ToolBoxes {
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
