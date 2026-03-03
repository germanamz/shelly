package effects

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolScopeEffect_ExcludesNamedTools(t *testing.T) {
	e := NewToolScopeEffect(ToolScopeConfig{
		Exclude: []string{"dangerous_tool", "debug_tool"},
	})

	tools := []toolbox.Tool{
		{Name: "safe_tool", Description: "Safe", InputSchema: json.RawMessage(`{}`)},
		{Name: "dangerous_tool", Description: "Dangerous", InputSchema: json.RawMessage(`{}`)},
		{Name: "debug_tool", Description: "Debug", InputSchema: json.RawMessage(`{}`)},
		{Name: "another_safe", Description: "Also safe", InputSchema: json.RawMessage(`{}`)},
	}

	filtered := e.FilterTools(context.Background(), agent.IterationContext{}, tools)

	require.Len(t, filtered, 2)
	assert.Equal(t, "safe_tool", filtered[0].Name)
	assert.Equal(t, "another_safe", filtered[1].Name)
}

func TestToolScopeEffect_EmptyExclude(t *testing.T) {
	e := NewToolScopeEffect(ToolScopeConfig{})

	tools := []toolbox.Tool{
		{Name: "tool_a", Description: "A", InputSchema: json.RawMessage(`{}`)},
		{Name: "tool_b", Description: "B", InputSchema: json.RawMessage(`{}`)},
	}

	filtered := e.FilterTools(context.Background(), agent.IterationContext{}, tools)

	assert.Len(t, filtered, 2)
}

func TestToolScopeEffect_EvalIsNoop(t *testing.T) {
	e := NewToolScopeEffect(ToolScopeConfig{Exclude: []string{"tool"}})

	err := e.Eval(context.Background(), agent.IterationContext{})
	require.NoError(t, err)
}
