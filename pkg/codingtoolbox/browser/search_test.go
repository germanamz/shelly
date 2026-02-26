package browser

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
)

func TestSearch_EmptyQuery(t *testing.T) {
	store := newTestStore(t)
	b := New(context.Background(), store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)
	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_search",
		Arguments: `{"query":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "query is required")
}

func TestSearch_InvalidInput(t *testing.T) {
	store := newTestStore(t)
	b := New(context.Background(), store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)
	tb := b.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "web_search",
		Arguments: `{invalid`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "invalid input")
}

func TestSearch_ToolRegistered(t *testing.T) {
	store := newTestStore(t)
	b := New(context.Background(), store, autoApprove, WithHeadless())
	t.Cleanup(b.Close)

	tb := b.Tools()
	tool, ok := tb.Get("web_search")
	assert.True(t, ok)
	assert.Equal(t, "web_search", tool.Name)
}
