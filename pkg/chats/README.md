# chats

A provider-agnostic data model for LLM chat interactions. Chats defines the core types -- roles, content parts, messages, and conversations -- without coupling to any specific LLM provider or API. It serves as the foundation layer of Shelly that provider adapters and the agent system build on.

## Architecture

```
chats/
├── doc.go      Package-level documentation (no code)
├── role/       Conversation roles (system, user, assistant, tool)
├── content/    Multi-modal content parts (text, image, tool call/result)
├── message/    Messages composed of a sender, role, and content parts
└── chat/       Mutable, concurrency-safe conversation container
```

Sub-packages depend only on each other in a strict layering order: `role` has no dependencies, `content` has no dependencies, `message` imports `role` and `content`, and `chat` imports `message`, `content`, and `role`. The top-level `chats` package (`doc.go`) contains no code -- it exists only for documentation.

### `role` -- Conversation Roles

Defines `Role`, a string type with four constants: `System`, `User`, `Assistant`, and `Tool`.

**Exported API:**

- `type Role string`
- Constants: `System`, `User`, `Assistant`, `Tool`
- `(r Role) Valid() bool` -- reports whether `r` is one of the four known roles
- `(r Role) String() string` -- returns the underlying string value

### `content` -- Multi-Modal Content Parts

Defines the `Part` interface and four concrete implementations:

| Type         | Kind            | Fields                                   | Description                                  |
|--------------|-----------------|------------------------------------------|----------------------------------------------|
| `Text`       | `"text"`        | `Text string`                            | Plain text content                           |
| `Image`      | `"image"`       | `URL string`, `Data []byte`, `MediaType string` | Image by URL or embedded raw bytes     |
| `ToolCall`   | `"tool_call"`   | `ID string`, `Name string`, `Arguments string`  | Assistant's request to invoke a tool (Arguments is raw JSON) |
| `ToolResult` | `"tool_result"` | `ToolCallID string`, `Content string`, `IsError bool` | Output from a tool invocation          |

**Exported API:**

- `type Part interface { PartKind() string }` -- the single-method interface all content types implement
- `type Text struct` / `type Image struct` / `type ToolCall struct` / `type ToolResult struct`

The `Part` interface has a single method (`PartKind() string`), making it straightforward to add custom content types in external packages.

### `message` -- Conversation Messages

A `Message` combines a `Sender` string, a `Role`, a slice of `Part` values, and an optional `Metadata` map. `Message` is a value type that copies cheaply.

**Exported API:**

- `type Message struct { Sender string; Role role.Role; Parts []content.Part; Metadata map[string]any }`
- `New(sender string, r role.Role, parts ...content.Part) Message` -- constructor with arbitrary content parts
- `NewText(sender string, r role.Role, text string) Message` -- convenience constructor for a single `Text` part
- `(m Message) TextContent() string` -- concatenates the text of all `Text` parts in the message
- `(m Message) ToolCalls() []content.ToolCall` -- extracts all `ToolCall` parts from the message
- `(m *Message) SetMeta(key string, value any)` -- sets a metadata key-value pair (initializes the map if nil; pointer receiver)
- `(m Message) GetMeta(key string) (any, bool)` -- retrieves a metadata value by key

The `Sender` field identifies who produced the message (e.g., an agent name), making it easy to track participants in multi-agent conversations. Note that `SetMeta` uses a pointer receiver while all other methods use a value receiver.

### `chat` -- Conversation Container

`Chat` is a mutable, ordered collection of messages. The zero value is ready to use. All methods are safe for concurrent use via an internal `sync.RWMutex`.

**Exported API:**

