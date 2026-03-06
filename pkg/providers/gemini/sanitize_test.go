package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeSchema_Combinators(t *testing.T) {
	input := json.RawMessage(`{
		"oneOf": [
			{"type":"string","$schema":"http://json-schema.org/draft-07/schema#"},
			{"type":"object","additionalProperties":true,"properties":{"x":{"type":"integer","additionalProperties":false}}}
		],
		"anyOf": [
			{"$schema":"draft","type":"number"}
		],
		"allOf": [
			{"additionalProperties":true,"type":"boolean"}
		]
	}`)

	got := sanitizeSchema(input)

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got, &m))

	// oneOf
	var oneOf []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(m["oneOf"], &oneOf))
	assert.NotContains(t, oneOf[0], "$schema")
	assert.NotContains(t, oneOf[1], "additionalProperties")
	// nested property inside oneOf element
	var innerProps map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(oneOf[1]["properties"], &innerProps))
	assert.NotContains(t, innerProps["x"], "additionalProperties")

	// anyOf
	var anyOf []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(m["anyOf"], &anyOf))
	assert.NotContains(t, anyOf[0], "$schema")

	// allOf
	var allOf []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(m["allOf"], &allOf))
	assert.NotContains(t, allOf[0], "additionalProperties")
}
