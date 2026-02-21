package toolbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/germanamz/shelly/pkg/chats/content"
)

// ToolBox orchestrates a collection of tools. It allows registering, retrieving,
// listing, and calling tools. Agents use ToolBox to execute tool calls.
type ToolBox struct {
	tools map[string]Tool
}

// New creates a new ToolBox ready for use.
func New() *ToolBox {
	return &ToolBox{
		tools: make(map[string]Tool),
	}
}

// Register adds one or more tools to the ToolBox. If a tool with the same name
// already exists, it is replaced.
func (tb *ToolBox) Register(tools ...Tool) {
	for _, t := range tools {
		tb.tools[t.Name] = t
	}
}

// Get returns a tool by name and a boolean indicating whether it was found.
func (tb *ToolBox) Get(name string) (Tool, bool) {
	t, ok := tb.tools[name]
	return t, ok
}

// Merge registers all tools from another ToolBox into this one. If a tool
// with the same name already exists, it is replaced.
func (tb *ToolBox) Merge(other *ToolBox) {
	for _, t := range other.tools {
		tb.tools[t.Name] = t
	}
}

// Tools returns all registered tools as a slice.
func (tb *ToolBox) Tools() []Tool {
	result := make([]Tool, 0, len(tb.tools))
	for _, t := range tb.tools {
		result = append(result, t)
	}
	return result
}

// Call executes a tool call and returns a ToolResult. If the tool is not found
// or the handler returns an error, the result will have IsError set to true.
func (tb *ToolBox) Call(ctx context.Context, tc content.ToolCall) content.ToolResult {
	t, ok := tb.tools[tc.Name]
	if !ok {
		return content.ToolResult{
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("tool not found: %s", tc.Name),
			IsError:    true,
		}
	}

	result, err := t.Handler(ctx, json.RawMessage(tc.Arguments))
	if err != nil {
		return content.ToolResult{
			ToolCallID: tc.ID,
			Content:    err.Error(),
			IsError:    true,
		}
	}

	return content.ToolResult{
		ToolCallID: tc.ID,
		Content:    result,
	}
}
