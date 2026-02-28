package toolbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func echoHandler(_ context.Context, input json.RawMessage) (string, error) {
	return string(input), nil
}

func errorHandler(_ context.Context, _ json.RawMessage) (string, error) {
	return "", errors.New("tool failed")
}

func newEchoTool(name string) Tool {
	return Tool{
		Name:        name,
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     echoHandler,
	}
}

func TestNew(t *testing.T) {
	tb := New()
	assert.NotNil(t, tb)
	assert.Empty(t, tb.Tools())
}

func TestRegisterAndGet(t *testing.T) {
	tb := New()
	tool := newEchoTool("echo")

	tb.Register(tool)

	got, ok := tb.Get("echo")
	assert.True(t, ok)
	assert.Equal(t, "echo", got.Name)
}

func TestGetNotFound(t *testing.T) {
	tb := New()

	_, ok := tb.Get("missing")
	assert.False(t, ok)
}

func TestRegisterMultiple(t *testing.T) {
	tb := New()
	tb.Register(
		newEchoTool("a"),
		newEchoTool("b"),
		newEchoTool("c"),
	)

	assert.Len(t, tb.Tools(), 3)
}

func TestRegisterReplace(t *testing.T) {
	tb := New()
	tb.Register(Tool{
		Name:        "tool",
		Description: "original",
		Handler:     echoHandler,
	})
	tb.Register(Tool{
		Name:        "tool",
		Description: "replaced",
		Handler:     echoHandler,
	})

	got, ok := tb.Get("tool")
	require.True(t, ok)
	assert.Equal(t, "replaced", got.Description)
	assert.Len(t, tb.Tools(), 1)
}

func TestTools(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("x"))
	tb.Register(newEchoTool("y"))

	tools := tb.Tools()
	assert.Len(t, tools, 2)

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	assert.True(t, names["x"])
	assert.True(t, names["y"])
}

func TestCallSuccess(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("echo"))

	tc := content.ToolCall{
		ID:        "call-1",
		Name:      "echo",
		Arguments: `{"msg":"hi"}`,
	}

	result := tb.Call(context.Background(), tc)
	assert.Equal(t, "call-1", result.ToolCallID)
	assert.JSONEq(t, `{"msg":"hi"}`, result.Content)
	assert.False(t, result.IsError)
}

func TestMerge(t *testing.T) {
	tb1 := New()
	tb1.Register(newEchoTool("a"), newEchoTool("b"))

	tb2 := New()
	tb2.Register(newEchoTool("c"))

	tb1.Merge(tb2)

	assert.Len(t, tb1.Tools(), 3)
	_, ok := tb1.Get("c")
	assert.True(t, ok)
}

func TestMergeOverwrite(t *testing.T) {
	tb1 := New()
	tb1.Register(Tool{Name: "x", Description: "original", Handler: echoHandler})

	tb2 := New()
	tb2.Register(Tool{Name: "x", Description: "replaced", Handler: echoHandler})

	tb1.Merge(tb2)

	got, ok := tb1.Get("x")
	require.True(t, ok)
	assert.Equal(t, "replaced", got.Description)
	assert.Len(t, tb1.Tools(), 1)
}

func TestFilterSubset(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"), newEchoTool("b"), newEchoTool("c"))

	filtered := tb.Filter([]string{"a", "c"})

	assert.Len(t, filtered.Tools(), 2)
	_, ok := filtered.Get("a")
	assert.True(t, ok)
	_, ok = filtered.Get("c")
	assert.True(t, ok)
	_, ok = filtered.Get("b")
	assert.False(t, ok)
}

func TestFilterEmptyReturnsSamePointer(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"))

	filtered := tb.Filter(nil)
	assert.Same(t, tb, filtered)

	filtered = tb.Filter([]string{})
	assert.Same(t, tb, filtered)
}

func TestFilterMissingNamesSkipped(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"))

	filtered := tb.Filter([]string{"a", "missing", "also_missing"})

	assert.Len(t, filtered.Tools(), 1)
	_, ok := filtered.Get("a")
	assert.True(t, ok)
}

func TestFilterOriginalNotMutated(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"), newEchoTool("b"), newEchoTool("c"))

	filtered := tb.Filter([]string{"a"})

	// Original still has all three tools.
	assert.Len(t, tb.Tools(), 3)
	// Filtered has only one.
	assert.Len(t, filtered.Tools(), 1)
}

func TestCallNotFound(t *testing.T) {
	tb := New()

	tc := content.ToolCall{
		ID:   "call-2",
		Name: "missing",
	}

	result := tb.Call(context.Background(), tc)
	assert.Equal(t, "call-2", result.ToolCallID)
	assert.Contains(t, result.Content, "tool not found: missing")
	assert.True(t, result.IsError)
}

func TestCallHandlerError(t *testing.T) {
	tb := New()
	tb.Register(Tool{
		Name:    "fail",
		Handler: errorHandler,
	})

	tc := content.ToolCall{
		ID:   "call-3",
		Name: "fail",
	}

	result := tb.Call(context.Background(), tc)
	assert.Equal(t, "call-3", result.ToolCallID)
	assert.Equal(t, "tool failed", result.Content)
	assert.True(t, result.IsError)
}
