package grok

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestGrokAdapter_ImplementsCompleter(t *testing.T) {
	var _ modeladapter.Completer = (*GrokAdapter)(nil)
}

func TestNew(t *testing.T) {
	g := New("test-key", nil)

	assert.Equal(t, DefaultBaseURL, g.BaseURL)
	assert.Equal(t, "test-key", g.Auth.Key)
}

func TestComplete_TextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req chatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)
		assert.Equal(t, "grok-3", req.Model)
		assert.Len(t, req.Messages, 2)
		assert.Equal(t, "system", req.Messages[0].Role)
		assert.Equal(t, "You are helpful.", *req.Messages[0].Content)
		assert.Equal(t, "user", req.Messages[1].Role)
		assert.Equal(t, "Hello", *req.Messages[1].Content)

		resp := chatResponse{
			ID: "resp-1",
			Choices: []choice{{
				Message:      apiMessage{Role: "assistant", Content: strPtr("Hi there!")},
				FinishReason: "stop",
			}},
			Usage: apiUsage{PromptTokens: 10, CompletionTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New("test-key", srv.Client())
	g.BaseURL = srv.URL
	g.Name = "grok-3"

	c := chat.New(
		message.NewText("", role.System, "You are helpful."),
		message.NewText("", role.User, "Hello"),
	)

	msg, err := g.Complete(context.Background(), c, nil)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, msg.Role)
	assert.Equal(t, "Hi there!", msg.TextContent())

	last, ok := g.Usage.Last()
	assert.True(t, ok)
	assert.Equal(t, 10, last.InputTokens)
	assert.Equal(t, 5, last.OutputTokens)
}

