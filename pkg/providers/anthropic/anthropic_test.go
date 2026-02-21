package anthropic_test

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
	"github.com/germanamz/shelly/pkg/providers/anthropic"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *anthropic.Adapter) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	a := anthropic.New(srv.URL, "test-key", "claude-test")

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
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		req := readBody(t, r)

		assert.Equal(t, "claude-test", req["model"])
		assert.Equal(t, "You are helpful.", req["system"])

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)
		assert.Len(t, msgs, 1)

		writeJSON(t, w, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hello there!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		})
	})

	c := chat.New(
		message.NewText("system", role.System, "You are helpful."),
		message.NewText("user", role.User, "Hi"),
	)

	msg, err := adapter.Complete(context.Background(), c)
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
		assert.Len(t, msgs, 3)

		writeJSON(t, w, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "The capital of France is Paris."},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 20, "output_tokens": 10},
		})
	})

	c := chat.New(
		message.NewText("system", role.System, "You are helpful."),
		message.NewText("user", role.User, "What is the capital of France?"),
		message.NewText("assistant", role.Assistant, "Let me think..."),
		message.NewText("user", role.User, "Please answer."),
	)

	msg, err := adapter.Complete(context.Background(), c)
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
			assert.Equal(t, "get_weather", tool["name"])

			writeJSON(t, w, map[string]any{
				"content": []map[string]any{
					{"type": "tool_use", "id": "call_1", "name": "get_weather", "input": map[string]any{"city": "Paris"}},
				},
				"stop_reason": "tool_use",
				"usage":       map[string]any{"input_tokens": 15, "output_tokens": 8},
			})
		} else {
			msgs, ok := req["messages"].([]any)
			assert.True(t, ok)
			assert.GreaterOrEqual(t, len(msgs), 3)

			writeJSON(t, w, map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "The weather in Paris is sunny."},
				},
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 25, "output_tokens": 12},
			})
		}
	})

	adapter.Tools = []toolbox.Tool{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
	}

	c := chat.New(
		message.NewText("user", role.User, "What's the weather in Paris?"),
	)

	msg, err := adapter.Complete(context.Background(), c)
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

	msg, err = adapter.Complete(context.Background(), c)
	require.NoError(t, err)
	assert.Equal(t, "The weather in Paris is sunny.", msg.TextContent())

	total := adapter.Usage.Total()
	assert.Equal(t, 40, total.InputTokens)
	assert.Equal(t, 20, total.OutputTokens)
}

func TestComplete_SystemPromptSkipped(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)

		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			assert.NotEqual(t, "system", msg["role"], "system role must not appear in messages array")
		}

		assert.Equal(t, "Be concise.", req["system"])

		writeJSON(t, w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "OK"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 5, "output_tokens": 1},
		})
	})

	c := chat.New(
		message.NewText("system", role.System, "Be concise."),
		message.NewText("user", role.User, "Hello"),
	)

	_, err := adapter.Complete(context.Background(), c)
	require.NoError(t, err)
}

func TestComplete_HTTPError(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	})

	c := chat.New(
		message.NewText("user", role.User, "Hi"),
	)

	_, err := adapter.Complete(context.Background(), c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
