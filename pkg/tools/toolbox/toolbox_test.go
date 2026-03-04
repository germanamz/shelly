package toolbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func echoHandler(_ context.Context, input json.RawMessage) (string, error) {
	return string(input), nil
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
	assert.Equal(t, 0, tb.Len())
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

	assert.Equal(t, 3, tb.Len())
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
	assert.Equal(t, 1, tb.Len())
}

func TestRegisterReplacePreservesOrder(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"), newEchoTool("b"), newEchoTool("c"))
	tb.Register(Tool{Name: "b", Description: "replaced", Handler: echoHandler})

	tools := tb.Tools()
	require.Len(t, tools, 3)
	assert.Equal(t, "a", tools[0].Name)
	assert.Equal(t, "b", tools[1].Name)
	assert.Equal(t, "replaced", tools[1].Description)
	assert.Equal(t, "c", tools[2].Name)
}

func TestToolsInsertionOrder(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("x"))
	tb.Register(newEchoTool("y"))

	tools := tb.Tools()
	require.Len(t, tools, 2)
	assert.Equal(t, "x", tools[0].Name)
	assert.Equal(t, "y", tools[1].Name)
}

func TestMerge(t *testing.T) {
	tb1 := New()
	tb1.Register(newEchoTool("a"), newEchoTool("b"))

	tb2 := New()
	tb2.Register(newEchoTool("c"))

	tb1.Merge(tb2)

	assert.Equal(t, 3, tb1.Len())
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
	assert.Equal(t, 1, tb1.Len())
}

func TestFilterSubset(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"), newEchoTool("b"), newEchoTool("c"))

	filtered := tb.Filter([]string{"a", "c"})

	assert.Equal(t, 2, filtered.Len())
	_, ok := filtered.Get("a")
	assert.True(t, ok)
	_, ok = filtered.Get("c")
	assert.True(t, ok)
	_, ok = filtered.Get("b")
	assert.False(t, ok)
}

func TestFilterPreservesRequestedOrder(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"), newEchoTool("b"), newEchoTool("c"))

	filtered := tb.Filter([]string{"c", "a"})
	tools := filtered.Tools()
	require.Len(t, tools, 2)
	assert.Equal(t, "c", tools[0].Name)
	assert.Equal(t, "a", tools[1].Name)
}

func TestFilterNilReturnsSamePointer(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"))

	filtered := tb.Filter(nil)
	assert.Same(t, tb, filtered)
}

func TestFilterEmptySliceReturnsEmptyToolBox(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"))

	filtered := tb.Filter([]string{})
	assert.NotSame(t, tb, filtered)
	assert.Equal(t, 0, filtered.Len())
}

func TestFilterMissingNamesSkipped(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"))

	filtered := tb.Filter([]string{"a", "missing", "also_missing"})

	assert.Equal(t, 1, filtered.Len())
	_, ok := filtered.Get("a")
	assert.True(t, ok)
}

func TestFilterOriginalNotMutated(t *testing.T) {
	tb := New()
	tb.Register(newEchoTool("a"), newEchoTool("b"), newEchoTool("c"))

	filtered := tb.Filter([]string{"a"})

	// Original still has all three tools.
	assert.Equal(t, 3, tb.Len())
	// Filtered has only one.
	assert.Equal(t, 1, filtered.Len())
}

func TestLen(t *testing.T) {
	tb := New()
	assert.Equal(t, 0, tb.Len())

	tb.Register(newEchoTool("a"))
	assert.Equal(t, 1, tb.Len())

	tb.Register(newEchoTool("b"))
	assert.Equal(t, 2, tb.Len())

	// Replacing doesn't change count.
	tb.Register(newEchoTool("a"))
	assert.Equal(t, 2, tb.Len())
}
