package sessions

import (
	"encoding/json"
	"path/filepath"
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

func TestMarshalWithAttachments_ExtractsImageData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Text{Text: "check this"},
				content.Image{Data: imgData, MediaType: "image/png"},
			},
		},
	}

	data, err := MarshalMessagesWithAttachments(msgs, store)
	require.NoError(t, err)

	// Verify JSON has attachment_ref, not inline data.
	var jmsgs []jsonMessage
	require.NoError(t, json.Unmarshal(data, &jmsgs))
	imgPart := jmsgs[0].Parts[1]
	assert.NotEmpty(t, imgPart.AttachmentRef)
	assert.Empty(t, imgPart.Data)
}

func TestUnmarshalWithAttachments_RestoresImageData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Image{Data: imgData, MediaType: "image/png"},
			},
		},
	}

	data, err := MarshalMessagesWithAttachments(msgs, store)
	require.NoError(t, err)

	got, err := UnmarshalMessagesWithAttachments(data, store)
	require.NoError(t, err)
	require.Len(t, got, 1)

	img := got[0].Parts[0].(content.Image)
	assert.Equal(t, imgData, img.Data)
	assert.Equal(t, "image/png", img.MediaType)
}

func TestUnmarshalWithAttachments_BackwardsCompatible(t *testing.T) {
	// Marshal WITHOUT attachments (old-style inline data).
	imgData := []byte("fake-image")
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Image{Data: imgData, MediaType: "image/png", URL: "http://example.com/img.png"},
			},
		},
	}

	data, err := MarshalMessages(msgs)
	require.NoError(t, err)

	// Unmarshal WITH attachment reader — should still work with inline data.
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)
	got, err := UnmarshalMessagesWithAttachments(data, store)
	require.NoError(t, err)
	require.Len(t, got, 1)

	img := got[0].Parts[0].(content.Image)
	assert.Equal(t, imgData, img.Data)
	assert.Equal(t, "http://example.com/img.png", img.URL)
}

func TestMarshalWithAttachments_Document_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	pdfData := []byte("%PDF-1.4 fake content")
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Text{Text: "Check this document"},
				content.Document{Path: "/tmp/report.pdf", Data: pdfData, MediaType: "application/pdf"},
			},
		},
	}

	data, err := MarshalMessagesWithAttachments(msgs, store)
	require.NoError(t, err)

	// Verify JSON has attachment_ref, not inline data.
	var jmsgs []jsonMessage
	require.NoError(t, json.Unmarshal(data, &jmsgs))
	docPart := jmsgs[0].Parts[1]
	assert.Equal(t, "document", docPart.Kind)
	assert.NotEmpty(t, docPart.AttachmentRef)
	assert.Empty(t, docPart.Data)
	assert.Equal(t, "/tmp/report.pdf", docPart.URL)

	// Round-trip: unmarshal restores data.
	got, err := UnmarshalMessagesWithAttachments(data, store)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)

	doc := got[0].Parts[1].(content.Document)
	assert.Equal(t, pdfData, doc.Data)
	assert.Equal(t, "application/pdf", doc.MediaType)
	assert.Equal(t, "/tmp/report.pdf", doc.Path)
}

func TestMarshalUnmarshal_MixedTextImageDocument(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Text{Text: "Here are my files"},
				content.Image{Data: []byte("img-bytes"), MediaType: "image/png"},
				content.Document{Path: "spec.pdf", Data: []byte("pdf-bytes"), MediaType: "application/pdf"},
			},
		},
	}

	data, err := MarshalMessagesWithAttachments(msgs, store)
	require.NoError(t, err)

	got, err := UnmarshalMessagesWithAttachments(data, store)
	require.NoError(t, err)
	require.Len(t, got[0].Parts, 3)

	assert.Equal(t, content.Text{Text: "Here are my files"}, got[0].Parts[0])

	img := got[0].Parts[1].(content.Image)
	assert.Equal(t, []byte("img-bytes"), img.Data)

	doc := got[0].Parts[2].(content.Document)
	assert.Equal(t, []byte("pdf-bytes"), doc.Data)
	assert.Equal(t, "spec.pdf", doc.Path)
}

func TestMarshalWithAttachments_DocumentNoData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Document{Path: "/tmp/report.pdf", MediaType: "application/pdf"},
			},
		},
	}

	data, err := MarshalMessagesWithAttachments(msgs, store)
	require.NoError(t, err)

	var jmsgs []jsonMessage
	require.NoError(t, json.Unmarshal(data, &jmsgs))
	docPart := jmsgs[0].Parts[0]
	assert.Empty(t, docPart.AttachmentRef)
	assert.Equal(t, "/tmp/report.pdf", docPart.URL)
}

func TestMarshalWithAttachments_URLOnlyImage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Image{URL: "http://example.com/img.png", MediaType: "image/png"},
			},
		},
	}

	data, err := MarshalMessagesWithAttachments(msgs, store)
	require.NoError(t, err)

	// Verify no attachment_ref for URL-only images.
	var jmsgs []jsonMessage
	require.NoError(t, json.Unmarshal(data, &jmsgs))
	imgPart := jmsgs[0].Parts[0]
	assert.Empty(t, imgPart.AttachmentRef)
	assert.Equal(t, "http://example.com/img.png", imgPart.URL)
}
