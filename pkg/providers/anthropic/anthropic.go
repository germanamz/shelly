// Package anthropic provides a Completer implementation for the Anthropic Messages API.
package anthropic

import (
	"context"
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

const messagesPath = "/v1/messages"

var (
	_ modeladapter.Completer             = (*Adapter)(nil)
	_ modeladapter.UsageReporter         = (*Adapter)(nil)
	_ modeladapter.RateLimitInfoReporter = (*Adapter)(nil)
)

// Adapter implements modeladapter.Completer for the Anthropic Messages API.
type Adapter struct {
	client *modeladapter.Client
	Config modeladapter.ModelConfig
	usage  usage.Tracker
}

// New creates an Adapter configured for the Anthropic API.
// The baseURL should be "https://api.anthropic.com" (no trailing slash).
func New(baseURL, apiKey, model string) *Adapter {
	return &Adapter{
		client: modeladapter.NewClient(baseURL,
			modeladapter.Auth{Key: apiKey, Header: "x-api-key"},
			modeladapter.WithHeaders(map[string]string{"anthropic-version": "2023-06-01"}),
			modeladapter.WithHeaderParser(modeladapter.ParseAnthropicRateLimitHeaders)),
		Config: modeladapter.ModelConfig{
			Name:      model,
			MaxTokens: 4096,
		},
	}
}

// UsageTracker returns the adapter's token usage tracker.
func (a *Adapter) UsageTracker() *usage.Tracker { return &a.usage }

// ModelMaxTokens returns the maximum tokens the model will generate per response.
func (a *Adapter) ModelMaxTokens() int { return a.Config.MaxTokens }

// LastRateLimitInfo returns the most recently observed rate limit info, or nil.
func (a *Adapter) LastRateLimitInfo() *modeladapter.RateLimitInfo {
	return a.client.LastRateLimitInfo()
}

// Complete sends a conversation to the Anthropic Messages API and returns the
// assistant's reply.
func (a *Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	req := a.buildRequest(c, tools)

	var resp apiResponse
	if err := a.client.PostJSON(ctx, messagesPath, req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("anthropic: %w", err)
	}

	a.usage.Add(usage.TokenCount{
		InputTokens:              resp.Usage.InputTokens,
		OutputTokens:             resp.Usage.OutputTokens,
		CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
	})

	return a.parseResponse(resp), nil
}

// --- request types ---

type cacheControl struct {
	Type string `json:"type"`
}

type apiRequest struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       string        `json:"system,omitempty"`
	Messages     []apiMessage  `json:"messages"`
	Temperature  *float64      `json:"temperature,omitempty"`
	Tools        []apiToolDef  `json:"tools,omitempty"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type apiMessage struct {
	Role    string       `json:"role"`
	Content []apiContent `json:"content"`
}

type apiContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Source    *apiSource      `json:"source,omitempty"`
}

type apiSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type apiToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// --- response types ---

type apiResponse struct {
	Content    []apiContent `json:"content"`
	StopReason string       `json:"stop_reason"`
	Usage      apiUsage     `json:"usage"`
}

type apiUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// --- conversion helpers ---

func (a *Adapter) buildRequest(c *chat.Chat, tools []toolbox.Tool) apiRequest {
	req := apiRequest{
		Model:     a.Config.Name,
		MaxTokens: a.Config.MaxTokens,
		System:    c.SystemPrompt(),
	}

	if a.Config.Temperature != 0 {
		t := a.Config.Temperature
		req.Temperature = &t
	}

	req.CacheControl = &cacheControl{Type: "ephemeral"}

	if len(tools) > 0 {
		req.Tools = make([]apiToolDef, len(tools))
		for i, t := range tools {
			schema := t.InputSchema
			if schema == nil {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			req.Tools[i] = apiToolDef{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
			}
		}
	}

	msgs := c.Messages()
	for _, m := range msgs {
		if m.Role == role.System {
			continue
		}
		a.appendMessage(&req.Messages, m)
	}

	return req
}

func (a *Adapter) appendMessage(msgs *[]apiMessage, m message.Message) {
	for _, p := range m.Parts {
		block := partToBlock(p)
		if block == nil {
			continue
		}

		msgRole := mapRole(m.Role)

		// Tool results must be in a "user" role message per Anthropic API.
		if _, ok := p.(content.ToolResult); ok {
			msgRole = "user"
		}

		// Merge into the last message if it has the same role.
		if len(*msgs) > 0 && (*msgs)[len(*msgs)-1].Role == msgRole {
			(*msgs)[len(*msgs)-1].Content = append((*msgs)[len(*msgs)-1].Content, *block)
			continue
		}

		*msgs = append(*msgs, apiMessage{
			Role:    msgRole,
			Content: []apiContent{*block},
		})
	}
}

func partToBlock(p content.Part) *apiContent {
	switch v := p.(type) {
	case content.Text:
		return &apiContent{Type: "text", Text: v.Text}
	case content.ToolCall:
		input := json.RawMessage(v.Arguments)
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		return &apiContent{Type: "tool_use", ID: v.ID, Name: v.Name, Input: input}
	case content.Image:
		return &apiContent{
			Type: "image",
			Source: &apiSource{
				Type:      "base64",
				MediaType: v.MediaType,
				Data:      base64.StdEncoding.EncodeToString(v.Data),
			},
		}
	case content.ToolResult:
		return &apiContent{Type: "tool_result", ToolUseID: v.ToolCallID, Content: v.Content, IsError: v.IsError}
	default:
		return nil
	}
}

func mapRole(r role.Role) string {
	switch r {
	case role.Assistant:
		return "assistant"
	case role.Tool:
		return "user"
	default:
		return "user"
	}
}

func (a *Adapter) parseResponse(resp apiResponse) message.Message {
	var parts []content.Part

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			parts = append(parts, content.Text{Text: block.Text})
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			parts = append(parts, content.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	return message.New("", role.Assistant, parts...)
}
