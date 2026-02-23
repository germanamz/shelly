// Package openai provides a Completer implementation for the OpenAI Chat Completions API.
package openai

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

const completionsPath = "/v1/chat/completions"

var _ modeladapter.Completer = (*Adapter)(nil)

// Adapter implements modeladapter.Completer for the OpenAI Chat Completions API.
type Adapter struct {
	modeladapter.ModelAdapter
}

// New creates an Adapter configured for the OpenAI API.
// The baseURL should be "https://api.openai.com" (no trailing slash).
func New(baseURL, apiKey, model string) *Adapter {
	a := &Adapter{}
	a.BaseURL = baseURL
	a.Auth = modeladapter.Auth{Key: apiKey}
	a.Name = model
	a.MaxTokens = 4096
	a.HeaderParser = modeladapter.ParseOpenAIRateLimitHeaders

	return a
}

// Complete sends a conversation to the OpenAI Chat Completions API and returns
// the assistant's reply.
func (a *Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	req := a.buildRequest(c, tools)

	var resp apiResponse
	if err := a.PostJSON(ctx, completionsPath, req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("openai: %w", err)
	}

	a.Usage.Add(usage.TokenCount{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	})

	if len(resp.Choices) == 0 {
		return message.Message{}, fmt.Errorf("openai: empty choices in response")
	}

	return a.parseChoice(resp.Choices[0]), nil
}

// --- request types ---

type apiRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	Tools       []apiToolDef `json:"tools,omitempty"`
}

type apiMessage struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type apiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function apiToolFunction `json:"function"`
}

type apiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type apiToolDef struct {
	Type     string         `json:"type"`
	Function apiToolDefFunc `json:"function"`
}

type apiToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// --- response types ---

type apiResponse struct {
	Choices []apiChoice `json:"choices"`
	Usage   apiUsage    `json:"usage"`
}

type apiChoice struct {
	Message      apiRespMessage `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type apiRespMessage struct {
	Role      string        `json:"role"`
	Content   *string       `json:"content"`
	ToolCalls []apiToolCall `json:"tool_calls,omitempty"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- conversion helpers ---

func (a *Adapter) buildRequest(c *chat.Chat, tools []toolbox.Tool) apiRequest {
	req := apiRequest{
		Model:     a.Name,
		MaxTokens: a.MaxTokens,
	}

	if a.Temperature != 0 {
		t := a.Temperature
		req.Temperature = &t
	}

	if len(tools) > 0 {
		req.Tools = make([]apiToolDef, len(tools))
		for i, t := range tools {
			schema := t.InputSchema
			if schema == nil {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			req.Tools[i] = apiToolDef{
				Type: "function",
				Function: apiToolDefFunc{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  schema,
				},
			}
		}
	}

	msgs := c.Messages()
	for _, m := range msgs {
		a.appendMessages(&req.Messages, m)
	}

	return req
}

func (a *Adapter) appendMessages(msgs *[]apiMessage, m message.Message) {
	switch m.Role {
	case role.System:
		text := m.TextContent()
		*msgs = append(*msgs, apiMessage{Role: "system", Content: &text})

	case role.User:
		text := m.TextContent()
		*msgs = append(*msgs, apiMessage{Role: "user", Content: &text})

	case role.Assistant:
		var toolCalls []apiToolCall
		var textParts []string

		for _, p := range m.Parts {
			switch v := p.(type) {
			case content.Text:
				textParts = append(textParts, v.Text)
			case content.ToolCall:
				toolCalls = append(toolCalls, apiToolCall{
					ID:   v.ID,
					Type: "function",
					Function: apiToolFunction{
						Name:      v.Name,
						Arguments: v.Arguments,
					},
				})
			}
		}

		msg := apiMessage{Role: "assistant"}
		if len(textParts) > 0 {
			joined := ""
			for _, t := range textParts {
				joined += t
			}
			msg.Content = &joined
		}
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
		}

		*msgs = append(*msgs, msg)

	case role.Tool:
		for _, p := range m.Parts {
			if tr, ok := p.(content.ToolResult); ok {
				*msgs = append(*msgs, apiMessage{
					Role:       "tool",
					Content:    &tr.Content,
					ToolCallID: tr.ToolCallID,
				})
			}
		}
	}
}

func (a *Adapter) parseChoice(choice apiChoice) message.Message {
	var parts []content.Part

	if choice.Message.Content != nil && *choice.Message.Content != "" {
		parts = append(parts, content.Text{Text: *choice.Message.Content})
	}

	for _, tc := range choice.Message.ToolCalls {
		parts = append(parts, content.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return message.New("", role.Assistant, parts...)
}
