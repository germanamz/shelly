package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtract_FullPage(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Extract Test</title></head><body>
			<h1>Header</h1>
			<script>var hidden = true;</script>
			<style>.x { display: none; }</style>
			<noscript>No JS</noscript>
			<svg><circle/></svg>
			<p>Paragraph text</p>
		</body></html>`))
	}))
	defer srv.Close()

	tb := b.Tools()

	// Navigate first.
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})
	require.False(t, tr.IsError, tr.Content)

	// Extract.
	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "web_extract",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)

	var out extractOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Contains(t, out.Text, "Header")
	assert.Contains(t, out.Text, "Paragraph text")
	assert.NotContains(t, out.Text, "var hidden")
}

func TestExtract_WithSelector(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<div id="target">Target Content</div>
			<div id="other">Other Content</div>
		</body></html>`))
	}))
	defer srv.Close()

	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})
	require.False(t, tr.IsError, tr.Content)

	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "web_extract",
		Arguments: mustJSON(t, extractInput{Selector: "#target"}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out extractOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Contains(t, out.Text, "Target Content")
	assert.NotContains(t, out.Text, "Other Content")
}

func TestExtract_Truncation(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	// Build a page with more than maxContentBytes of text.
	bigText := strings.Repeat("A", maxContentBytes+1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><p>` + bigText + `</p></body></html>`))
	}))
	defer srv.Close()

	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_navigate",
		Arguments: mustJSON(t, navigateInput{URL: srv.URL}),
	})
	require.False(t, tr.IsError, tr.Content)

	tr = tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "web_extract",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)

	var out extractOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.Contains(t, out.Text, "[content truncated]")
}
