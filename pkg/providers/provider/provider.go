package provider

import (
	"context"

	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/message"
)

// Provider sends a conversation to an LLM and returns the assistant's reply.
type Provider interface {
	Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}
