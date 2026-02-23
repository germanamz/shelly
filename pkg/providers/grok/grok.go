// Package grok implements the modeladapter.Completer interface for xAI's Grok models
// using the OpenAI-compatible chat completions API.
package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// DefaultBaseURL is the base URL for the xAI API.
const DefaultBaseURL = "https://api.x.ai/v1"

// GrokAdapter sends chat completions to xAI's Grok API.
type GrokAdapter struct {
	modeladapter.ModelAdapter
}

// New creates a GrokAdapter with the given API key and HTTP client.
// A nil client falls back to http.DefaultClient.
func New(apiKey string, client *http.Client) *GrokAdapter {
	a := &GrokAdapter{
		ModelAdapter: modeladapter.New(DefaultBaseURL, modeladapter.Auth{Key: apiKey}, client),
	}
	a.HeaderParser = modeladapter.ParseOpenAIRateLimitHeaders
	return a
}

// Complete sends a conversation to the Grok chat completions endpoint
// and returns the assistant's reply.
func (g *GrokAdapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	req := chatRequest{
		Model:       g.Name,
		Messages:    convertMessages(c),
		Temperature: g.Temperature,
		MaxTokens:   g.MaxTokens,
	}

	for _, t := range tools {
		schema := t.InputSchema
		if schema == nil {
			schema = json.RawMessage(`{"type":"object"}`)
		}

		req.Tools = append(req.Tools, MarshalToolDef(t.Name, t.Description, schema))
	}

	var resp chatResponse
	if err := g.PostJSON(ctx, "/chat/completions", req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("grok: %w", err)
	}

	if len(resp.Choices) == 0 {
		return message.Message{}, fmt.Errorf("grok: empty response")
	}

	g.Usage.Add(usage.TokenCount{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	})

	return convertResponse(resp.Choices[0].Message), nil
}

// API request/response types.

type chatRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Tools       []apiTool    `json:"tools,omitempty"`
}

type apiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type apiToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function apiFunction `json:"function"`
}

type apiFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	ID      string   `json:"id"`
	Choices []choice `json:"choices"`
	Usage   apiUsage `json:"usage"`
}

type choice struct {
	Message      apiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// convertMessages transforms a Chat into the API message format.
func convertMessages(c *chat.Chat) []apiMessage {
	var msgs []apiMessage

	c.Each(func(_ int, m message.Message) bool {
		switch m.Role {
		case role.System, role.User:
			msgs = append(msgs, apiMessage{
				Role:    m.Role.String(),
				Content: m.TextContent(),
			})
		case role.Assistant:
			am := apiMessage{
				Role:    role.Assistant.String(),
				Content: m.TextContent(),
			}

			for _, tc := range m.ToolCalls() {
				am.ToolCalls = append(am.ToolCalls, apiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: apiFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}

			msgs = append(msgs, am)
		case role.Tool:
			for _, p := range m.Parts {
				tr, ok := p.(content.ToolResult)
				if !ok {
					continue
				}

				msgs = append(msgs, apiMessage{
					Role:       role.Tool.String(),
					Content:    tr.Content,
					ToolCallID: tr.ToolCallID,
				})
			}
		}

		return true
	})

	return msgs
}

// convertResponse transforms an API message into a chats Message.
func convertResponse(am apiMessage) message.Message {
	var parts []content.Part

	if am.Content != "" {
		parts = append(parts, content.Text{Text: am.Content})
	}

	for _, tc := range am.ToolCalls {
		parts = append(parts, content.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return message.New("", role.Assistant, parts...)
}

// verifyCompleter ensures GrokAdapter satisfies the Completer interface at compile time.
var _ modeladapter.Completer = (*GrokAdapter)(nil)

// MarshalToolDef converts a tool name, description, and JSON schema into the
// API tool format used in the chat request. This is a convenience for callers
// that need to attach tools to the request.
func MarshalToolDef(name, description string, schema json.RawMessage) apiTool {
	return apiTool{
		Type: "function",
		Function: apiToolDef{
			Name:        name,
			Description: description,
			Parameters:  schema,
		},
	}
}

type apiTool struct {
	Type     string     `json:"type"`
	Function apiToolDef `json:"function"`
}

type apiToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}
