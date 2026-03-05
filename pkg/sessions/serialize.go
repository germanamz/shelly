// Package sessions provides session persistence with JSON serialization and file-based storage.
package sessions

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

type jsonPart struct {
	Kind          string            `json:"kind"`
	Text          string            `json:"text,omitempty"`
	URL           string            `json:"url,omitempty"`
	Data          []byte            `json:"data,omitempty"`
	MediaType     string            `json:"media_type,omitempty"`
	ID            string            `json:"id,omitempty"`
	Name          string            `json:"name,omitempty"`
	Arguments     string            `json:"arguments,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	ToolCallID    string            `json:"tool_call_id,omitempty"`
	Content       string            `json:"content,omitempty"`
	IsError       bool              `json:"is_error,omitempty"`
	AttachmentRef string            `json:"attachment_ref,omitempty"`
}

type jsonMessage struct {
	Sender   string         `json:"sender"`
	Role     string         `json:"role"`
	Parts    []jsonPart     `json:"parts"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func partToJSON(p content.Part) jsonPart {
	return partToJSONWithAttachments(p, nil)
}

func partToJSONWithAttachments(p content.Part, w AttachmentWriter) jsonPart {
	switch v := p.(type) {
	case content.Text:
		return jsonPart{Kind: "text", Text: v.Text}
	case content.Image:
		jp := jsonPart{Kind: "image", URL: v.URL, MediaType: v.MediaType}
		if len(v.Data) > 0 && w != nil {
			ref, err := w.WriteAttachment(v.Data, v.MediaType)
			if err != nil {
				slog.Warn("sessions: failed to write attachment, falling back to inline", "err", err)
				jp.Data = v.Data
			} else {
				jp.AttachmentRef = ref
			}
		} else {
			jp.Data = v.Data
		}
		return jp
	case content.Document:
		jp := jsonPart{Kind: "document", URL: v.Path, MediaType: v.MediaType}
		if len(v.Data) > 0 && w != nil {
			ref, err := w.WriteAttachment(v.Data, v.MediaType)
			if err != nil {
				slog.Warn("sessions: failed to write document attachment, falling back to inline", "err", err)
				jp.Data = v.Data
			} else {
				jp.AttachmentRef = ref
			}
		} else {
			jp.Data = v.Data
		}
		return jp
	case content.ToolCall:
		return jsonPart{Kind: "tool_call", ID: v.ID, Name: v.Name, Arguments: v.Arguments, Metadata: v.Metadata}
	case content.ToolResult:
		return jsonPart{Kind: "tool_result", ToolCallID: v.ToolCallID, Content: v.Content, IsError: v.IsError}
	default:
		slog.Warn("sessions: skipping unknown part kind", "kind", p.PartKind())
		return jsonPart{Kind: p.PartKind()}
	}
}

func jsonToPart(jp jsonPart) (content.Part, bool) {
	return jsonToPartWithAttachments(jp, nil)
}

func jsonToPartWithAttachments(jp jsonPart, r AttachmentReader) (content.Part, bool) {
	switch jp.Kind {
	case "text":
		return content.Text{Text: jp.Text}, true
	case "image":
		img := content.Image{URL: jp.URL, MediaType: jp.MediaType}
		if jp.AttachmentRef != "" && r != nil {
			data, mediaType, err := r.ReadAttachment(jp.AttachmentRef)
			if err != nil {
				slog.Warn("sessions: failed to read attachment", "ref", jp.AttachmentRef, "err", err)
			} else {
				img.Data = data
				if mediaType != "" {
					img.MediaType = mediaType
				}
			}
		} else {
			img.Data = jp.Data
		}
		return img, true
	case "document":
		doc := content.Document{Path: jp.URL, MediaType: jp.MediaType}
		if jp.AttachmentRef != "" && r != nil {
			data, mediaType, err := r.ReadAttachment(jp.AttachmentRef)
			if err != nil {
				slog.Warn("sessions: failed to read document attachment", "ref", jp.AttachmentRef, "err", err)
			} else {
				doc.Data = data
				if mediaType != "" {
					doc.MediaType = mediaType
				}
			}
		} else {
			doc.Data = jp.Data
		}
		return doc, true
	case "tool_call":
		return content.ToolCall{ID: jp.ID, Name: jp.Name, Arguments: jp.Arguments, Metadata: jp.Metadata}, true
	case "tool_result":
		return content.ToolResult{ToolCallID: jp.ToolCallID, Content: jp.Content, IsError: jp.IsError}, true
	default:
		slog.Warn("sessions: skipping unknown part kind on unmarshal", "kind", jp.Kind)
		return nil, false
	}
}

// MarshalMessages serializes a slice of messages to JSON.
func MarshalMessages(msgs []message.Message) ([]byte, error) {
	jmsgs := make([]jsonMessage, len(msgs))
	for i, m := range msgs {
		parts := make([]jsonPart, 0, len(m.Parts))
		for _, p := range m.Parts {
			parts = append(parts, partToJSON(p))
		}
		jmsgs[i] = jsonMessage{
			Sender:   m.Sender,
			Role:     m.Role.String(),
			Parts:    parts,
			Metadata: m.Metadata,
		}
	}
	return json.Marshal(jmsgs)
}

// MarshalMessagesWithAttachments serializes messages to JSON, extracting binary
// data into the provided AttachmentWriter instead of embedding it inline.
func MarshalMessagesWithAttachments(msgs []message.Message, w AttachmentWriter) ([]byte, error) {
	jmsgs := make([]jsonMessage, len(msgs))
	for i, m := range msgs {
		parts := make([]jsonPart, 0, len(m.Parts))
		for _, p := range m.Parts {
			parts = append(parts, partToJSONWithAttachments(p, w))
		}
		jmsgs[i] = jsonMessage{
			Sender:   m.Sender,
			Role:     m.Role.String(),
			Parts:    parts,
			Metadata: m.Metadata,
		}
	}
	return json.Marshal(jmsgs)
}

// UnmarshalMessages deserializes JSON into a slice of messages.
func UnmarshalMessages(data []byte) ([]message.Message, error) {
	var jmsgs []jsonMessage
	if err := json.Unmarshal(data, &jmsgs); err != nil {
		return nil, fmt.Errorf("sessions: unmarshal messages: %w", err)
	}

	msgs := make([]message.Message, len(jmsgs))
	for i, jm := range jmsgs {
		r := role.Role(jm.Role)
		if !r.Valid() {
			slog.Warn("sessions: unknown role, keeping as-is", "role", jm.Role)
		}

		var parts []content.Part
		for _, jp := range jm.Parts {
			if p, ok := jsonToPart(jp); ok {
				parts = append(parts, p)
			}
		}

		msgs[i] = message.Message{
			Sender:   jm.Sender,
			Role:     r,
			Parts:    parts,
			Metadata: jm.Metadata,
		}
	}
	return msgs, nil
}

// UnmarshalMessagesWithAttachments deserializes JSON into messages, loading
// binary data from the provided AttachmentReader for parts that reference attachments.
func UnmarshalMessagesWithAttachments(data []byte, ar AttachmentReader) ([]message.Message, error) {
	var jmsgs []jsonMessage
	if err := json.Unmarshal(data, &jmsgs); err != nil {
		return nil, fmt.Errorf("sessions: unmarshal messages: %w", err)
	}

	msgs := make([]message.Message, len(jmsgs))
	for i, jm := range jmsgs {
		r := role.Role(jm.Role)
		if !r.Valid() {
			slog.Warn("sessions: unknown role, keeping as-is", "role", jm.Role)
		}

		var parts []content.Part
		for _, jp := range jm.Parts {
			if p, ok := jsonToPartWithAttachments(jp, ar); ok {
				parts = append(parts, p)
			}
		}

		msgs[i] = message.Message{
			Sender:   jm.Sender,
			Role:     r,
			Parts:    parts,
			Metadata: jm.Metadata,
		}
	}
	return msgs, nil
}
