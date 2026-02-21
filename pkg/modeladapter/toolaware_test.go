package modeladapter_test

import (
	"testing"

	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/providers/anthropic"
	"github.com/germanamz/shelly/pkg/providers/openai"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
)

func TestAnthropicAdapter_ImplementsToolAware(t *testing.T) {
	a := anthropic.New("https://api.anthropic.com", "key", "model")
	var ta modeladapter.ToolAware = a
	tools := []toolbox.Tool{{Name: "test"}}
	ta.SetTools(tools)
	assert.Equal(t, tools, a.Tools)
}

func TestOpenAIAdapter_ImplementsToolAware(t *testing.T) {
	a := openai.New("https://api.openai.com", "key", "model")
	var ta modeladapter.ToolAware = a
	tools := []toolbox.Tool{{Name: "test"}}
	ta.SetTools(tools)
	assert.Equal(t, tools, a.Tools)
}
