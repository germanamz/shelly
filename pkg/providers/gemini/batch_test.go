package gemini_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/providers/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBatchTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *gemini.BatchSubmitter) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	adapter := gemini.New(srv.URL, "test-key", "gemini-test")
	return srv, gemini.NewBatchSubmitter(adapter)
}

func TestBatchSubmitter_SubmitAndPoll(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/models/gemini-test:batchGenerateContent", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]any
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			return
		}

		reqs, ok := body["requests"].([]any)
		if !assert.True(t, ok) {
			return
		}
		assert.Len(t, reqs, 2)

		writeJSON(t, w, map[string]any{
			"responses": []map[string]any{
				{
					"candidates": []map[string]any{
						{
							"content": map[string]any{
								"role":  "model",
								"parts": []map[string]any{{"text": "response 1"}},
							},
							"finishReason": "STOP",
						},
					},
					"usageMetadata": map[string]any{
						"promptTokenCount":     10,
						"candidatesTokenCount": 5,
						"totalTokenCount":      15,
					},
				},
				{
					"candidates": []map[string]any{
						{
							"content": map[string]any{
								"role":  "model",
								"parts": []map[string]any{{"text": "response 2"}},
							},
							"finishReason": "STOP",
						},
					},
					"usageMetadata": map[string]any{
						"promptTokenCount":     12,
						"candidatesTokenCount": 6,
						"totalTokenCount":      18,
					},
				},
			},
		})
	})

	reqs := []batch.Request{
		{
			ID:   "req-1",
			Chat: chat.New(message.NewText("", role.System, "system"), message.NewText("", role.User, "hello")),
		},
		{
			ID:   "req-2",
			Chat: chat.New(message.NewText("", role.System, "system"), message.NewText("", role.User, "world")),
		},
	}

	batchID, err := sub.SubmitBatch(context.Background(), reqs)
	require.NoError(t, err)
	assert.NotEmpty(t, batchID)

	// PollBatch should return results immediately (Gemini batch is synchronous).
	results, done, err := sub.PollBatch(context.Background(), batchID)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Len(t, results, 2)

	r1 := results["req-1"]
	require.NoError(t, r1.Err)
	assert.Equal(t, "response 1", r1.Message.TextContent())
	assert.Equal(t, 10, r1.Usage.InputTokens)
	assert.Equal(t, 5, r1.Usage.OutputTokens)

	r2 := results["req-2"]
	require.NoError(t, r2.Err)
	assert.Equal(t, "response 2", r2.Message.TextContent())
}

func TestBatchSubmitter_SubmitBatch_APIError(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	})

	_, err := sub.SubmitBatch(context.Background(), []batch.Request{
		{ID: "req-1", Chat: chat.New(message.NewText("", role.User, "hello"))},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gemini batch: submit")
}

func TestBatchSubmitter_SubmitBatch_ErroredResponse(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"responses": []map[string]any{
				{
					"error": map[string]any{
						"code":    400,
						"message": "invalid request",
					},
				},
			},
		})
	})

	batchID, err := sub.SubmitBatch(context.Background(), []batch.Request{
		{ID: "req-1", Chat: chat.New(message.NewText("", role.User, "hello"))},
	})
	require.NoError(t, err)

	results, done, err := sub.PollBatch(context.Background(), batchID)
	require.NoError(t, err)
	assert.True(t, done)

	r1 := results["req-1"]
	require.Error(t, r1.Err)
	assert.Contains(t, r1.Err.Error(), "invalid request")
}

func TestBatchSubmitter_PollBatch_UnknownBatch(t *testing.T) {
	_, sub := newBatchTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {})

	_, _, err := sub.PollBatch(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown batch")
}

func TestBatchSubmitter_CancelBatch(t *testing.T) {
	_, sub := newBatchTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {})

	// CancelBatch is a no-op for Gemini.
	err := sub.CancelBatch(context.Background(), "any-batch")
	require.NoError(t, err)
}
