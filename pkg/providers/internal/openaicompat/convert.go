package openaicompat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// BuildRequest constructs a Request from a model config, chat, and tool list.
func BuildRequest(cfg modeladapter.ModelConfig, c *chat.Chat, tools []toolbox.Tool) Request {
	req := Request{
		Model:     cfg.Name,
		MaxTokens: cfg.MaxTokens,
		Messages:  ConvertMessages(c.Messages()),
		Tools:     ConvertTools(tools),
	}

	if cfg.Temperature != 0 {
		t := cfg.Temperature
		req.Temperature = &t
	}

	return req
}

// ConvertMessages converts internal messages to the OpenAI wire format.
func ConvertMessages(msgs []message.Message) []Message {
	var out []Message

	for _, m := range msgs {
		switch m.Role {
		case role.System, role.User:
			if hasImages(m) {
				out = append(out, convertMultiModalMessage(m))
			} else {
				text := m.TextContent()
				out = append(out, Message{
					Role:    m.Role.String(),
					Content: &text,
				})
			}

		case role.Assistant:
			am := Message{Role: role.Assistant.String()}

			if text := m.TextContent(); text != "" {
				am.Content = &text
			}

			for _, tc := range m.ToolCalls() {
				am.ToolCalls = append(am.ToolCalls, ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: ToolFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}

			out = append(out, am)

		case role.Tool:
			for _, p := range m.Parts {
				tr, ok := p.(content.ToolResult)
				if !ok {
					continue
				}

				out = append(out, Message{
					Role:       role.Tool.String(),
					Content:    &tr.Content,
					ToolCallID: tr.ToolCallID,
				})
			}
		}
	}

	return out
}

// ConvertTools converts toolbox tools to the OpenAI wire format.
func ConvertTools(tools []toolbox.Tool) []ToolDef {
	if len(tools) == 0 {
		return nil
	}

	defs := make([]ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = MarshalToolDef(t.Name, t.Description, t.InputSchema)
	}

	return defs
}

// MarshalToolDef creates a ToolDef from a name, description, and JSON schema.
func MarshalToolDef(name, description string, schema json.RawMessage) ToolDef {
	if schema == nil {
		schema = json.RawMessage(`{"type":"object"}`)
	}

	return ToolDef{
		Type: "function",
		Function: ToolDefFunc{
			Name:        name,
			Description: description,
			Parameters:  schema,
		},
	}
}

// ParseMessage converts an API message to an internal message.
func ParseMessage(m Message) message.Message {
	var parts []content.Part

	if m.Content != nil && *m.Content != "" {
		parts = append(parts, content.Text{Text: *m.Content})
	}

	for _, tc := range m.ToolCalls {
		parts = append(parts, content.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return message.New("", role.Assistant, parts...)
}

// hasImages returns true if the message contains any Image parts.
func hasImages(m message.Message) bool {
	for _, p := range m.Parts {
		if _, ok := p.(content.Image); ok {
			return true
		}
	}
	return false
}

// convertMultiModalMessage converts a message with Image parts into a Message
// using the ContentParts array format required by OpenAI for multi-modal input.
func convertMultiModalMessage(m message.Message) Message {
	msg := Message{Role: m.Role.String()}
	for _, p := range m.Parts {
		switch v := p.(type) {
		case content.Text:
			msg.ContentParts = append(msg.ContentParts, ContentPart{
				Type: "text",
				Text: v.Text,
			})
		case content.Image:
			dataURI := fmt.Sprintf("data:%s;base64,%s",
				v.MediaType, base64.StdEncoding.EncodeToString(v.Data))
			msg.ContentParts = append(msg.ContentParts, ContentPart{
				Type:     "image_url",
				ImageURL: &ImageURL{URL: dataURI},
			})
		}
	}
	return msg
}

// ParseUsage converts API usage to a TokenCount.
func ParseUsage(u Usage) usage.TokenCount {
	tc := usage.TokenCount{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}

	if u.PromptTokensDetails != nil {
		tc.CacheReadInputTokens = u.PromptTokensDetails.CachedTokens
	}

	return tc
}
