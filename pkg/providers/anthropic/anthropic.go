// Package anthropic provides a Completer implementation for the Anthropic Messages API.
package anthropic

import (
	"context"
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
	_ modeladapter.Completer = (*Adapter)(nil)
	_ modeladapter.ToolAware = (*Adapter)(nil)
)

// Adapter implements modeladapter.Completer for the Anthropic Messages API.
type Adapter struct {
	modeladapter.ModelAdapter
	Tools []toolbox.Tool
}

// New creates an Adapter configured for the Anthropic API.
// The baseURL should be "https://api.anthropic.com" (no trailing slash).
func New(baseURL, apiKey, model string) *Adapter {
	a := &Adapter{}
	a.BaseURL = baseURL
	a.Auth = modeladapter.Auth{
		Key:    apiKey,
		Header: "x-api-key",
	}
	a.Name = model
	a.MaxTokens = 4096
	a.Headers = map[string]string{
		"anthropic-version": "2023-06-01",
	}

	return a
}

// SetTools sets the tools that will be declared in API requests.
func (a *Adapter) SetTools(tools []toolbox.Tool) {
	a.Tools = tools
}

// Complete sends a conversation to the Anthropic Messages API and returns the
// assistant's reply.
func (a *Adapter) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
	req := a.buildRequest(c)

	var resp apiResponse
	if err := a.PostJSON(ctx, messagesPath, req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("anthropic: %w", err)
	}

	a.Usage.Add(usage.TokenCount{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	})

	return a.parseResponse(resp), nil
}

// --- request types ---

type apiRequest struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	System      string       `json:"system,omitempty"`
	Messages    []apiMessage `json:"messages"`
	Temperature *float64     `json:"temperature,omitempty"`
	Tools       []apiToolDef `json:"tools,omitempty"`
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
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- conversion helpers ---

func (a *Adapter) buildRequest(c *chat.Chat) apiRequest {
	req := apiRequest{
		Model:     a.Name,
		MaxTokens: a.MaxTokens,
		System:    c.SystemPrompt(),
	}

	if a.Temperature != 0 {
		t := a.Temperature
		req.Temperature = &t
	}

	if len(a.Tools) > 0 {
		req.Tools = make([]apiToolDef, len(a.Tools))
		for i, t := range a.Tools {
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
	case content.ToolResult:
		return &apiContent{Type: "tool_result", ToolUseID: v.ToolCallID, Content: v.Content}
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
