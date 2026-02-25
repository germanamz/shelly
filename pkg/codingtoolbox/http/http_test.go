package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autoApprove(_ context.Context, _ string, _ []string) (string, error) {
	return "yes", nil
}

func autoTrust(_ context.Context, _ string, _ []string) (string, error) {
	return "trust", nil
}

func autoDeny(_ context.Context, _ string, _ []string) (string, error) {
	return "no", nil
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return string(data)
}

func newTestHTTP(t *testing.T, askFn AskFunc) (*HTTP, *permissions.Store) {
	t.Helper()

	dir := t.TempDir()
	store, err := permissions.New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	return New(store, askFn), store
}

func TestFetch_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "hello")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response body"))
	}))
	defer srv.Close()

	h, _ := newTestHTTP(t, autoApprove)
	tb := h.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "http_fetch",
		Arguments: mustJSON(t, fetchInput{URL: srv.URL}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out fetchOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Equal(t, 200, out.Status)
	assert.Equal(t, "response body", out.Body)
	assert.Equal(t, "hello", out.Headers["X-Test"])
}

func TestFetch_POST(t *testing.T) {
	var receivedBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer srv.Close()

	h, _ := newTestHTTP(t, autoApprove)
	tb := h.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "http_fetch",
		Arguments: mustJSON(t, fetchInput{
			URL:    srv.URL,
			Method: "POST",
			Body:   "payload",
			Headers: map[string]string{
				"Content-Type": "text/plain",
			},
		}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out fetchOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Equal(t, 201, out.Status)
	assert.Equal(t, "payload", receivedBody)
}

func TestFetch_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h, _ := newTestHTTP(t, autoDeny)
	tb := h.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "http_fetch",
		Arguments: mustJSON(t, fetchInput{URL: srv.URL}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestFetch_Trust(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	h, store := newTestHTTP(t, autoTrust)
	tb := h.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "http_fetch",
		Arguments: mustJSON(t, fetchInput{URL: srv.URL}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.True(t, store.IsDomainTrusted("127.0.0.1"))

	// Subsequent calls bypass the ask — switch to deny to prove it.
	h.ask = autoDeny

	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "http_fetch",
		Arguments: mustJSON(t, fetchInput{URL: srv.URL}),
	})

	assert.False(t, tr.IsError, tr.Content)
}

func TestFetch_EmptyURL(t *testing.T) {
	h, _ := newTestHTTP(t, autoApprove)
	tb := h.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "http_fetch",
		Arguments: `{"url":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "url is required")
}

func TestNew_ClientTimeout(t *testing.T) {
	h, _ := newTestHTTP(t, autoApprove)
	assert.Equal(t, 60*time.Second, h.client.Timeout)
}

func TestFetch_CustomHeaders(t *testing.T) {
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h, _ := newTestHTTP(t, autoApprove)
	tb := h.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:   "tc1",
		Name: "http_fetch",
		Arguments: mustJSON(t, fetchInput{
			URL:     srv.URL,
			Headers: map[string]string{"Authorization": "Bearer token123"},
		}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "Bearer token123", receivedAuth)
}
