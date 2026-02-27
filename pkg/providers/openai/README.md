# openai

Package `openai` provides a `modeladapter.Completer` implementation for the
OpenAI Chat Completions API.

## Purpose

This package translates between Shelly's provider-agnostic chat model
(`pkg/chats`) and the OpenAI Chat Completions API wire format
(`/v1/chat/completions`). It handles authentication, request construction,
response parsing, and token usage tracking.

## Architecture

`Adapter` embeds `modeladapter.ModelAdapter` and adds OpenAI-specific request and
response mapping. It uses `ModelAdapter.PostJSON` for HTTP calls with the default
`Authorization: Bearer` header scheme.

Key mapping details:

- System prompts are sent as `"system"` role messages in the messages array.
- User messages are sent as `"user"` role messages.
- Assistant messages include both text content and a `tool_calls` array when
  tool calls are present. Text parts are concatenated into a single content
  string.
- Tool results are sent as individual `"tool"` role messages, each with a
  `tool_call_id` field.
- Tool definitions use the `{"type":"function","function":{...}}` wrapper format
  with `parameters` for the JSON schema. When a tool has no schema, a default
  `{"type":"object"}` is used.
- An empty `choices` array in the response is treated as an error.
- Rate limit headers are parsed via `modeladapter.ParseOpenAIRateLimitHeaders`.
- HTTP 429 responses are returned as `*modeladapter.RateLimitError`.

## Exported API

### Types

- **`Adapter`** -- Main type. Embeds `modeladapter.ModelAdapter`. Implements
  `modeladapter.Completer`.

### Functions

- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates an `Adapter`
  configured for the OpenAI API. Sets `MaxTokens` to 4096 and registers the
  OpenAI rate limit header parser. Uses the default `Authorization: Bearer`
  authentication scheme.

### Methods

- **`(*Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)`**
  -- Sends a conversation to the OpenAI Chat Completions API and returns the
  assistant's reply. Tools available for this call are passed directly as a
  parameter. Token usage is accumulated in `adapter.Usage`.

## Usage

```go
adapter := openai.New("https://api.openai.com", apiKey, "gpt-4")
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
- `pkg/modeladapter` -- Base `ModelAdapter` struct, `Completer` interface, `RateLimitError`, auth, HTTP helpers
- `pkg/modeladapter/usage` -- Token usage tracking
- `pkg/tools/toolbox` -- Tool definition type
