package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeduplicateToolsNoDuplicates(t *testing.T) {
	tb1 := toolbox.New()
	tb1.Register(toolbox.Tool{
		Name:        "tool_a",
		Description: "Tool A",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "a", nil
		},
	})

	tb2 := toolbox.New()
	tb2.Register(toolbox.Tool{
		Name:        "tool_b",
		Description: "Tool B",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "b", nil
		},
	})

	tools := deduplicateTools([]*toolbox.ToolBox{tb1, tb2})

	require.Len(t, tools, 2)
	assert.Equal(t, "tool_a", tools[0].Name)
	assert.Equal(t, "tool_b", tools[1].Name)
}

func TestDeduplicateToolsRemovesDuplicates(t *testing.T) {
	tb1 := toolbox.New()
	tb1.Register(toolbox.Tool{
		Name:        "shared",
		Description: "First version",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "first", nil
		},
	})

	tb2 := toolbox.New()
	tb2.Register(toolbox.Tool{
		Name:        "shared",
		Description: "Second version",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "second", nil
		},
	})

	tools := deduplicateTools([]*toolbox.ToolBox{tb1, tb2})

	require.Len(t, tools, 1)
	assert.Equal(t, "shared", tools[0].Name)
	// First version wins.
	assert.Equal(t, "First version", tools[0].Description)
}

func TestDeduplicateToolsEmpty(t *testing.T) {
	tools := deduplicateTools(nil)
	assert.Empty(t, tools)
}
