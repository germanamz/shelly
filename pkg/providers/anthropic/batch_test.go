package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBatchTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *anthropic.BatchSubmitter) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	adapter := anthropic.New(srv.URL, "test-key", "claude-test")
	return srv, anthropic.NewBatchSubmitter(adapter)
}

func TestBatchSubmitter_SubmitBatch(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages/batches", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))

		var body map[string]any
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			return
		}

		reqs, ok := body["requests"].([]any)
		if !assert.True(t, ok) {
			return
		}
		assert.Len(t, reqs, 2)

		first := reqs[0].(map[string]any)
		assert.Equal(t, "req-1", first["custom_id"])

		writeJSON(t, w, map[string]any{
			"id":                "batch-abc123",
			"processing_status": "in_progress",
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
	assert.Equal(t, "batch-abc123", batchID)
}

func TestBatchSubmitter_PollBatch_NotDone(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages/batches/batch-123", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		writeJSON(t, w, map[string]any{
			"id":                "batch-123",
			"processing_status": "in_progress",
		})
	})

	results, done, err := sub.PollBatch(context.Background(), "batch-123")
	require.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, results)
}

func TestBatchSubmitter_PollBatch_Done(t *testing.T) {
	callNum := 0
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callNum++
		switch callNum {
		case 1:
			assert.Equal(t, "/v1/messages/batches/batch-123", r.URL.Path)
			writeJSON(t, w, map[string]any{
				"id":                "batch-123",
				"processing_status": "ended",
			})
		case 2:
			assert.Equal(t, "/v1/messages/batches/batch-123/results", r.URL.Path)
			w.Header().Set("Content-Type", "application/jsonl")
			lines := []string{
				`{"custom_id":"req-1","result":{"type":"succeeded","message":{"content":[{"type":"text","text":"response 1"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}}}`,
				`{"custom_id":"req-2","result":{"type":"succeeded","message":{"content":[{"type":"text","text":"response 2"}],"stop_reason":"end_turn","usage":{"input_tokens":15,"output_tokens":8}}}}`,
			}
			_, _ = w.Write([]byte(strings.Join(lines, "\n")))
		default:
			t.Errorf("unexpected call %d to %s", callNum, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	results, done, err := sub.PollBatch(context.Background(), "batch-123")
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
	assert.Equal(t, 15, r2.Usage.InputTokens)
	assert.Equal(t, 8, r2.Usage.OutputTokens)
}

func TestBatchSubmitter_PollBatch_ErroredResult(t *testing.T) {
	callNum := 0
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		callNum++
		switch callNum {
		case 1:
			writeJSON(t, w, map[string]any{
				"id":                "batch-err",
				"processing_status": "ended",
			})
		case 2:
			w.Header().Set("Content-Type", "application/jsonl")
			_, _ = w.Write([]byte(`{"custom_id":"req-1","result":{"type":"errored","error":{"type":"invalid_request_error","message":"too many tokens"}}}`))
		}
	})

	results, done, err := sub.PollBatch(context.Background(), "batch-err")
	require.NoError(t, err)
	assert.True(t, done)
	assert.Len(t, results, 1)

	r1 := results["req-1"]
	require.Error(t, r1.Err)
	assert.Contains(t, r1.Err.Error(), "too many tokens")
}

func TestBatchSubmitter_CancelBatch(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages/batches/batch-cancel/cancel", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	})

	err := sub.CancelBatch(context.Background(), "batch-cancel")
	require.NoError(t, err)
}

func TestBatchSubmitter_SubmitBatch_APIError(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal error"}`))
	})

	_, err := sub.SubmitBatch(context.Background(), []batch.Request{
		{ID: "req-1", Chat: chat.New(message.NewText("", role.User, "hello"))},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anthropic batch: submit")
}
