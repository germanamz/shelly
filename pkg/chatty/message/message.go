// Package message defines the Message type used in LLM conversations.
package message

import (
	"strings"

	"github.com/germanamz/shelly/pkg/chatty/content"
	"github.com/germanamz/shelly/pkg/chatty/role"
)

// Message represents a single message in a conversation.
// It is a value type that copies cheaply.
type Message struct {
	Sender   string
	Role     role.Role
	Parts    []content.Part
	Metadata map[string]any
}

// New creates a message with the given sender, role, and content parts.
func New(sender string, r role.Role, parts ...content.Part) Message {
	return Message{
		Sender: sender,
		Role:   r,
		Parts:  parts,
	}
}

// NewText creates a message with a single Text content part.
func NewText(sender string, r role.Role, text string) Message {
	return New(sender, r, content.Text{Text: text})
}

// TextContent concatenates the text of all Text parts in the message.
func (m Message) TextContent() string {
	var b strings.Builder
	for _, p := range m.Parts {
		if t, ok := p.(content.Text); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// ToolCalls returns all ToolCall parts in the message.
func (m Message) ToolCalls() []content.ToolCall {
	var calls []content.ToolCall
	for _, p := range m.Parts {
		if tc, ok := p.(content.ToolCall); ok {
			calls = append(calls, tc)
		}
	}
	return calls
}

// SetMeta sets a metadata key-value pair on the message.
// It initializes the Metadata map if nil.
func (m *Message) SetMeta(key string, value any) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata[key] = value
}

// GetMeta retrieves a metadata value by key.
func (m Message) GetMeta(key string) (any, bool) {
	if m.Metadata == nil {
		return nil, false
	}
	v, ok := m.Metadata[key]
	return v, ok
}