func TestComplete_ToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := chatResponse{
			ID: "resp-2",
			Choices: []choice{{
				Message: apiMessage{
					Role: "assistant",
					ToolCalls: []apiToolCall{{
						ID:   "call-1",
						Type: "function",
						Function: apiFunction{
							Name:      "get_weather",
							Arguments: `{"city":"NYC"}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
			Usage: apiUsage{PromptTokens: 15, CompletionTokens: 8},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New("key", srv.Client())
	g.BaseURL = srv.URL
	g.Name = "grok-3"

	c := chat.New(message.NewText("", role.User, "What's the weather?"))
	msg, err := g.Complete(context.Background(), c, nil)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, msg.Role)
	assert.Empty(t, msg.TextContent())

	calls := msg.ToolCalls()
	assert.Len(t, calls, 1)
	assert.Equal(t, "call-1", calls[0].ID)
	assert.Equal(t, "get_weather", calls[0].Name)
	assert.JSONEq(t, `{"city":"NYC"}`, calls[0].Arguments)
}

func TestComplete_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := chatResponse{ID: "resp-3", Choices: []choice{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New("key", srv.Client())
	g.BaseURL = srv.URL
	g.Name = "grok-3"

	_, err := g.Complete(context.Background(), chat.New(message.NewText("", role.User, "hi")), nil)
	assert.ErrorContains(t, err, "grok: empty response")
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	g := New("bad-key", srv.Client())
	g.BaseURL = srv.URL
	g.Name = "grok-3"

	_, err := g.Complete(context.Background(), chat.New(message.NewText("", role.User, "hi")), nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "grok:")
}

func TestComplete_TemperatureAndMaxTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.InDelta(t, 0.7, req.Temperature, 0.001)
		assert.Equal(t, 256, req.MaxTokens)

		resp := chatResponse{
			ID:      "resp-4",
			Choices: []choice{{Message: apiMessage{Role: "assistant", Content: strPtr("ok")}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New("key", srv.Client())
	g.BaseURL = srv.URL
	g.Name = "grok-3"
	g.Temperature = 0.7
	g.MaxTokens = 256

	_, err := g.Complete(context.Background(), chat.New(message.NewText("", role.User, "hi")), nil)
	require.NoError(t, err)
}

func TestConvertMessages_AllRoles(t *testing.T) {
	c := chat.New(
		message.NewText("", role.System, "system prompt"),
		message.NewText("", role.User, "user msg"),
		message.New("", role.Assistant,
			content.Text{Text: "let me check"},
			content.ToolCall{ID: "tc-1", Name: "search", Arguments: `{"q":"test"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "tc-1", Content: "result data"},
		),
		message.New("", role.Assistant, content.Text{Text: "here you go"}),
	)

	msgs := convertMessages(c)

	assert.Len(t, msgs, 5)

	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "system prompt", *msgs[0].Content)

	assert.Equal(t, "user", msgs[1].Role)
	assert.Equal(t, "user msg", *msgs[1].Content)

	assert.Equal(t, "assistant", msgs[2].Role)
	assert.Equal(t, "let me check", *msgs[2].Content)
	assert.Len(t, msgs[2].ToolCalls, 1)
	assert.Equal(t, "tc-1", msgs[2].ToolCalls[0].ID)
	assert.Equal(t, "function", msgs[2].ToolCalls[0].Type)
	assert.Equal(t, "search", msgs[2].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"q":"test"}`, msgs[2].ToolCalls[0].Function.Arguments)

	assert.Equal(t, "tool", msgs[3].Role)
	assert.Equal(t, "tc-1", msgs[3].ToolCallID)
	assert.Equal(t, "result data", *msgs[3].Content)

	assert.Equal(t, "assistant", msgs[4].Role)
	assert.Equal(t, "here you go", *msgs[4].Content)
}

func TestConvertMessages_MultipleToolResults(t *testing.T) {
	c := chat.New(
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "tc-1", Content: "result 1"},
			content.ToolResult{ToolCallID: "tc-2", Content: "result 2"},
		),
	)

	msgs := convertMessages(c)

	assert.Len(t, msgs, 2)
	assert.Equal(t, "tc-1", msgs[0].ToolCallID)
	assert.Equal(t, "result 1", *msgs[0].Content)
	assert.Equal(t, "tc-2", msgs[1].ToolCallID)
	assert.Equal(t, "result 2", *msgs[1].Content)
}

func TestConvertResponse_TextOnly(t *testing.T) {
	am := apiMessage{Role: "assistant", Content: strPtr("hello")}
	msg := convertResponse(am)

	assert.Equal(t, role.Assistant, msg.Role)
	assert.Equal(t, "hello", msg.TextContent())
	assert.Empty(t, msg.ToolCalls())
}

func TestConvertResponse_ToolCallsOnly(t *testing.T) {
	am := apiMessage{
		Role: "assistant",
		ToolCalls: []apiToolCall{
			{ID: "c1", Type: "function", Function: apiFunction{Name: "fn1", Arguments: "{}"}},
			{ID: "c2", Type: "function", Function: apiFunction{Name: "fn2", Arguments: `{"x":1}`}},
		},
	}
	msg := convertResponse(am)

	assert.Equal(t, role.Assistant, msg.Role)
	assert.Empty(t, msg.TextContent())

	calls := msg.ToolCalls()
	assert.Len(t, calls, 2)
	assert.Equal(t, "c1", calls[0].ID)
	assert.Equal(t, "fn1", calls[0].Name)
	assert.Equal(t, "c2", calls[1].ID)
	assert.Equal(t, "fn2", calls[1].Name)
}

func TestConvertResponse_TextAndToolCalls(t *testing.T) {
	am := apiMessage{
		Role:    "assistant",
		Content: strPtr("thinking..."),
		ToolCalls: []apiToolCall{
			{ID: "c1", Type: "function", Function: apiFunction{Name: "fn", Arguments: "{}"}},
		},
	}
	msg := convertResponse(am)

	assert.Equal(t, "thinking...", msg.TextContent())
	assert.Len(t, msg.ToolCalls(), 1)
}

func TestMarshalToolDef(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	tool := MarshalToolDef("search", "Search the web", schema)

	assert.Equal(t, "function", tool.Type)
	assert.Equal(t, "search", tool.Function.Name)
	assert.Equal(t, "Search the web", tool.Function.Description)
	assert.JSONEq(t, `{"type":"object","properties":{"q":{"type":"string"}}}`, string(tool.Function.Parameters))
}
