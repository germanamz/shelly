// Package gemini provides a Completer implementation for the Google Gemini API.
package gemini

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

var _ modeladapter.Completer = (*Adapter)(nil)

// Adapter implements modeladapter.Completer for the Google Gemini API.
type Adapter struct {
	modeladapter.ModelAdapter
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

	// HeaderParser is intentionally not set. The Gemini API does not return
	// rate limit headers as of 2026-03. Adaptive throttling
	// (adaptFromServerInfo) is unavailable; the RateLimitedCompleter falls
	// back to proactive throttling only. When Google adds rate limit headers,
	// set a.HeaderParser here (likely ParseOpenAIRateLimitHeaders).

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
	ThoughtSignature string           `json:"thoughtSignature,omitempty"`
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
				Parameters:  sanitizeSchema(schema),
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
		if err := a.appendContent(&req.Contents, m, callNameMap); err != nil {
			// Log but continue — partial context is better than failing entirely.
			// The error means a tool result references a call ID no longer in history.
			continue
		}
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

func (a *Adapter) appendContent(contents *[]apiContent, m message.Message, callNameMap map[string]string) error {
	for _, p := range m.Parts {
		part, err := a.partToAPIPart(p, callNameMap)
		if err != nil {
			return err
		}
		if part == nil {
			continue
		}

		apiRole := mapRole(m.Role)

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
	return nil
}

func (a *Adapter) partToAPIPart(p content.Part, callNameMap map[string]string) (*apiPart, error) {
	switch v := p.(type) {
	case content.Text:
		return &apiPart{Text: v.Text}, nil
	case content.ToolCall:
		args := json.RawMessage(v.Arguments)
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		part := &apiPart{
			FunctionCall: &apiFunctionCall{
				Name: v.Name,
				Args: args,
			},
		}
		if sig := v.Metadata["thoughtSignature"]; sig != "" {
			part.ThoughtSignature = sig
		}
		return part, nil
	case content.ToolResult:
		name := callNameMap[v.ToolCallID]
		if name == "" {
			return nil, fmt.Errorf("gemini: no function name found for tool call ID %q; conversation history may be incomplete", v.ToolCallID)
		}
		return &apiPart{
			FunctionResponse: &apiFunctionResp{
				Name:     name,
				Response: marshalFunctionResponse(v.Content),
			},
		}, nil
	default:
		return nil, nil
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

// sanitizeSchema removes JSON Schema keywords that the Gemini API does not
// support (e.g. $schema, additionalProperties). It operates recursively so
// nested schemas (inside "properties", "items", etc.) are also cleaned.
func sanitizeSchema(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw // not an object — return as-is
	}

	// Fields the Gemini API rejects at any level.
	delete(obj, "$schema")
	delete(obj, "additionalProperties")

	// Recurse into nested schemas.
	if props, ok := obj["properties"]; ok {
		var propMap map[string]json.RawMessage
		if err := json.Unmarshal(props, &propMap); err == nil {
			for k, v := range propMap {
				propMap[k] = sanitizeSchema(v)
			}
			if b, err := json.Marshal(propMap); err == nil {
				obj["properties"] = b
			}
		}
	}

	if items, ok := obj["items"]; ok {
		obj["items"] = sanitizeSchema(items)
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return b
}

func mapRole(r role.Role) string {
	if r == role.Assistant {
		return "model"
	}
	return "user"
}

// generateCallID creates a unique tool call ID using random bytes.
// Gemini does not return call IDs, so we synthesize them.
func generateCallID(name string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("call_%s_%s", name, hex.EncodeToString(b))
}

func (a *Adapter) parseCandidate(cand apiCandidate) message.Message {
	var parts []content.Part

	for _, p := range cand.Content.Parts {
		switch {
		case p.FunctionCall != nil:
			tc := content.ToolCall{
				ID:        generateCallID(p.FunctionCall.Name),
				Name:      p.FunctionCall.Name,
				Arguments: string(p.FunctionCall.Args),
			}
			if p.ThoughtSignature != "" {
				tc.Metadata = map[string]string{
					"thoughtSignature": p.ThoughtSignature,
				}
			}
			parts = append(parts, tc)
		case p.Text != "":
			parts = append(parts, content.Text{Text: p.Text})
		}
	}

	return message.New("", role.Assistant, parts...)
}
