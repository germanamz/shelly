# Chat Data Model Foundation

## Overview

The `pkg/chats` package provides a provider-agnostic data model for LLM chat interactions. It serves as the foundational layer that all other Shelly components build upon, defining core types for roles, content parts, messages, and conversations without coupling to any specific LLM provider or API.

## Architecture

The package follows a clean modular structure with four sub-packages:

```
chats/
├── role/      # Conversation roles (system, user, assistant, tool)
├── content/   # Multi-modal content parts (text, image, tool call/result) 
├── message/   # Message container with role, parts, and metadata
└── chat/      # Mutable conversation container with thread-safe operations
```

## Core Types

### Roles (`pkg/chats/role`)

Defines the four standard conversation roles:

```go
type Role string

const (
    System    Role = "system"    // System instructions/context
    User      Role = "user"      // Human input
    Assistant Role = "assistant" // LLM response
    Tool      Role = "tool"      // Tool execution results
)
```

**Key Methods:**
- `Valid() bool` - Validates role is one of the four known types
- `String() string` - Returns string representation

### Content Parts (`pkg/chats/content`)

Multi-modal content system supporting text, images, and tool interactions:

```go
type Part interface {
    PartKind() string
}

// Core implementations:
type Text struct { Text string }
type Image struct { URL string }
type ToolCall struct { ID, Name string; Args json.RawMessage }
type ToolResult struct { CallID, Result string }
```

**Design Patterns:**
- Extensible interface allows custom content types
- Each part declares its kind for serialization/routing
- Tool calls and results are linked via CallID
- JSON raw messages preserve argument structure

### Messages (`pkg/chats/message`)

Container for a single conversation message:

```go
type Message struct {
    Sender   string           // Agent identifier (optional)
    Role     role.Role        // Message role
    Parts    []content.Part   // Content parts (text, images, tools)
    Metadata map[string]any   // Extensible metadata
}
```

**Key Features:**
- Value type that copies cheaply
- Multi-modal content support via Parts slice
- Flexible metadata for provider-specific data
- Helper methods for common operations:
  - `TextParts() []string` - Extract all text content
  - `String() string` - Human-readable representation
  - `ToolCalls() []*content.ToolCall` - Extract tool calls
  - `Clone() Message` - Deep copy with new metadata map

### Conversations (`pkg/chats/chat`)

Thread-safe mutable conversation container:

```go
type Chat struct {
    // Unexported fields with RWMutex for concurrency
}
```

**Core Operations:**
- `Add(msg message.Message)` - Append message
- `Messages() []message.Message` - Get all messages (copy)
- `Clear()` - Remove all messages
- `Len() int` - Message count
- `Clone() *Chat` - Deep copy of conversation

**Convenience Methods:**
- `System(text string)` - Add system message
- `User(text string)` - Add user message  
- `Assistant(text string)` - Add assistant message
- `Tool(callID, result string)` - Add tool result

**Advanced Features:**
- `Compact(keepLast int)` - Remove old messages, keep recent ones
- `Filter(predicate func(message.Message) bool)` - Remove messages not matching predicate
- Thread-safe for concurrent access

## Design Principles

### Zero Dependencies
- No imports from other `pkg/` packages
- Only standard library dependencies
- Foundation layer that others build upon

### Provider Agnostic
- No coupling to specific LLM APIs (OpenAI, Anthropic, etc.)
- Generic enough to support all major providers
- Extensible content system accommodates provider differences

### Multi-Modal Support
- Text, images, and tool interactions as first-class citizens
- Extensible Part interface for future content types
- Proper linking between tool calls and results

### Thread Safety
- Chat operations are safe for concurrent use
- RWMutex provides efficient read/write access
- Value types (Message, Role) copy safely

### Clean Architecture
- Separation of concerns across sub-packages
- Interface-based extensibility
- Value objects where appropriate

## Usage Patterns

### Creating Messages
```go
// Simple text message
msg := message.New(role.User, content.Text{Text: "Hello"})

// Multi-modal message
msg := message.Message{
    Role: role.User,
    Parts: []content.Part{
        content.Text{Text: "Analyze this image:"},
        content.Image{URL: "data:image/jpeg;base64,..."},
    },
}
```

### Building Conversations
```go
chat := &chat.Chat{}
chat.System("You are a helpful assistant")
chat.User("What's the weather like?")
chat.Assistant("I need to check the weather for you.")

// Add tool call
chat.Add(message.Message{
    Role: role.Assistant,
    Parts: []content.Part{
        content.ToolCall{
            ID: "call_123",
            Name: "get_weather", 
            Args: json.RawMessage(`{"location": "San Francisco"}`),
        },
    },
})

// Add tool result
chat.Tool("call_123", "Sunny, 72°F")
```

### Conversation Management
```go
// Keep only last 10 messages
chat.Compact(10)

// Remove system messages
chat.Filter(func(msg message.Message) bool {
    return msg.Role != role.System
})

// Clone for branching conversations
branch := chat.Clone()
```

## Integration Points

The chats package serves as the data interchange format between:

- **Model Adapters**: Convert to/from provider-specific formats
- **Agent System**: Maintain conversation history and context
- **Tool System**: Embed tool calls/results in content
- **Engine Layer**: Persist and restore conversations

## Key Abstractions

### Content Extensibility
The `Part` interface allows providers and tools to inject custom content types while maintaining type safety and serialization capabilities.

### Metadata Flexibility  
The `map[string]any` metadata field provides escape hatches for provider-specific data (model settings, usage tracking, etc.) without polluting the core model.

### Immutable Messages, Mutable Containers
Messages are value types that can be safely shared, while Chat provides controlled mutation with proper synchronization.

This foundation enables Shelly to work uniformly across different LLM providers while supporting the full spectrum of modern LLM capabilities including multi-modal inputs and tool use.