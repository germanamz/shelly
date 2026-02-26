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

func TestClick_EmptySelector(t *testing.T) {
	store := newTestStore(t)
	b := New(store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)
	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "browser_click",
		Arguments: `{"selector":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "selector is required")
}

func TestClick_Success(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/page2" {
			_, _ = w.Write([]byte(`<html><head><title>Page 2</title></head><body>Page 2</body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<html><head><title>Page 1</title></head><body><a id="link" href="/page2">Go</a></body></html>`))
	}))
	defer srv.Close()

	tb := b.Tools()

	// First navigate to the page.
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "browser_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})
	require.False(t, tr.IsError, tr.Content)

	// Click the link.
	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "browser_click",
		Arguments: mustJSON(t, clickInput{Selector: "#link"}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out clickOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Equal(t, "Page 2", out.Title)
}

func TestType_EmptySelector(t *testing.T) {
	store := newTestStore(t)
	b := New(store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)
	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "browser_type",
		Arguments: `{"selector":"","text":"hello"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "selector is required")
}

func TestType_EmptyText(t *testing.T) {
	store := newTestStore(t)
	b := New(store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)
	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "browser_type",
		Arguments: `{"selector":"#input","text":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "text is required")
}

func TestType_Success(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Form</title></head><body>
			<input id="input" type="text" />
		</body></html>`))
	}))
	defer srv.Close()

	tb := b.Tools()

	// Navigate first.
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "browser_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})
	require.False(t, tr.IsError, tr.Content)

	// Type into the input.
	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "browser_type",
		Arguments: mustJSON(t, typeInput{Selector: "#input", Text: "hello world"}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out typeOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Equal(t, "Form", out.Title)
}
