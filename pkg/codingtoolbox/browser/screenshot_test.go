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

func TestScreenshot_Viewport(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Screenshot</h1></body></html>`))
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
		Name:      "web_screenshot",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)

	var out screenshotOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.NotEmpty(t, out.Base64)
	assert.NotEmpty(t, out.URL)
}

func TestScreenshot_FullPage(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body style="height:3000px"><h1>Tall Page</h1></body></html>`))
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
		Name:      "web_screenshot",
		Arguments: mustJSON(t, screenshotInput{FullPage: true}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out screenshotOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.NotEmpty(t, out.Base64)
}

func TestScreenshot_Element(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<div id="box" style="width:100px;height:100px;background:red;"></div>
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
		Name:      "web_screenshot",
		Arguments: mustJSON(t, screenshotInput{Selector: "#box"}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var out screenshotOutput
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &out))
	assert.NotEmpty(t, out.Base64)
}
