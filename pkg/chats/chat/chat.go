// Package chat provides a mutable conversation container for LLM interactions.
package chat

import (
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// Chat is a mutable conversation container. The zero value is ready to use.
// Chat is not safe for concurrent use; callers must synchronize externally.
type Chat struct {
	messages []message.Message
}

// New creates a Chat pre-populated with the given messages.
func New(msgs ...message.Message) *Chat {
	return &Chat{messages: msgs}
}

// Append adds one or more messages to the conversation.
func (c *Chat) Append(msgs ...message.Message) {
	c.messages = append(c.messages, msgs...)
}

// Len returns the number of messages in the conversation.
func (c *Chat) Len() int {
	return len(c.messages)
}

// At returns the message at the given index.
// It panics if the index is out of range.
func (c *Chat) At(index int) message.Message {
	return c.messages[index]
}

// Last returns the most recent message and true, or a zero Message and false
// if the conversation is empty.
func (c *Chat) Last() (message.Message, bool) {
	if len(c.messages) == 0 {
		return message.Message{}, false
	}
	return c.messages[len(c.messages)-1], true
}

// Messages returns a copy of all messages in the conversation.
func (c *Chat) Messages() []message.Message {
	cp := make([]message.Message, len(c.messages))
	copy(cp, c.messages)
	return cp
}

// Each iterates over messages, calling fn for each one. If fn returns false,
// iteration stops early.
func (c *Chat) Each(fn func(int, message.Message) bool) {
	for i, m := range c.messages {
		if !fn(i, m) {
			return
		}
	}
}

// BySender returns all messages from the given sender.
func (c *Chat) BySender(sender string) []message.Message {
	var out []message.Message
	for _, m := range c.messages {
		if m.Sender == sender {
			out = append(out, m)
		}
	}
	return out
}

// SystemPrompt returns the text content of the first system message, or an
// empty string if there is none.
func (c *Chat) SystemPrompt() string {
	for _, m := range c.messages {
		if m.Role == role.System {
			return m.TextContent()
		}
	}
	return ""
}
