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

	msg, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)

	assert.Equal(t, role.Assistant, msg.Role)
	assert.Equal(t, "Hello there!", msg.TextContent())

	last, ok := adapter.UsageTracker().Last()
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

	total := adapter.UsageTracker().Total()
	assert.Equal(t, 40, total.InputTokens)
	assert.Equal(t, 20, total.OutputTokens)
}

func TestComplete_ToolResultIsError(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)

		// Find the user message containing the tool_result.
		var foundIsError bool
		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			parts, _ := msg["content"].([]any)
			for _, p := range parts {
				block, _ := p.(map[string]any)
				if block["type"] == "tool_result" {
					isErr, exists := block["is_error"]
					if exists {
						foundIsError = isErr == true
					}
				}
			}
		}
		assert.True(t, foundIsError, "tool_result should include is_error: true")

		writeJSON(t, w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "Error noted."}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	})

	c := chat.New(
		message.NewText("user", role.User, "Do something"),
		message.New("assistant", role.Assistant, content.ToolCall{
			ID:        "call_1",
			Name:      "some_tool",
			Arguments: `{}`,
		}),
		message.New("tool", role.Tool, content.ToolResult{
			ToolCallID: "call_1",
			Content:    "something went wrong",
			IsError:    true,
		}),
	)

	msg, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)
	assert.Equal(t, "Error noted.", msg.TextContent())
}

func TestComplete_ToolResultIsErrorOmittedWhenFalse(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)

		// Verify is_error is omitted (not present) when false.
		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			parts, _ := msg["content"].([]any)
			for _, p := range parts {
				block, _ := p.(map[string]any)
				if block["type"] == "tool_result" {
					_, exists := block["is_error"]
					assert.False(t, exists, "is_error should be omitted when false (omitempty)")
				}
			}
		}

		writeJSON(t, w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "OK"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	})

	c := chat.New(
		message.NewText("user", role.User, "Do something"),
		message.New("assistant", role.Assistant, content.ToolCall{
			ID:        "call_1",
			Name:      "some_tool",
			Arguments: `{}`,
		}),
		message.New("tool", role.Tool, content.ToolResult{
			ToolCallID: "call_1",
			Content:    "success",
			IsError:    false,
		}),
	)

	_, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)
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

	_, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)
}

func TestComplete_CacheControl(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		// Verify cache_control is present in the request.
		cc, ok := req["cache_control"].(map[string]any)
		assert.True(t, ok, "cache_control field should be present")
		assert.Equal(t, "ephemeral", cc["type"])

		writeJSON(t, w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "OK"}},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":                10,
				"output_tokens":               5,
				"cache_creation_input_tokens": 100,
				"cache_read_input_tokens":     200,
			},
		})
	})

	c := chat.New(
		message.NewText("system", role.System, "You are helpful."),
		message.NewText("user", role.User, "Hi"),
	)

	_, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)

	last, ok := adapter.UsageTracker().Last()
	require.True(t, ok)
	assert.Equal(t, 10, last.InputTokens)
	assert.Equal(t, 5, last.OutputTokens)
	assert.Equal(t, 100, last.CacheCreationInputTokens)
	assert.Equal(t, 200, last.CacheReadInputTokens)
}

func TestComplete_ImagePart(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)
		assert.Len(t, msgs, 1)

		msg, _ := msgs[0].(map[string]any)
		parts, _ := msg["content"].([]any)
		assert.Len(t, parts, 2) // text + image

		imgBlock, _ := parts[1].(map[string]any)
		assert.Equal(t, "image", imgBlock["type"])

		source, _ := imgBlock["source"].(map[string]any)
		assert.Equal(t, "base64", source["type"])
		assert.Equal(t, "image/png", source["media_type"])
		assert.NotEmpty(t, source["data"])

		writeJSON(t, w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "I see an image."}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 50, "output_tokens": 5},
		})
	})

	c := chat.New(
		message.New("user", role.User,
			content.Text{Text: "What is this?"},
			content.Image{Data: []byte("fake-png-data"), MediaType: "image/png"},
		),
	)

	msg, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)
	assert.Equal(t, "I see an image.", msg.TextContent())
}

func TestComplete_DocumentPart(t *testing.T) {
	_, adapter := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readBody(t, r)

		msgs, ok := req["messages"].([]any)
		assert.True(t, ok)
		assert.Len(t, msgs, 1)

		msg, _ := msgs[0].(map[string]any)
		parts, _ := msg["content"].([]any)
		assert.Len(t, parts, 2) // text + document

		docBlock, _ := parts[1].(map[string]any)
		assert.Equal(t, "document", docBlock["type"])

		source, _ := docBlock["source"].(map[string]any)
		assert.Equal(t, "base64", source["type"])
		assert.Equal(t, "application/pdf", source["media_type"])
		assert.NotEmpty(t, source["data"])

		writeJSON(t, w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "I see a PDF."}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 50, "output_tokens": 5},
		})
	})

	c := chat.New(
		message.New("user", role.User,
			content.Text{Text: "Summarize this."},
			content.Document{Data: []byte("fake-pdf-data"), MediaType: "application/pdf", Path: "report.pdf"},
		),
	)

	msg, err := adapter.Complete(context.Background(), c, nil)
	require.NoError(t, err)
	assert.Equal(t, "I see a PDF.", msg.TextContent())
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
