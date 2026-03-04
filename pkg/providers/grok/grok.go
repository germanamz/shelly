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

// DefaultBaseURL is the base URL for the xAI API (without version prefix,
// consistent with the OpenAI and Anthropic providers).
const DefaultBaseURL = "https://api.x.ai"

var (
	_ modeladapter.Completer             = (*GrokAdapter)(nil)
	_ modeladapter.UsageReporter         = (*GrokAdapter)(nil)
	_ modeladapter.RateLimitInfoReporter = (*GrokAdapter)(nil)
)

// GrokAdapter sends chat completions to xAI's Grok API.
type GrokAdapter struct {
	client *modeladapter.Client
	Config modeladapter.ModelConfig
	usage  usage.Tracker
}

// New creates a GrokAdapter with the given base URL, API key, model, and HTTP client.
// A nil client falls back to a default HTTP client with a 10-minute timeout.
func New(baseURL, apiKey, model string, httpClient *http.Client) *GrokAdapter {
	opts := []modeladapter.ClientOption{
		modeladapter.WithHeaderParser(modeladapter.ParseOpenAIRateLimitHeaders),
	}
	if httpClient != nil {
		opts = append(opts, modeladapter.WithHTTPClient(httpClient))
	}
	return &GrokAdapter{
		client: modeladapter.NewClient(baseURL, modeladapter.Auth{Key: apiKey}, opts...),
		Config: modeladapter.ModelConfig{
			Name:      model,
			MaxTokens: 4096,
		},
	}
}

// UsageTracker returns the adapter's token usage tracker.
func (g *GrokAdapter) UsageTracker() *usage.Tracker { return &g.usage }

// ModelMaxTokens returns the maximum tokens the model will generate per response.
func (g *GrokAdapter) ModelMaxTokens() int { return g.Config.MaxTokens }

// LastRateLimitInfo returns the most recently observed rate limit info, or nil.
func (g *GrokAdapter) LastRateLimitInfo() *modeladapter.RateLimitInfo {
	return g.client.LastRateLimitInfo()
}

// Complete sends a conversation to the Grok chat completions endpoint
// and returns the assistant's reply.
func (g *GrokAdapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	req := chatRequest{
		Model:       g.Config.Name,
		Messages:    convertMessages(c),
		Temperature: g.Config.Temperature,
		MaxTokens:   g.Config.MaxTokens,
	}

	for _, t := range tools {
		schema := t.InputSchema
		if schema == nil {
			schema = json.RawMessage(`{"type":"object"}`)
		}

		req.Tools = append(req.Tools, MarshalToolDef(t.Name, t.Description, schema))
	}

	var resp chatResponse
	if err := g.client.PostJSON(ctx, "/v1/chat/completions", req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("grok: %w", err)
	}

	if len(resp.Choices) == 0 {
		return message.Message{}, fmt.Errorf("grok: empty response")
	}

	g.usage.Add(usage.TokenCount{
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
	Content    *string       `json:"content,omitempty"`
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
			text := m.TextContent()
			msgs = append(msgs, apiMessage{
				Role:    m.Role.String(),
				Content: &text,
			})
		case role.Assistant:
			am := apiMessage{
				Role: role.Assistant.String(),
			}

			if text := m.TextContent(); text != "" {
				am.Content = &text
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
					Content:    &tr.Content,
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

	if am.Content != nil && *am.Content != "" {
		parts = append(parts, content.Text{Text: *am.Content})
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
