package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNavigate_EmptyURL(t *testing.T) {
	store := newTestStore(t)
	b := New(context.Background(), store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)
	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_navigate",
		Arguments: `{"url":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "url is required")
}

func TestNavigate_Denied(t *testing.T) {
	b, _ := newTestBrowser(t, autoDeny)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer srv.Close()

	tb := b.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestNavigate_Success(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Test Page</title></head><body>
			<h1>Hello World</h1>
			<script>var x = 1;</script>
			<style>body { color: red; }</style>
			<p>Visible text here</p>
		</body></html>`))
	}))
	defer srv.Close()

	tb := b.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out navigateOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Equal(t, "Test Page", out.Title)
	assert.Contains(t, out.Text, "Hello World")
	assert.Contains(t, out.Text, "Visible text here")
	assert.NotContains(t, out.Text, "var x = 1")
}

func TestNavigate_Trust(t *testing.T) {
	b, store := newTestBrowser(t, autoTrust)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer srv.Close()

	tb := b.Tools()
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.True(t, store.IsDomainTrusted("127.0.0.1"))
}
