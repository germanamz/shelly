# grok

Package `grok` provides a `modeladapter.Completer` implementation for xAI's
Grok models using the OpenAI-compatible chat completions API.

## Purpose

This package translates between Shelly's provider-agnostic chat model
(`pkg/chats`) and the xAI Grok chat completions API wire format
(`/v1/chat/completions`). It handles authentication, request construction,
response parsing, and token usage tracking.

## Architecture

`GrokAdapter` embeds `modeladapter.ModelAdapter` and adds Grok-specific request
and response mapping. It uses `ModelAdapter.PostJSON` for HTTP calls with Bearer
token authentication.

Unlike the `anthropic` and `openai` providers, `GrokAdapter` uses the
`modeladapter.New()` constructor to initialize the embedded `ModelAdapter` and
accepts an `*http.Client` parameter (nil falls back to `http.DefaultClient`).
The model name is not set by the constructor -- callers set `adapter.Name`
after creation.

Key mapping details:

- System prompts stay in the messages array as `"system"` role entries, matching
  the OpenAI convention.
- User messages are sent as `"user"` role messages.
- Assistant messages include text content and a `tool_calls` array when tool
  calls are present.
- Tool results are sent as individual `"tool"` role messages with a
  `tool_call_id`. Multiple tool results in a single message are expanded into
  separate API messages.
- Tool definitions use the OpenAI-compatible
  `{"type":"function","function":{...}}` wrapper format. When a tool has no
  schema, a default `{"type":"object"}` is used.
- An empty `choices` array in the response is treated as an error.
- Rate limit headers are parsed via `modeladapter.ParseOpenAIRateLimitHeaders`.
- Messages are converted using `chat.Each()` for iteration rather than
  `chat.Messages()`.

## Exported API

### Constants

- **`DefaultBaseURL`** (`"https://api.x.ai"`) -- The default base URL for the
  xAI API.

### Types

- **`GrokAdapter`** -- Main type. Embeds `modeladapter.ModelAdapter`. Implements
  `modeladapter.Completer`.

### Functions

- **`New(apiKey string, client *http.Client) *GrokAdapter`** -- Creates a
  `GrokAdapter` configured with the default xAI base URL and Bearer
  authentication. A nil client falls back to `http.DefaultClient`. Callers
  must set `adapter.Name` to the desired model (e.g. `"grok-3"`) after
  creation. Does not set `MaxTokens` by default.

- **`MarshalToolDef(name, description string, schema json.RawMessage) apiTool`**
  -- Converts a tool name, description, and JSON schema into the API tool
  format used in the chat request. This is a convenience for callers that need
  to construct tool definitions outside the `Complete` flow. Note: the returned
  `apiTool` type is unexported, but the function itself is exported for use
  within the package's API boundary.

### Methods

- **`(*GrokAdapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)`**
  -- Sends a conversation to the Grok chat completions endpoint and returns the
  assistant's reply. Tools available for this call are passed directly as a
  parameter. Token usage is accumulated in `adapter.Usage`.

## Usage

```go
adapter := grok.New(apiKey, nil) // nil uses http.DefaultClient
adapter.Name = "grok-3"
adapter.MaxTokens = 4096
adapter.Temperature = 0.7

msg, err := adapter.Complete(ctx, myChat, tools)
// Token usage is tracked via adapter.Usage
total := adapter.Usage.Total()
```

## Dependencies

- `pkg/chats/chat` -- Chat type for conversations
- `pkg/chats/content` -- Content part types (Text, ToolCall, ToolResult)
- `pkg/chats/message` -- Message type
- `pkg/chats/role` -- Role constants (System, User, Assistant, Tool)
- `pkg/modeladapter` -- Base `ModelAdapter` struct, `Completer` interface, `New()` constructor, auth, HTTP helpers
- `pkg/modeladapter/usage` -- Token usage tracking
- `pkg/tools/toolbox` -- Tool definition type
- `net/http` -- For the optional `*http.Client` parameter