- `type Chat struct` (unexported fields: `mu`, `once`, `signal`, `messages`)
- `New(msgs ...message.Message) *Chat` -- constructor with optional initial messages
- `(c *Chat) Append(msgs ...message.Message)` -- appends messages and notifies waiters
- `(c *Chat) Replace(msgs ...message.Message)` -- atomically swaps all messages and notifies waiters
- `(c *Chat) Len() int` -- returns the message count
- `(c *Chat) At(index int) message.Message` -- returns the message at the given index (panics if out of range)
- `(c *Chat) Last() (message.Message, bool)` -- returns the most recent message, or zero value and `false` if empty
- `(c *Chat) Messages() []message.Message` -- returns a deep copy of all messages (Parts slices and Metadata maps are independently copied so callers cannot mutate conversation data)
- `(c *Chat) Each(fn func(int, message.Message) bool)` -- iterates over messages with early-stop support; holds the read lock for the duration, so `fn` must not call other `Chat` methods (deadlock risk)
- `(c *Chat) BySender(sender string) []message.Message` -- returns all messages from the given sender
- `(c *Chat) SystemPrompt() string` -- returns the text content of the first system-role message, or empty string if none
- `(c *Chat) Since(offset int) []message.Message` -- returns a copy of messages starting from the given offset; returns nil if offset is out of range or negative
- `(c *Chat) Wait(ctx context.Context, n int) (int, error)` -- blocks until the chat contains more than `n` messages or the context is cancelled; returns the current message count

Both `Append` and `Replace` close an internal signal channel and re-create it, which wakes up all goroutines blocked in `Wait`. The `Wait` + `Since` pair is designed for streaming-style consumers that need to react to new messages as they arrive.

## Dependencies

`pkg/chats` has no dependencies on other `pkg/` packages. It uses only the Go standard library (`context`, `maps`, `strings`, `sync`).

## Examples

### Multi-Turn Conversation

```go
c := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("alice", role.User, "What is Go?"),
)
c.Append(message.NewText("bot", role.Assistant, "Go is a programming language."))

fmt.Println(c.SystemPrompt()) // "You are a helpful assistant."
fmt.Println(c.Len())          // 3
```

### Multi-Modal Messages

```go
msg := message.New("alice", role.User,
    content.Text{Text: "Describe this image:"},
    content.Image{URL: "https://example.com/photo.png", MediaType: "image/png"},
)
```

### Tool Use

```go
assistantMsg := message.New("bot", role.Assistant,
    content.Text{Text: "Let me search for that."},
    content.ToolCall{ID: "call_1", Name: "search", Arguments: `{"q":"golang"}`},
)
toolMsg := message.New("", role.Tool,
    content.ToolResult{ToolCallID: "call_1", Content: "Go is a statically typed language...", IsError: false},
)
c.Append(assistantMsg, toolMsg)
```

### Message Metadata

```go
msg := message.NewText("bot", role.Assistant, "Hello!")
msg.SetMeta("model", "claude-sonnet-4-5-20250929")
msg.SetMeta("tokens", 12)

model, _ := msg.GetMeta("model") // "claude-sonnet-4-5-20250929"
```

### Streaming with Wait and Since

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

cursor := 0
for {
    cursor, err = c.Wait(ctx, cursor)
    if err != nil {
        break
    }
    newMsgs := c.Since(cursor)
    for _, m := range newMsgs {
        fmt.Println(m.Sender, ":", m.TextContent())
    }
    cursor += len(newMsgs)
}
```

### Custom Content Types

```go
type Audio struct {
    URL      string
    Duration time.Duration
}

func (a Audio) PartKind() string { return "audio" }
```

### Filtering by Sender

```go
c.BySender("alice") // all messages where Sender == "alice"
```

### Multi-Agent Orchestration

```go
c := chat.New(
    message.NewText("", role.System, "Collaborative problem-solving session."),
    message.NewText("user", role.User, "Write a summary of Go's concurrency model."),
)

for _, agent := range []string{"researcher", "critic", "writer"} {
    response := dispatch(agent, c)
    c.Append(message.NewText(agent, role.Assistant, response))
}

// Retrieve a specific agent's contributions
for _, m := range c.BySender("critic") {
    fmt.Println("Critic:", m.TextContent())
}
```

### Conversation Branching

```go
shared := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("user", role.User, "Propose a name for a Go testing library."),
)

// Fork into independent branches (Messages() returns a deep copy)
creative := chat.New(shared.Messages()...)
creative.Append(message.NewText("creative", role.Assistant, "How about 'testigo'?"))

practical := chat.New(shared.Messages()...)
practical.Append(message.NewText("practical", role.Assistant, "I suggest 'goassert'."))
```

### Provider Adapter

```go
func toOpenAI(c *chat.Chat) []openai.Message {
    var out []openai.Message
    c.Each(func(_ int, m message.Message) bool {
        out = append(out, openai.Message{
            Role:    m.Role.String(),
            Content: m.TextContent(),
        })
        return true
    })
    return out
}
```
