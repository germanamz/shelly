package modeladapter

import (
	"context"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Completer sends a conversation to an LLM and returns the assistant's reply.
// The tools parameter declares which tools are available for this call.
type Completer interface {
	Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)
}

// UsageReporter provides token usage information from a completer.
type UsageReporter interface {
	UsageTracker() *usage.Tracker
	ModelMaxTokens() int
}
