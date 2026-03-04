package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/tools/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type simpleInput struct {
	Name   string `json:"name" desc:"The name"`
	Age    int    `json:"age,omitempty" desc:"The age"`
	Active bool   `json:"active,omitempty"`
}

func TestGenerate_Simple(t *testing.T) {
	raw := schema.Generate[simpleInput]()

	var s map[string]any
	require.NoError(t, json.Unmarshal(raw, &s))

	assert.Equal(t, "object", s["type"])

	props := s["properties"].(map[string]any)
	assert.Len(t, props, 3)

	nameProp := props["name"].(map[string]any)
	assert.Equal(t, "string", nameProp["type"])
	assert.Equal(t, "The name", nameProp["description"])

	ageProp := props["age"].(map[string]any)
	assert.Equal(t, "integer", ageProp["type"])
	assert.Equal(t, "The age", ageProp["description"])

	activeProp := props["active"].(map[string]any)
	assert.Equal(t, "boolean", activeProp["type"])
	assert.Nil(t, activeProp["description"])

	required := toStringSlice(s["required"])
	assert.Equal(t, []string{"name"}, required)
}

type arrayInput struct {
	Tags []string `json:"tags" desc:"List of tags"`
}

func TestGenerate_ArrayOfStrings(t *testing.T) {
	raw := schema.Generate[arrayInput]()

	var s map[string]any
	require.NoError(t, json.Unmarshal(raw, &s))

	props := s["properties"].(map[string]any)
	tagsProp := props["tags"].(map[string]any)
	assert.Equal(t, "array", tagsProp["type"])
	assert.Equal(t, "List of tags", tagsProp["description"])

	items := tagsProp["items"].(map[string]any)
	assert.Equal(t, "string", items["type"])
}

type mapInput struct {
	Headers map[string]string `json:"headers,omitempty" desc:"Request headers"`
}

func TestGenerate_MapOfStrings(t *testing.T) {
	raw := schema.Generate[mapInput]()

	var s map[string]any
	require.NoError(t, json.Unmarshal(raw, &s))

	props := s["properties"].(map[string]any)
	headersProp := props["headers"].(map[string]any)
	assert.Equal(t, "object", headersProp["type"])
	assert.Equal(t, "Request headers", headersProp["description"])

	addlProps := headersProp["additionalProperties"].(map[string]any)
	assert.Equal(t, "string", addlProps["type"])

	assert.Nil(t, s["required"])
}

type innerStruct struct {
	OldText string `json:"old_text" desc:"Text to find"`
	NewText string `json:"new_text,omitempty" desc:"Replacement text"`
}

type nestedInput struct {
	Path  string        `json:"path" desc:"File path"`
	Hunks []innerStruct `json:"hunks" desc:"Hunks to apply"`
}

func TestGenerate_NestedStruct(t *testing.T) {
	raw := schema.Generate[nestedInput]()

	var s map[string]any
	require.NoError(t, json.Unmarshal(raw, &s))

	props := s["properties"].(map[string]any)
	hunksProp := props["hunks"].(map[string]any)
	assert.Equal(t, "array", hunksProp["type"])

	items := hunksProp["items"].(map[string]any)
	assert.Equal(t, "object", items["type"])

	innerProps := items["properties"].(map[string]any)
	assert.Contains(t, innerProps, "old_text")
	assert.Contains(t, innerProps, "new_text")

	innerRequired := toStringSlice(items["required"])
	assert.Equal(t, []string{"old_text"}, innerRequired)
}

type emptyInput struct{}

func TestGenerate_EmptyStruct(t *testing.T) {
	raw := schema.Generate[emptyInput]()

	var s map[string]any
	require.NoError(t, json.Unmarshal(raw, &s))

	assert.Equal(t, "object", s["type"])
	assert.Nil(t, s["properties"])
	assert.Nil(t, s["required"])
}

func TestGenerate_ValidJSON(t *testing.T) {
	raw := schema.Generate[simpleInput]()
	assert.True(t, json.Valid(raw))
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}

	arr := v.([]any)
	result := make([]string, len(arr))
	for i, item := range arr {
		result[i] = item.(string)
	}

	return result
}
