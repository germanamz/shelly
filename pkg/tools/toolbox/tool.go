package toolbox

import (
	"context"
	"encoding/json"
)

// Handler executes a tool with the given JSON input and returns a text result.
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Tool represents an executable tool with a name, description, JSON Schema, and handler.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     Handler
}
