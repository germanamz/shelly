package gemini_test

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
	"github.com/germanamz/shelly/pkg/providers/gemini"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *gemini.Adapter) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	a := gemini.New(srv.URL, "test-key", "gemini-test")

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
		assert.Equal(t, "/v1beta/models/gemini-test:generateContent", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		req := readBody(t, r)

		// System prompt should be in systemInstruction, not in contents.
		si, ok := req["systemInstruction"].(map[string]any)
		assert.True(t, ok)
		siParts, _ := si["parts"].([]any)
		assert.Len(t, siParts, 1)
		firstPart, _ := siParts[0].(map[string]any)
		assert.Equal(t, "You are helpful.", firstPart["text"])

		contents, ok := req["contents"].([]any)
		assert.True(t, ok)
		assert.Len(t, contents, 1)

		writeJSON(t, w, map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role":  "model",
						"parts": []map[string]any{{"text": "Hello there!"}},
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     10,
				"candidatesTokenCount": 5,
				"totalTokenCount":      15,
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

		contents, ok := req["contents"].([]any)
		assert.True(t, ok)
		// user, model, user → 3 content entries
		assert.Len(t, contents, 3)

		// Verify role alternation.
		c0, _ := contents[0].(map[string]any)
		c1, _ := contents[1].(map[string]any)
		c2, _ := contents[2].(map[string]any)
		assert.Equal(t, "user", c0["role"])
		assert.Equal(t, "model", c1["role"])
		assert.Equal(t, "user", c2["role"])

		writeJSON(t, w, map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role":  "model",
						"parts": []map[string]any{{"text": "The capital of France is Paris."}},
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     20,
				"candidatesTokenCount": 10,
				"totalTokenCount":      30,
			},
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

			toolSet, _ := tools[0].(map[string]any)
			decls, _ := toolSet["functionDeclarations"].([]any)
			assert.Len(t, decls, 1)

			decl, _ := decls[0].(map[string]any)
			assert.Equal(t, "get_weather", decl["name"])

			writeJSON(t, w, map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"role": "model",
							"parts": []map[string]any{
								{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"}}},
							},
						},
						"finishReason": "STOP",
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":     15,
					"candidatesTokenCount": 8,
					"totalTokenCount":      23,
				},
			})
		} else {
			// Verify the functionResponse is in the request.
			contents, ok := req["contents"].([]any)
			assert.True(t, ok)
			assert.GreaterOrEqual(t, len(contents), 3)

			writeJSON(t, w, map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"role":  "model",
							"parts": []map[string]any{{"text": "The weather in Paris is sunny."}},
						},
						"finishReason": "STOP",
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":     25,
					"candidatesTokenCount": 12,
					"totalTokenCount":      37,
				},
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
	assert.Equal(t, "get_weather", calls[0].Name)
	assert.Contains(t, calls[0].ID, "call_get_weather_")

	c.Append(msg)
	c.Append(message.New("tool", role.Tool, content.ToolResult{
		ToolCallID: calls[0].ID,
		Content:    `{"temp": "22C", "condition": "sunny"}`,
	}))

	msg, err = adapter.Complete(context.Background(), c, tools)
	require.NoError(t, err)
	assert.Equal(t, "The weather in Paris is sunny.", msg.TextContent())

	total := adapter.Usage.Total()
	assert.Equal(t, 40, total.InputTokens)
	assert.Equal(t, 20, total.OutputTokens)
}

func TestComplete_SystemPromptInInstruction(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		// System messages should not appear in contents.
		contents, ok := req["contents"].([]any)
		assert.True(t, ok)
		for _, c := range contents {
			entry, _ := c.(map[string]any)
			assert.NotEqual(t, "system", entry["role"], "system role must not appear in contents")
		}

		// Should be in systemInstruction.
		si, ok := req["systemInstruction"].(map[string]any)
		assert.True(t, ok)
		siParts, _ := si["parts"].([]any)
		assert.Len(t, siParts, 1)
		firstPart, _ := siParts[0].(map[string]any)
		assert.Equal(t, "Be concise.", firstPart["text"])

		writeJSON(t, w, map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role":  "model",
						"parts": []map[string]any{{"text": "OK"}},
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     5,
				"candidatesTokenCount": 1,
				"totalTokenCount":      6,
			},
		})
	})

	c := chat.New(
		message.NewText("system", role.System, "Be concise."),
		message.NewText("user", role.User, "Hello"),
	)

	_, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)
}

func TestComplete_EmptyCandidates(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"candidates":    []map[string]any{},
			"usageMetadata": map[string]any{"promptTokenCount": 5, "candidatesTokenCount": 0, "totalTokenCount": 5},
		})
	})

	c := chat.New(
		message.NewText("user", role.User, "Hi"),
	)

	_, err := adapter.Complete(context.Background(), c, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty candidates")
}

func TestComplete_HTTPError(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	})

	c := chat.New(
		message.NewText("user", role.User, "Hi"),
	)

	_, err := adapter.Complete(context.Background(), c, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
