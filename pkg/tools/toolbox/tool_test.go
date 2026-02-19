package toolbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolHandler(t *testing.T) {
	tool := Tool{
		Name:        "echo",
		Description: "Echoes input back",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var params struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", err
			}
			return params.Text, nil
		},
	}

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"text":"hello"}`))
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestToolFields(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	tool := Tool{
		Name:        "test",
		Description: "A test tool",
		InputSchema: schema,
	}

	assert.Equal(t, "test", tool.Name)
	assert.Equal(t, "A test tool", tool.Description)
	assert.JSONEq(t, `{"type":"object"}`, string(tool.InputSchema))
	assert.Nil(t, tool.Handler)
}
