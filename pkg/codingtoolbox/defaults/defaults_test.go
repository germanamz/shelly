package defaults

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
)

func echo(_ context.Context, input json.RawMessage) (string, error) {
	return string(input), nil
}

func tool(name string) toolbox.Tool {
	return toolbox.Tool{
		Name:        name,
		Description: name + " tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     echo,
	}
}

func TestNew_Empty(t *testing.T) {
	tb := New()
	assert.Empty(t, tb.Tools())
}

func TestNew_MergesMultiple(t *testing.T) {
	tb1 := toolbox.New()
	tb1.Register(tool("ask_user"))

	tb2 := toolbox.New()
	tb2.Register(tool("fs_read"), tool("fs_write"))

	defaults := New(tb1, tb2)

	assert.Len(t, defaults.Tools(), 3)

	_, ok := defaults.Get("ask_user")
	assert.True(t, ok)

	_, ok = defaults.Get("fs_read")
	assert.True(t, ok)

	_, ok = defaults.Get("fs_write")
	assert.True(t, ok)
}

func TestNew_LaterOverwritesEarlier(t *testing.T) {
	tb1 := toolbox.New()
	tb1.Register(toolbox.Tool{Name: "x", Description: "first", Handler: echo})

	tb2 := toolbox.New()
	tb2.Register(toolbox.Tool{Name: "x", Description: "second", Handler: echo})

	defaults := New(tb1, tb2)

	got, ok := defaults.Get("x")
	assert.True(t, ok)
	assert.Equal(t, "second", got.Description)
}
