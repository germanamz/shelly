package sessions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	msgs := []message.Message{
		message.NewText("", role.System, "You are helpful."),
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Text{Text: "Describe this:"},
				content.Image{URL: "https://example.com/img.png", Data: []byte("fake"), MediaType: "image/png"},
			},
		},
		{
			Sender: "bot",
			Role:   role.Assistant,
			Parts: []content.Part{
				content.Text{Text: "Let me search."},
				content.ToolCall{
					ID:        "call_1",
					Name:      "search",
					Arguments: `{"q":"go"}`,
					Metadata:  map[string]string{"provider_key": "val"},
				},
			},
		},
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "call_1", Content: "Go is great", IsError: false},
		),
	}

	// Add metadata to one message
	message.SetMeta(&msgs[0], "model", "test-model")

	data, err := MarshalMessages(msgs)
	require.NoError(t, err)

	got, err := UnmarshalMessages(data)
	require.NoError(t, err)

	require.Len(t, got, len(msgs))

	// System message
	assert.Equal(t, role.System, got[0].Role)
	assert.Equal(t, "You are helpful.", got[0].TextContent())
	assert.Equal(t, "test-model", got[0].Metadata["model"])

	// User message with text + image
	assert.Equal(t, "user", got[1].Sender)
	assert.Equal(t, role.User, got[1].Role)
	require.Len(t, got[1].Parts, 2)
	assert.Equal(t, content.Text{Text: "Describe this:"}, got[1].Parts[0])
	img := got[1].Parts[1].(content.Image)
	assert.Equal(t, "https://example.com/img.png", img.URL)
	assert.Equal(t, []byte("fake"), img.Data)
	assert.Equal(t, "image/png", img.MediaType)

	// Assistant message with text + tool call
	assert.Equal(t, role.Assistant, got[2].Role)
	require.Len(t, got[2].Parts, 2)
	tc := got[2].Parts[1].(content.ToolCall)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "search", tc.Name)
	assert.JSONEq(t, `{"q":"go"}`, tc.Arguments)
	assert.Equal(t, "val", tc.Metadata["provider_key"])

	// Tool result
	tr := got[3].Parts[0].(content.ToolResult)
	assert.Equal(t, "call_1", tr.ToolCallID)
	assert.Equal(t, "Go is great", tr.Content)
	assert.False(t, tr.IsError)
}

func TestMarshalUnmarshal_EmptyMessages(t *testing.T) {
	data, err := MarshalMessages(nil)
	require.NoError(t, err)

	got, err := UnmarshalMessages(data)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMarshalUnmarshal_EmptyParts(t *testing.T) {
	msgs := []message.Message{
		{Sender: "x", Role: role.User, Parts: nil},
	}

	data, err := MarshalMessages(msgs)
	require.NoError(t, err)

	got, err := UnmarshalMessages(data)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].Parts)
}

func TestUnmarshalMessages_InvalidJSON(t *testing.T) {
	_, err := UnmarshalMessages([]byte("not json"))
	assert.Error(t, err)
}

func TestMarshalUnmarshal_ToolResultWithError(t *testing.T) {
	msgs := []message.Message{
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "failed", IsError: true},
		),
	}

	data, err := MarshalMessages(msgs)
	require.NoError(t, err)

	got, err := UnmarshalMessages(data)
	require.NoError(t, err)

	tr := got[0].Parts[0].(content.ToolResult)
	assert.True(t, tr.IsError)
	assert.Equal(t, "failed", tr.Content)
}
