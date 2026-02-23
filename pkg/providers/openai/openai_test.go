package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/providers/openai"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *openai.Adapter) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	a := openai.New(srv.URL, "test-key", "gpt-4")

	return srv, a
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("failed to encode response: %v", err)
	}
}

func readBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}

	return req
}

func TestComplete_SimpleText(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		req := readBody(t, r)

		assert.Equal(t, "gpt-4", req["model"])

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)
		assert.Len(t, msgs, 2) // system + user

		first, _ := msgs[0].(map[string]any)
		assert.Equal(t, "system", first["role"])

		text := "Hello there!"
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"role": "assistant", "content": text},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		})
	})

	c := chat.New(
		message.NewText("system", role.System, "You are helpful."),
		message.NewText("user", role.User, "Hi"),
	)

	msg, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)

	assert.Equal(t, role.Assistant, msg.Role)
	assert.Equal(t, "Hello there!", msg.TextContent())

	last, ok := adapter.Usage.Last()
	require.True(t, ok)
	assert.Equal(t, 10, last.InputTokens)
	assert.Equal(t, 5, last.OutputTokens)
}

func TestComplete_MultiTurn(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)
		assert.Len(t, msgs, 4) // system + user + assistant + user

		text := "The capital of France is Paris."
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"role": "assistant", "content": text},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 10},
		})
	})

	c := chat.New(
		message.NewText("system", role.System, "You are helpful."),
		message.NewText("user", role.User, "What is the capital of France?"),
		message.NewText("assistant", role.Assistant, "Let me think..."),
		message.NewText("user", role.User, "Please answer."),
	)

	msg, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)
	assert.Equal(t, "The capital of France is Paris.", msg.TextContent())
}

func TestComplete_ToolCall(t *testing.T) {
	callCount := 0

	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++

		req := readBody(t, r)

		if callCount == 1 {
			tools, ok := req["tools"].([]any)
			assert.True(t, ok)
			assert.Len(t, tools, 1)

			tool, _ := tools[0].(map[string]any)
			assert.Equal(t, "function", tool["type"])

			fn, _ := tool["function"].(map[string]any)
			assert.Equal(t, "get_weather", fn["name"])

			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": nil,
							"tool_calls": []map[string]any{
								{
									"id":   "call_1",
									"type": "function",
									"function": map[string]any{
										"name":      "get_weather",
										"arguments": `{"city":"Paris"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
				"usage": map[string]any{"prompt_tokens": 15, "completion_tokens": 8},
			})
		} else {
			msgs, ok := req["messages"].([]any)
			assert.True(t, ok)
			assert.GreaterOrEqual(t, len(msgs), 3)

			lastMsg, _ := msgs[len(msgs)-1].(map[string]any)
			assert.Equal(t, "tool", lastMsg["role"])

			text := "The weather in Paris is sunny."
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"message":       map[string]any{"role": "assistant", "content": text},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{"prompt_tokens": 25, "completion_tokens": 12},
			})
		}
	})

	tools := []toolbox.Tool{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
	}

	c := chat.New(
		message.NewText("user", role.User, "What's the weather in Paris?"),
	)

	msg, err := adapter.Complete(context.Background(), c, tools)
	require.NoError(t, err)

	calls := msg.ToolCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "call_1", calls[0].ID)
	assert.Equal(t, "get_weather", calls[0].Name)

	c.Append(msg)
	c.Append(message.New("tool", role.Tool, content.ToolResult{
		ToolCallID: "call_1",
		Content:    `{"temp": "22C", "condition": "sunny"}`,
	}))

	msg, err = adapter.Complete(context.Background(), c, tools)
	require.NoError(t, err)
	assert.Equal(t, "The weather in Paris is sunny.", msg.TextContent())

	total := adapter.Usage.Total()
	assert.Equal(t, 40, total.InputTokens)
	assert.Equal(t, 20, total.OutputTokens)
}

func TestComplete_EmptyChoices(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 0},
		})
	})

	c := chat.New(
		message.NewText("user", role.User, "Hi"),
	)

	_, err := adapter.Complete(context.Background(), c, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty choices")
}

func TestComplete_HTTPError(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit exceeded"}}`))
	})

	c := chat.New(
		message.NewText("user", role.User, "Hi"),
	)

	_, err := adapter.Complete(context.Background(), c, nil)
	require.Error(t, err)

	var rle *modeladapter.RateLimitError
	assert.ErrorAs(t, err, &rle)
}
