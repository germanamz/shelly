# anthropic

Package `anthropic` provides a `modeladapter.Completer` implementation for the
Anthropic Messages API (Claude models).

## Purpose

This package translates between Shelly's provider-agnostic chat model
(`pkg/chats`) and the Anthropic Messages API wire format (`/v1/messages`). It
handles authentication, request construction, response parsing, and token usage
tracking.

## Architecture

`Adapter` embeds `modeladapter.ModelAdapter` and adds Anthropic-specific request
and response mapping. It uses `ModelAdapter.PostJSON` for HTTP calls and sets the
`x-api-key` header (not the default Bearer scheme) plus the required
`anthropic-version: 2023-06-01` header.

Key mapping details:

- System prompts are extracted from the chat via `chat.SystemPrompt()` and sent
  as a top-level `system` parameter. System-role messages are excluded from the
  messages array.
- Tool results are always placed in `"user"` role messages, as required by the
  Anthropic API.
- Consecutive content blocks with the same effective role are merged into a
  single message.
- Tool definitions are sent with `input_schema` (not `parameters` like the
  OpenAI format). When a tool has no schema, a default `{"type":"object"}` is
  used.
- Rate limit headers are parsed via `modeladapter.ParseAnthropicRateLimitHeaders`.

## Exported API

### Types

- **`Adapter`** -- Main type. Embeds `modeladapter.ModelAdapter`. Implements
  `modeladapter.Completer`.

### Functions

- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates an `Adapter`
  configured for the Anthropic API. Sets `MaxTokens` to 4096, configures
  `x-api-key` authentication, applies the `anthropic-version` header, and
  registers the Anthropic rate limit header parser.

### Methods

- **`(*Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)`**
  -- Sends a conversation to the Anthropic Messages API and returns the
  assistant's reply. Tools available for this call are passed directly as a
  parameter. Token usage is accumulated in `adapter.Usage`.

## Usage

```go
adapter := anthropic.New("https://api.anthropic.com", apiKey, "claude-sonnet-4-20250514")
adapter.MaxTokens = 8192 // override default if needed

msg, err := adapter.Complete(ctx, myChat, tools)
// Token usage is tracked via adapter.Usage
total := adapter.Usage.Total()
```

## Dependencies

- `pkg/chats/chat` -- Chat type for conversations
- `pkg/chats/content` -- Content part types (Text, ToolCall, ToolResult)
- `pkg/chats/message` -- Message type
- `pkg/chats/role` -- Role constants (System, User, Assistant, Tool)
- `pkg/modeladapter` -- Base `ModelAdapter` struct, `Completer` interface, auth, HTTP helpers
- `pkg/modeladapter/usage` -- Token usage tracking
- `pkg/tools/toolbox` -- Tool definition type
