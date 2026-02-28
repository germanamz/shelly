// Package gemini provides a Completer implementation for the Google Gemini API.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

var _ modeladapter.Completer = (*Adapter)(nil)

// Adapter implements modeladapter.Completer for the Google Gemini API.
type Adapter struct {
	modeladapter.ModelAdapter

	callSeq atomic.Int64
}

// New creates an Adapter configured for the Gemini API.
// The baseURL should be "https://generativelanguage.googleapis.com" (no trailing slash).
func New(baseURL, apiKey, model string) *Adapter {
	a := &Adapter{}
	a.BaseURL = baseURL
	a.Auth = modeladapter.Auth{
		Key:    apiKey,
		Header: "x-goog-api-key",
	}
	a.Name = model
	a.MaxTokens = 8192

	return a
}

// Complete sends a conversation to the Gemini API and returns the assistant's reply.
func (a *Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	req := a.buildRequest(c, tools)
	path := fmt.Sprintf("/v1beta/models/%s:generateContent", a.Name)

	var resp apiResponse
	if err := a.PostJSON(ctx, path, req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("gemini: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return message.Message{}, fmt.Errorf("gemini: empty candidates in response")
	}

	a.Usage.Add(usage.TokenCount{
		InputTokens:  resp.UsageMetadata.PromptTokenCount,
		OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
	})

	return a.parseCandidate(resp.Candidates[0]), nil
}

// --- request types ---

type apiRequest struct {
	Contents          []apiContent     `json:"contents"`
	SystemInstruction *apiContent      `json:"systemInstruction,omitempty"`
	Tools             []apiToolSet     `json:"tools,omitempty"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

type apiContent struct {
	Role  string    `json:"role"`
	Parts []apiPart `json:"parts"`
}

type apiPart struct {
	Text             string           `json:"text,omitempty"`
	FunctionCall     *apiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *apiFunctionResp `json:"functionResponse,omitempty"`
}

type apiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type apiFunctionResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type apiToolSet struct {
	FunctionDeclarations []apiFuncDecl `json:"functionDeclarations"`
}

type apiFuncDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type generationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens"`
}

// --- response types ---

type apiResponse struct {
	Candidates    []apiCandidate `json:"candidates"`
	UsageMetadata apiUsageMeta   `json:"usageMetadata"`
}

type apiCandidate struct {
	Content      apiContent `json:"content"`
	FinishReason string     `json:"finishReason"`
}

type apiUsageMeta struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// --- conversion helpers ---

func (a *Adapter) buildRequest(c *chat.Chat, tools []toolbox.Tool) apiRequest {
	req := apiRequest{
		GenerationConfig: generationConfig{
			MaxOutputTokens: a.MaxTokens,
		},
	}

	if a.Temperature != 0 {
		t := a.Temperature
		req.GenerationConfig.Temperature = &t
	}

	if len(tools) > 0 {
		decls := make([]apiFuncDecl, len(tools))
		for i, t := range tools {
			schema := t.InputSchema
			if schema == nil {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			decls[i] = apiFuncDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			}
		}
		req.Tools = []apiToolSet{{FunctionDeclarations: decls}}
	}

	// Extract system prompt into systemInstruction.
	if sp := c.SystemPrompt(); sp != "" {
		req.SystemInstruction = &apiContent{
			Role:  "",
			Parts: []apiPart{{Text: sp}},
		}
	}

	// Build toolCallID → functionName lookup for functionResponse conversion.
	callNameMap := buildCallNameMap(c.Messages())

	msgs := c.Messages()
	for _, m := range msgs {
		if m.Role == role.System {
			continue
		}
		a.appendContent(&req.Contents, m, callNameMap)
	}

	return req
}

// buildCallNameMap scans messages for ToolCall parts and builds a map from
// tool call ID to function name. This is needed because ToolResult only
// carries the call ID, but Gemini's functionResponse requires the function name.
func buildCallNameMap(msgs []message.Message) map[string]string {
	m := make(map[string]string)
	for _, msg := range msgs {
		for _, p := range msg.Parts {
			if tc, ok := p.(content.ToolCall); ok {
				m[tc.ID] = tc.Name
			}
		}
	}
	return m
}

func (a *Adapter) appendContent(contents *[]apiContent, m message.Message, callNameMap map[string]string) {
	for _, p := range m.Parts {
		part := a.partToAPIPart(p, callNameMap)
		if part == nil {
			continue
		}

		apiRole := mapRole(m.Role, p)

		// Merge into the last content if it has the same role (Gemini requires alternation).
		if len(*contents) > 0 && (*contents)[len(*contents)-1].Role == apiRole {
			(*contents)[len(*contents)-1].Parts = append((*contents)[len(*contents)-1].Parts, *part)
			continue
		}

		*contents = append(*contents, apiContent{
			Role:  apiRole,
			Parts: []apiPart{*part},
		})
	}
}

func (a *Adapter) partToAPIPart(p content.Part, callNameMap map[string]string) *apiPart {
	switch v := p.(type) {
	case content.Text:
		return &apiPart{Text: v.Text}
	case content.ToolCall:
		args := json.RawMessage(v.Arguments)
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		return &apiPart{
			FunctionCall: &apiFunctionCall{
				Name: v.Name,
				Args: args,
			},
		}
	case content.ToolResult:
		name := callNameMap[v.ToolCallID]
		return &apiPart{
			FunctionResponse: &apiFunctionResp{
				Name:     name,
				Response: marshalFunctionResponse(v.Content),
			},
		}
	default:
		return nil
	}
}

// marshalFunctionResponse wraps tool result content into a JSON object
// suitable for Gemini's functionResponse.response field.
// If the content is already valid JSON, it wraps it as {"result": <json>}.
// Otherwise it wraps the plain string as {"result": "string"}.
func marshalFunctionResponse(content string) json.RawMessage {
	// Try to use content as raw JSON value.
	if json.Valid([]byte(content)) {
		return json.RawMessage(`{"result":` + content + `}`)
	}
	// Fall back to encoding as a JSON string.
	b, _ := json.Marshal(content)
	return json.RawMessage(`{"result":` + string(b) + `}`)
}

func mapRole(r role.Role, p content.Part) string {
	switch r {
	case role.Assistant:
		return "model"
	case role.Tool:
		return "user"
	default:
		// ToolResult parts from any role go under "user".
		if _, ok := p.(content.ToolResult); ok {
			return "user"
		}
		return "user"
	}
}

func (a *Adapter) parseCandidate(cand apiCandidate) message.Message {
	var parts []content.Part

	for _, p := range cand.Content.Parts {
		switch {
		case p.FunctionCall != nil:
			seq := a.callSeq.Add(1)
			parts = append(parts, content.ToolCall{
				ID:        fmt.Sprintf("call_%s_%d", p.FunctionCall.Name, seq),
				Name:      p.FunctionCall.Name,
				Arguments: string(p.FunctionCall.Args),
			})
		case p.Text != "":
			parts = append(parts, content.Text{Text: p.Text})
		}
	}

	return message.New("", role.Assistant, parts...)
}
