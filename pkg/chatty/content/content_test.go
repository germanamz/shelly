package content

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestText_PartKind(t *testing.T) {
	p := Text{Text: "hello"}
	assert.Equal(t, "text", p.PartKind())
}

func TestImage_PartKind(t *testing.T) {
	p := Image{URL: "https://example.com/img.png", MediaType: "image/png"}
	assert.Equal(t, "image", p.PartKind())
}

func TestToolCall_PartKind(t *testing.T) {
	p := ToolCall{ID: "1", Name: "search", Arguments: `{"q":"go"}`}
	assert.Equal(t, "tool_call", p.PartKind())
}

func TestToolResult_PartKind(t *testing.T) {
	p := ToolResult{ToolCallID: "1", Content: "result", IsError: false}
	assert.Equal(t, "tool_result", p.PartKind())
}

func TestPart_Interface(t *testing.T) {
	parts := []Part{
		Text{Text: "hi"},
		Image{URL: "u"},
		ToolCall{ID: "1"},
		ToolResult{ToolCallID: "1"},
	}

	expected := []string{"text", "image", "tool_call", "tool_result"}
	for i, p := range parts {
		assert.Equal(t, expected[i], p.PartKind())
	}
}
