# chats

A provider-agnostic data model for LLM chat interactions. Chats defines the core types — roles, content parts, messages, and conversations — without coupling to any specific LLM provider or API. It serves as a foundation layer that adapters can build on.

## Architecture

```
chats/
├── role/       Conversation roles (system, user, assistant, tool)
├── content/    Multi-modal content parts (text, image, tool call/result)
├── message/    Messages composed of a sender, role, and content parts
└── chat/       Mutable conversation container
```

### `role` — Conversation Roles

Defines `Role`, a string type with four constants: `System`, `User`, `Assistant`, and `Tool`. Includes `Valid()` for validation and `String()` for the underlying value.

### `content` — Multi-Modal Content Parts

Defines the `Part` interface and four concrete implementations:

| Type         | Kind            | Description                                |
|--------------|-----------------|--------------------------------------------|
| `Text`       | `"text"`        | Plain text                                 |
| `Image`      | `"image"`       | Image by URL or embedded bytes             |
| `ToolCall`   | `"tool_call"`   | Assistant's request to invoke a tool       |
| `ToolResult` | `"tool_result"` | Output from a tool invocation              |

The `Part` interface has a single method (`PartKind() string`), making it straightforward to add custom content types.

### `message` — Conversation Messages

A `Message` combines a `Sender`, `Role`, a slice of `Part` values, and an optional metadata map.

- `New(sender, role, ...parts)` / `NewText(sender, role, text)` — constructors
- `TextContent()` — concatenates all `Text` parts
- `ToolCalls()` — extracts all `ToolCall` parts
- `SetMeta(key, value)` / `GetMeta(key)` — arbitrary key-value metadata

The `Sender` field identifies who produced the message (e.g., an agent name), making it easy to track participants in multi-agent conversations. `Message` is a value type.

### `chat` — Conversation Container

`Chat` is a mutable, ordered collection of messages. The zero value is ready to use.

- `New(...messages)` — constructor with initial messages
- `Append(...messages)` — add messages
- `Replace(...messages)` — atomically swap all messages
- `Len()` — message count
- `At(index)` / `Last()` — access messages
- `Messages()` — defensive copy of all messages
- `Each(fn)` — iterate with early-stop support
- `BySender(sender)` — filter messages by sender
- `SystemPrompt()` — text of the first system message

`Chat` is safe for concurrent use. Both `Append` and `Replace` signal waiters blocked in `Wait`.

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
    content.ToolResult{ToolCallID: "call_1", Content: "Go is a statically typed language..."},
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

// Fork into independent branches
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
