package openai_test

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
	"github.com/germanamz/shelly/pkg/providers/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBatchTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *openai.BatchSubmitter) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	adapter := openai.New(srv.URL, "test-key", "gpt-4")
	return srv, openai.NewBatchSubmitter(adapter)
}

func TestBatchSubmitter_SubmitBatch(t *testing.T) {
	callNum := 0
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callNum++
		switch callNum {
		case 1:
			// File upload.
			assert.Equal(t, "/v1/files", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

			// Verify purpose field.
			assert.NoError(t, r.ParseMultipartForm(1<<20))
			assert.Equal(t, "batch", r.FormValue("purpose"))

			writeJSON(t, w, map[string]any{"id": "file-abc123"})
		case 2:
			// Batch creation.
			assert.Equal(t, "/v1/batches", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			var body map[string]any
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
				return
			}
			assert.Equal(t, "file-abc123", body["input_file_id"])
			assert.Equal(t, "/v1/chat/completions", body["endpoint"])

			writeJSON(t, w, map[string]any{
				"id":     "batch-xyz",
				"status": "validating",
			})
		}
	})

	reqs := []batch.Request{
		{
			ID:   "req-1",
			Chat: chat.New(message.NewText("", role.User, "hello")),
		},
	}

	batchID, err := sub.SubmitBatch(context.Background(), reqs)
	require.NoError(t, err)
	assert.Equal(t, "batch-xyz", batchID)
}

func TestBatchSubmitter_PollBatch_InProgress(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/batches/batch-123", r.URL.Path)
		writeJSON(t, w, map[string]any{
			"id":     "batch-123",
			"status": "in_progress",
		})
	})

	results, done, err := sub.PollBatch(context.Background(), "batch-123")
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
			// Status check.
			assert.Equal(t, "/v1/batches/batch-done", r.URL.Path)
			writeJSON(t, w, map[string]any{
				"id":             "batch-done",
				"status":         "completed",
				"output_file_id": "file-output-1",
			})
		case 2:
			// Download results.
			assert.Equal(t, "/v1/files/file-output-1/content", r.URL.Path)
			lines := []string{
				`{"custom_id":"req-1","response":{"status_code":200,"body":{"choices":[{"message":{"role":"assistant","content":"response 1"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}}}`,
				`{"custom_id":"req-2","response":{"status_code":200,"body":{"choices":[{"message":{"role":"assistant","content":"response 2"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":8}}}}`,
			}
			_, _ = w.Write([]byte(strings.Join(lines, "\n")))
		}
	})

	results, done, err := sub.PollBatch(context.Background(), "batch-done")
	require.NoError(t, err)
	assert.True(t, done)
	assert.Len(t, results, 2)

	r1 := results["req-1"]
	require.NoError(t, r1.Err)
	assert.Equal(t, "response 1", r1.Message.TextContent())
	assert.Equal(t, 10, r1.Usage.InputTokens)

	r2 := results["req-2"]
	require.NoError(t, r2.Err)
	assert.Equal(t, "response 2", r2.Message.TextContent())
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

func TestBatchSubmitter_SubmitBatch_UploadError(t *testing.T) {
	_, sub := newBatchTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error": "upload failed"}`)
	})

	_, err := sub.SubmitBatch(context.Background(), []batch.Request{
		{ID: "req-1", Chat: chat.New(message.NewText("", role.User, "hello"))},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload")
}
