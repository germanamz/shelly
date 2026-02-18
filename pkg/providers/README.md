# providers

An abstraction layer for LLM completion providers. The providers package defines the shared configuration and interface that concrete provider adapters (OpenAI, Anthropic, local models, etc.) must implement. It depends on the `chatty` package for its chat, message, and content types.

## Architecture

```
providers/
├── model/      Provider-agnostic model configuration (name, temperature, max tokens)
└── provider/   Interface that concrete provider adapters must satisfy
```

### `model` — Model Configuration

`Model` holds provider-agnostic LLM settings shared by all adapters:

| Field         | Type      | Description                       |
|---------------|-----------|-----------------------------------|
| `Name`        | `string`  | Model identifier (e.g. `"gpt-4"`) |
| `Temperature` | `float64` | Sampling temperature               |
| `MaxTokens`   | `int`     | Maximum tokens in the response     |

The zero value is valid — zero fields mean "use provider defaults". `Model` is designed to be **embedded** in provider-specific config structs so that shared settings are always available without duplication.

### `provider` — Provider Interface

Defines the single interface that all provider adapters must satisfy:

```go
type Provider interface {
    Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}
```

`Complete` sends a conversation to an LLM and returns the assistant's reply as a `message.Message`. It accepts the full `chat.Chat` so the provider has access to the entire conversation history, system prompt, and any tool-call context.

## Examples

### Basic Provider Usage

```go
var p provider.Provider = newMyProvider()

c := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("user", role.User, "Explain goroutines."),
)

reply, err := p.Complete(ctx, c)
if err != nil {
    log.Fatal(err)
}
c.Append(reply)
fmt.Println(reply.TextContent())
```

### Implementing a Provider Adapter

Embed `model.Model` for shared configuration and implement the `Provider` interface:

```go
type openAIAdapter struct {
    model.Model // Name, Temperature, MaxTokens
    apiKey string
}

func (a *openAIAdapter) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
    // 1. Convert c to the provider's wire format
    // 2. Call the API with a.Name, a.Temperature, a.MaxTokens, a.apiKey
    // 3. Return the result as a message.Message
}
```

### Model Configuration with Zero-Value Defaults

```go
// All zeros — the adapter decides what defaults to use
m := model.Model{}

// Override only what you need
m = model.Model{
    Name:        "claude-sonnet-4-5-20250929",
    Temperature: 0.7,
}

// Embed in a provider-specific struct
type anthropicAdapter struct {
    model.Model
    apiKey  string
    version string
}

adapter := &anthropicAdapter{
    Model:   m,
    apiKey:  os.Getenv("ANTHROPIC_API_KEY"),
    version: "2024-01-01",
}
```

### Tool-Use Flow Through a Provider

```go
c := chat.New(
    message.NewText("", role.System, "You can use the 'search' tool."),
    message.NewText("user", role.User, "Find information about Go generics."),
)

// First completion — the provider may return a tool call
reply, _ := p.Complete(ctx, c)
c.Append(reply)

// Check for tool calls and execute them
for _, tc := range reply.ToolCalls() {
    result := executeTool(tc.Name, tc.Arguments)
    c.Append(message.New("", role.Tool,
        content.ToolResult{ToolCallID: tc.ID, Content: result},
    ))
}

// Second completion — the provider uses the tool result to answer
final, _ := p.Complete(ctx, c)
c.Append(final)
fmt.Println(final.TextContent())
```
