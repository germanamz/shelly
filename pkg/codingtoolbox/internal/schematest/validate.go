// Package schematest provides test helpers for validating tool JSON Schemas.
package schematest

import (
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ValidateTools checks that every tool in the toolbox has a well-formed JSON
// Schema as its InputSchema. It verifies:
//   - The schema is valid JSON
//   - The top-level type is "object"
//   - Required fields reference properties that actually exist
//   - Property types are valid JSON Schema types
func ValidateTools(t *testing.T, tb *toolbox.ToolBox) {
	t.Helper()

	tools := tb.Tools()
	require.NotEmpty(t, tools, "toolbox should contain at least one tool")

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			validateTool(t, tool)
		})
	}
}

func validateTool(t *testing.T, tool toolbox.Tool) {
	t.Helper()

	require.NotEmpty(t, tool.Name, "tool name must not be empty")
	require.NotEmpty(t, tool.Description, "tool %q: description must not be empty", tool.Name)
	require.NotNil(t, tool.Handler, "tool %q: handler must not be nil", tool.Name)
	require.NotNil(t, tool.InputSchema, "tool %q: InputSchema must not be nil", tool.Name)
	assert.True(t, json.Valid(tool.InputSchema), "tool %q: InputSchema is not valid JSON: %s", tool.Name, string(tool.InputSchema))

	var schema map[string]any
	require.NoError(t, json.Unmarshal(tool.InputSchema, &schema), "tool %q: InputSchema unmarshal failed", tool.Name)

	assert.Equal(t, "object", schema["type"], "tool %q: top-level type must be \"object\"", tool.Name)

	props, _ := schema["properties"].(map[string]any)

	if required, ok := schema["required"]; ok {
		reqArr, ok := required.([]any)
		require.True(t, ok, "tool %q: required must be an array", tool.Name)

		for _, r := range reqArr {
			name, ok := r.(string)
			require.True(t, ok, "tool %q: required entry must be a string", tool.Name)
			assert.Contains(t, props, name, "tool %q: required field %q not found in properties", tool.Name, name)
		}
	}

	validTypes := map[string]bool{
		"string": true, "integer": true, "number": true,
		"boolean": true, "object": true, "array": true, "null": true,
	}

	for name, prop := range props {
		propMap, ok := prop.(map[string]any)
		require.True(t, ok, "tool %q: property %q must be an object", tool.Name, name)

		if typ, ok := propMap["type"]; ok {
			typStr, ok := typ.(string)
			require.True(t, ok, "tool %q: property %q type must be a string", tool.Name, name)
			assert.True(t, validTypes[typStr], "tool %q: property %q has invalid type %q", tool.Name, name, typStr)
		}
	}
}
