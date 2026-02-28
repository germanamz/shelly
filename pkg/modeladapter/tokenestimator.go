package modeladapter

import (
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// perMessageOverhead is the estimated token overhead for each message (role,
// structure delimiters, etc.).
const perMessageOverhead = 4

// perToolOverhead is the estimated token overhead for each tool definition
// (JSON wrapping, function object structure, etc.).
const perToolOverhead = 10

// TokenEstimator estimates token counts for chat messages and tool definitions.
// It uses a character-to-token heuristic (approximately 1 token per 4 characters
// for English text, with overhead for JSON structure in tool definitions).
// The zero value is ready to use.
type TokenEstimator struct{}

// charsToTokens converts a character count to an estimated token count using the
// 1-token-per-4-characters heuristic.
func charsToTokens(chars int) int {
	return (chars + 3) / 4 // round up
}

// EstimateChat estimates the total input tokens for a chat conversation.
// It accounts for the system prompt, each message's text content, tool calls,
// tool results, and per-message structural overhead.
func (e *TokenEstimator) EstimateChat(c *chat.Chat) int {
	tokens := 0

	// System prompt.
	if sp := c.SystemPrompt(); sp != "" {
		tokens += charsToTokens(len(sp)) + perMessageOverhead
	}

	c.Each(func(_ int, m message.Message) bool {
		if m.Role == role.System {
			return true // already counted above as system prompt
		}

		tokens += perMessageOverhead

		for _, p := range m.Parts {
			switch v := p.(type) {
			case content.Text:
				tokens += charsToTokens(len(v.Text))
			case content.ToolCall:
				tokens += charsToTokens(len(v.ID) + len(v.Name) + len(v.Arguments))
			case content.ToolResult:
				tokens += charsToTokens(len(v.ToolCallID) + len(v.Content))
			}
		}

		return true
	})

	return tokens
}

// EstimateTools estimates the token cost of tool definitions. For each tool it
// sums the name, description, and serialized input schema, then applies the
// character-to-token heuristic plus a per-tool structural overhead.
func (e *TokenEstimator) EstimateTools(tools []toolbox.Tool) int {
	tokens := 0

	for _, t := range tools {
		chars := len(t.Name) + len(t.Description) + len(t.InputSchema)
		tokens += charsToTokens(chars) + perToolOverhead
	}

	return tokens
}

// EstimateTotal estimates total input tokens for a chat conversation combined
// with tool definitions. This is the primary entry point for pre-call estimation.
func (e *TokenEstimator) EstimateTotal(c *chat.Chat, tools []toolbox.Tool) int {
	return e.EstimateChat(c) + e.EstimateTools(tools)
}
