package grok_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/providers/grok"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBatchTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *grok.BatchSubmitter) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	adapter := grok.New(srv.URL, "test-key", "grok-3", nil)

	return srv, grok.NewBatchSubmitter(adapter)
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("failed to encode response: %v", err)
	}
}

func TestBatchSubmitter_SubmitBatch(t *testing.T) {
	callNum := 0
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callNum++
		switch callNum {
		case 1:
			// File upload.
			assert.Equal(t, "/v1/files", r.URL.Path)
			assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

			assert.NoError(t, r.ParseMultipartForm(1<<20))
			assert.Equal(t, "batch", r.FormValue("purpose"))

			writeJSON(t, w, map[string]any{"id": "file-grok-1"})
		case 2:
			// Batch creation.
			assert.Equal(t, "/v1/batches", r.URL.Path)

			var body map[string]any
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
				return
			}
			assert.Equal(t, "file-grok-1", body["input_file_id"])

			writeJSON(t, w, map[string]any{
				"id":     "batch-grok-1",
				"status": "validating",
			})
		}
	})

	reqs := []batch.Request{
		{
			ID:   "req-1",
			Chat: chat.New(message.NewText("", role.System, "system"), message.NewText("", role.User, "hello")),
		},
	}

	batchID, err := sub.SubmitBatch(context.Background(), reqs)
	require.NoError(t, err)
	assert.Equal(t, "batch-grok-1", batchID)
}

func TestBatchSubmitter_PollBatch_InProgress(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"id":     "batch-1",
			"status": "in_progress",
		})
	})

	results, done, err := sub.PollBatch(context.Background(), "batch-1")
	require.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, results)
}

func TestBatchSubmitter_PollBatch_Completed(t *testing.T) {
	callNum := 0
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callNum++
		switch callNum {
		case 1:
			writeJSON(t, w, map[string]any{
				"id":             "batch-done",
				"status":         "completed",
				"output_file_id": "file-out-1",
			})
		case 2:
			assert.Equal(t, "/v1/files/file-out-1/content", r.URL.Path)
			lines := []string{
				`{"custom_id":"req-1","response":{"status_code":200,"body":{"id":"resp-1","choices":[{"message":{"role":"assistant","content":"grok response"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}}}`,
			}
			_, _ = io.WriteString(w, strings.Join(lines, "\n"))
		}
	})

	results, done, err := sub.PollBatch(context.Background(), "batch-done")
	require.NoError(t, err)
	assert.True(t, done)
	assert.Len(t, results, 1)

	r1 := results["req-1"]
	require.NoError(t, r1.Err)
	assert.Equal(t, "grok response", r1.Message.TextContent())
	assert.Equal(t, 10, r1.Usage.InputTokens)
	assert.Equal(t, 5, r1.Usage.OutputTokens)
}

func TestBatchSubmitter_PollBatch_Failed(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"id":     "batch-fail",
			"status": "failed",
		})
	})

	_, _, err := sub.PollBatch(context.Background(), "batch-fail")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

func TestBatchSubmitter_CancelBatch(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/batches/batch-cancel/cancel", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	})

	err := sub.CancelBatch(context.Background(), "batch-cancel")
	require.NoError(t, err)
}
