package grok

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/providers/internal/openaicompat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestGrokAdapter_ImplementsCompleter(t *testing.T) {
	var _ modeladapter.Completer = (*GrokAdapter)(nil)
}

func TestNew(t *testing.T) {
	g := New(DefaultBaseURL, "test-key", "grok-3", nil)

	assert.Equal(t, "grok-3", g.Config.Name)
	assert.Equal(t, 4096, g.Config.MaxTokens)
}

func TestComplete_TextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req openaicompat.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)
		assert.Equal(t, "grok-3", req.Model)
		assert.Len(t, req.Messages, 2)
		assert.Equal(t, "system", req.Messages[0].Role)
		assert.Equal(t, "You are helpful.", *req.Messages[0].Content)
		assert.Equal(t, "user", req.Messages[1].Role)
		assert.Equal(t, "Hello", *req.Messages[1].Content)

		resp := openaicompat.Response{
			ID: "resp-1",
			Choices: []openaicompat.Choice{{
				Message:      openaicompat.Message{Role: "assistant", Content: strPtr("Hi there!")},
				FinishReason: "stop",
			}},
			Usage: openaicompat.Usage{PromptTokens: 10, CompletionTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New(srv.URL, "test-key", "grok-3", srv.Client())

	c := chat.New(
		message.NewText("", role.System, "You are helpful."),
		message.NewText("", role.User, "Hello"),
	)

	msg, err := g.Complete(context.Background(), c, nil)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, msg.Role)
	assert.Equal(t, "Hi there!", msg.TextContent())

	last, ok := g.usage.Last()
	assert.True(t, ok)
	assert.Equal(t, 10, last.InputTokens)
	assert.Equal(t, 5, last.OutputTokens)
}

func TestComplete_ToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := openaicompat.Response{
			ID: "resp-2",
			Choices: []openaicompat.Choice{{
				Message: openaicompat.Message{
					Role: "assistant",
					ToolCalls: []openaicompat.ToolCall{{
						ID:   "call-1",
						Type: "function",
						Function: openaicompat.ToolFunction{
							Name:      "get_weather",
							Arguments: `{"city":"NYC"}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
			Usage: openaicompat.Usage{PromptTokens: 15, CompletionTokens: 8},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New(srv.URL, "key", "grok-3", srv.Client())

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
		resp := openaicompat.Response{ID: "resp-3", Choices: []openaicompat.Choice{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New(srv.URL, "key", "grok-3", srv.Client())

	_, err := g.Complete(context.Background(), chat.New(message.NewText("", role.User, "hi")), nil)
	assert.ErrorContains(t, err, "grok: empty response")
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	g := New(srv.URL, "bad-key", "grok-3", srv.Client())

	_, err := g.Complete(context.Background(), chat.New(message.NewText("", role.User, "hi")), nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "grok:")
}

func TestComplete_TemperatureAndMaxTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaicompat.Request
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if assert.NotNil(t, req.Temperature) {
			assert.InDelta(t, 0.7, *req.Temperature, 0.001)
		}
		assert.Equal(t, 256, req.MaxTokens)

		resp := openaicompat.Response{
			ID:      "resp-4",
			Choices: []openaicompat.Choice{{Message: openaicompat.Message{Role: "assistant", Content: strPtr("ok")}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := New(srv.URL, "key", "grok-3", srv.Client())
	g.Config.Temperature = 0.7
	g.Config.MaxTokens = 256

	_, err := g.Complete(context.Background(), chat.New(message.NewText("", role.User, "hi")), nil)
	require.NoError(t, err)
}
