// Package openaicompat provides shared wire types and conversion logic for
// providers that use OpenAI-compatible chat completion APIs (OpenAI, Grok, etc.).
package openaicompat

import (
	"encoding/json"
	"fmt"
)

// CompletionsPath is the standard OpenAI-compatible completions endpoint.
const CompletionsPath = "/v1/chat/completions"

// --- request types ---

// Request is the OpenAI-compatible chat completions request.
type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	Tools       []ToolDef `json:"tools,omitempty"`
}

// Message is a message in the OpenAI-compatible wire format.
// For multi-modal messages, ContentParts is used instead of Content.
type Message struct {
	Role         string        `json:"role"`
	Content      *string       `json:"-"`
	ContentParts []ContentPart `json:"-"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
}

// ContentPart is a part within a multi-modal content array.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
	File     *FileData `json:"file,omitempty"`
}

// ImageURL holds an image reference for multi-modal messages.
type ImageURL struct {
	URL string `json:"url"`
}

// FileData holds an inline file reference for multi-modal messages (e.g. PDF).
type FileData struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
}

// MarshalJSON implements custom JSON marshaling for Message.
// When ContentParts is set, the "content" field is serialized as an array;
// otherwise it is serialized as a string (or omitted if nil).
func (m Message) MarshalJSON() ([]byte, error) {
	type alias struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content,omitempty"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
	}

	a := alias{
		Role:       m.Role,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
	}

	switch {
	case len(m.ContentParts) > 0:
		b, err := json.Marshal(m.ContentParts)
		if err != nil {
			return nil, err
		}
		a.Content = b
	case m.Content != nil:
		b, err := json.Marshal(*m.Content)
		if err != nil {
			return nil, err
		}
		a.Content = b
	}

	return json.Marshal(a)
}

// UnmarshalJSON implements custom JSON unmarshaling for Message.
// The "content" field can be either a string or an array of ContentPart.
func (m *Message) UnmarshalJSON(data []byte) error {
	type alias struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content,omitempty"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
	}

	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}

	m.Role = a.Role
	m.ToolCalls = a.ToolCalls
	m.ToolCallID = a.ToolCallID

	if len(a.Content) == 0 || string(a.Content) == "null" {
		return nil
	}

	// Try string first.
	var s string
	if err := json.Unmarshal(a.Content, &s); err == nil {
		m.Content = &s
		return nil
	}

	// Try array of content parts.
	var parts []ContentPart
	if err := json.Unmarshal(a.Content, &parts); err == nil {
		m.ContentParts = parts
		return nil
	}

	return fmt.Errorf("openaicompat: unsupported content type in message")
}

// ToolCall represents a tool invocation in an assistant message.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction contains the function name and arguments for a tool call.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef is a tool definition in the request.
type ToolDef struct {
	Type     string      `json:"type"`
	Function ToolDefFunc `json:"function"`
}

// ToolDefFunc is the function definition within a tool.
type ToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// --- response types ---

// Response is the OpenAI-compatible chat completions response.
type Response struct {
	ID      string   `json:"id,omitempty"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is a single choice in the response.
type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage contains token usage information.
type Usage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// PromptTokensDetails contains prompt caching details.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}
