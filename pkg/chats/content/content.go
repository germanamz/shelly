// Package content defines multi-modal content parts for LLM messages.
package content

// Part is a piece of content within a message.
// External packages can implement this interface to add custom content types.
type Part interface {
	PartKind() string
}

// Text is a plain text content part.
type Text struct {
	Text string
}

func (t Text) PartKind() string { return "text" }

// Image is an image content part, referenced by URL or embedded as raw bytes.
type Image struct {
	URL       string
	Data      []byte
	MediaType string
}

func (i Image) PartKind() string { return "image" }

// Document is a document content part (PDF, DOCX, etc.) referenced by path or embedded as raw bytes.
type Document struct {
	Path      string // Original file path (for display)
	Data      []byte // Raw document bytes
	MediaType string // MIME type (application/pdf, etc.)
}

func (d Document) PartKind() string { return "document" }

// ToolCall represents an assistant's request to invoke a tool.
// Arguments holds the raw JSON string to avoid unnecessary deserialization.
// Metadata carries provider-specific opaque data (e.g. Gemini thought signatures)
// that must survive round-trips through the conversation history.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
	Metadata  map[string]string
}

func (tc ToolCall) PartKind() string { return "tool_call" }

// ToolResult holds the output of a tool invocation.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

func (tr ToolResult) PartKind() string { return "tool_result" }
