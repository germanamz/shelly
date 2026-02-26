package browser

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chromeAvailable() bool {
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	// macOS application bundle.
	if _, err := exec.LookPath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
		return true
	}
	return false
}

func skipIfNoChrome(t *testing.T) {
	t.Helper()
	if !chromeAvailable() {
		t.Skip("Chrome/Chromium not found on PATH")
	}
}

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

func newTestStore(t *testing.T) *permissions.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := permissions.New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)
	return store
}

func newTestBrowser(t *testing.T, askFn AskFunc) (*Browser, *permissions.Store) {
	t.Helper()
	skipIfNoChrome(t)

	store := newTestStore(t)
	b := New(store, askFn, WithHeadless())
	t.Cleanup(b.Close)

	return b, store
}

func TestTools_Count(t *testing.T) {
	store := newTestStore(t)
	b := New(store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)

	tb := b.Tools()
	tools := tb.Tools()
	assert.Len(t, tools, 6)
}

func TestClose_NoStart(t *testing.T) {
	store := newTestStore(t)
	b := New(store, autoApprove, WithHeadless())
	// Close without ever starting should not panic.
	b.Close()
}

func TestCheckPermission_Denied(t *testing.T) {
	store := newTestStore(t)
	b := New(store, autoDeny, WithHeadless())
	t.Cleanup(b.Close)

	err := b.checkPermission(context.Background(), "https://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestCheckPermission_Trust(t *testing.T) {
	store := newTestStore(t)
	b := New(store, autoTrust, WithHeadless())
	t.Cleanup(b.Close)

	err := b.checkPermission(context.Background(), "https://example.com")
	require.NoError(t, err)
	assert.True(t, store.IsDomainTrusted("example.com"))
}

func TestCollapseWhitespace(t *testing.T) {
	input := "Hello\n\n\n\nWorld\n\n\nFoo"
	result := collapseWhitespace(input)
	assert.Equal(t, "Hello\nWorld\nFoo", result)
}

func TestExtractTool_NoSelector(t *testing.T) {
	b, _ := newTestBrowser(t, autoApprove)
	tb := b.Tools()

	// Navigate to about:blank first (no permission needed).
	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "browser_extract",
		Arguments: `{}`,
	})

	// Should succeed (page is about:blank, empty text is fine).
	assert.False(t, tr.IsError, tr.Content)
}
