# Chat Data Model Foundation

## Overview

The `pkg/chats` package provides a provider-agnostic data model for LLM chat interactions. It is the foundational layer that all other Shelly components build upon — provider adapters, the agent system, and the engine all use these types. Zero dependencies on other `pkg/` packages.

## Sub-Package Structure

```
pkg/chats/
├── doc.go           # Package-level documentation (no code)
├── role/role.go     # Conversation roles
├── content/content.go  # Multi-modal content parts
├── message/message.go  # Message type
└── chat/chat.go     # Thread-safe conversation container
```

---

## role — Conversation Roles

**Package:** `pkg/chats/role`

```go
type Role string

const (
    System    Role = "system"
    User      Role = "user"
    Assistant Role = "assistant"
    Tool      Role = "tool"
)
```

- `(r Role) Valid() bool` — returns true if `r` is one of the four known roles.

---

## content — Multi-Modal Content Parts

**Package:** `pkg/chats/content`

### Part Interface

```go
type Part interface {
    PartKind() string
}
```

All content types implement `Part`. External packages can add custom types. `PartKind()` returns a string tag used for type-routing and serialization.

### Text

```go
type Text struct {
    Text string
}
func (t Text) PartKind() string { return "text" }
```

### Image

```go
type Image struct {
    Data      []byte
    MediaType string
}
func (i Image) PartKind() string { return "image" }
```

Binary image data with a MIME type (e.g. `"image/png"`). NOT a URL — raw bytes.

### Document

```go
type Document struct {
    Data      []byte
    MediaType string
}
func (d Document) PartKind() string { return "document" }
```

Binary document data (e.g. PDFs) with MIME type. Same shape as `Image`.

### ToolCall

```go
type ToolCall struct {
    ID        string
    Name      string
    Arguments string
    Metadata  map[string]string
}
func (tc ToolCall) PartKind() string { return "tool_call" }
```

- `ID` — unique call identifier (set by the LLM provider).
- `Name` — tool name being invoked.
- `Arguments` — **raw JSON string** of arguments.
- `Metadata` — provider-specific key-value pairs.

### ToolResult

```go
type ToolResult struct {
    CallID  string
    Content string
    IsError bool
}
func (tr ToolResult) PartKind() string { return "tool_result" }
```

- `CallID` — references the `ToolCall.ID` this result answers.
- `Content` — string output from the tool.
- `IsError` — true if the tool execution failed.

---

## message — Conversation Messages

**Package:** `pkg/chats/message`

### Message Type

```go
type Message struct {
    Sender   string
    Role     role.Role
    Parts    []content.Part
    Metadata map[string]any
}
```

Value type (copies cheaply). `Sender` identifies the originator (e.g. agent name). `Metadata` carries arbitrary provider/agent data.

### Constructors & Helpers

```go
func New(sender string, r role.Role, parts ...content.Part) Message
```

Creates a message with the given sender, role, and optional parts.

```go
func SetMeta(m Message, key string, value any) Message
```

**Free function** (not a method). Returns a copy of `m` with `Metadata[key] = value`. Initializes the map if nil.

### Content Extraction

```go
func (m Message) TextContent() string
```

Joins all `content.Text` parts with newlines and returns the combined string. Skips non-text parts.

```go
func (m Message) ToolCalls() []content.ToolCall
```

Returns all `content.ToolCall` parts in the message.

```go
func (m Message) ToolResults() []content.ToolResult
```

Returns all `content.ToolResult` parts in the message.

### Metadata Access

```go
func (m Message) Meta(key string) any
```

Returns `Metadata[key]` or `nil` if the map is nil or key is absent.

---

## chat — Conversation Container

**Package:** `pkg/chats/chat`

### Chat Type

```go
type Chat struct {
    // unexported fields: mu sync.Mutex, msgs []message.Message, meta map[string]any, etc.
}
```

Mutable, **thread-safe** conversation container. The zero value (`&Chat{}`) is ready to use.

### Adding Messages

```go
func (c *Chat) Append(msgs ...message.Message)
```

Appends one or more messages. Signals any goroutine blocked in `Wait`.

### Reading Messages

```go
func (c *Chat) Messages() []message.Message
```

Returns a **copy** of all messages (safe to iterate without holding the lock).

```go
func (c *Chat) Len() int
```

Returns the number of messages.

```go
func (c *Chat) At(i int) message.Message
```

Returns the message at index `i`.

```go
func (c *Chat) Last() (message.Message, bool)
```

Returns the last message and `true`, or zero value and `false` if empty.

```go
func (c *Chat) BySender(sender string) []message.Message
```

Returns all messages from the given sender.

```go
func (c *Chat) Since(offset int) []message.Message
```

Returns deep-copied messages from `offset` onward.

### Modification

```go
func (c *Chat) Replace(msgs ...message.Message)
```

Replaces the entire message list. Also signals waiters.

### System Prompt

```go
func (c *Chat) SystemPrompt() string
```

Returns the text of the first system message, or empty string if none.

### Synchronization — Wait

```go
func (c *Chat) Wait(ctx context.Context, n int) (int, error)
```

Blocks until the chat contains **more than `n` messages** or the context is cancelled. Returns the current message count. There is no separate `Signal()` method — `Append` and `Replace` signal implicitly by closing an internal channel.

Typical cursor-based usage:
```go
cursor := 0
for {
    cursor, err = chat.Wait(ctx, cursor)
    if err != nil { break }
    msgs := chat.Since(cursor)
    // process msgs
    cursor += len(msgs)
}
```

---

## Key Patterns

- **Value types for messages**: `Message` is a struct, not a pointer. Copy-safe.
- **Interface-based content**: The `Part` interface allows extending content types without modifying existing code.
- **Thread-safe chat**: All `Chat` methods lock internally, enabling concurrent reads/writes from agent loops and tool executors.
- **Wait/Append signaling**: `Wait(ctx, n)` blocks until `len > n`; `Append`/`Replace` close an internal channel to wake waiters — no explicit `Signal()` needed.
- **Free-function SetMeta**: `message.SetMeta(m, k, v)` returns a new message instead of mutating, preserving value semantics.
