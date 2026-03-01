# gemini

Package `gemini` provides a `modeladapter.Completer` implementation for the
Google Gemini API.

## Purpose

This package translates between Shelly's provider-agnostic chat model
(`pkg/chats`) and the Google Gemini API wire format
(`/v1beta/models/{model}:generateContent`). It handles authentication, request
construction, response parsing, and token usage tracking.

## Architecture

`Adapter` embeds `modeladapter.ModelAdapter` and adds Gemini-specific request and
response mapping. It uses `ModelAdapter.PostJSON` for HTTP calls with the
`x-goog-api-key` header for authentication (no scheme prefix).

Key mapping details:

- System prompts are sent in the top-level `systemInstruction` field, not in the
  `contents` array.
- User messages are sent as `"user"` role content entries.
- Assistant messages are sent as `"model"` role content entries.
- Tool calls from the assistant are represented as `functionCall` parts within
  `"model"` content.
- Tool results are represented as `functionResponse` parts within `"user"`
  content. The function name is resolved from a scan of prior `ToolCall` parts.
- Tool definitions use the `tools[].functionDeclarations[]` format with `name`,
  `description`, and `parameters` for the JSON schema.
- An empty `candidates` array in the response is treated as an error.
- Gemini does not return tool call IDs; synthetic IDs are generated using an
  atomic counter (`call_{name}_{seq}`).
- Consecutive same-role content entries are merged to satisfy Gemini's strict
  user/model alternation requirement.
- The model name is part of the URL path, not the request body.
- HTTP 429 responses are returned as `*modeladapter.RateLimitError`.

## Limitations

- **No adaptive throttling.** The Gemini API does not return rate limit
  headers (e.g., `x-ratelimit-remaining-*`). The `HeaderParser` field is not
  set, so `LastRateLimitInfo()` always returns nil and adaptive throttling
  via `adaptFromServerInfo()` is inactive. The `RateLimitedCompleter` still
  provides proactive throttling (TPM/RPM sliding window) and reactive retry
  (429 + exponential backoff).

## Exported API

### Types

- **`Adapter`** -- Main type. Embeds `modeladapter.ModelAdapter`. Implements
  `modeladapter.Completer`.

### Functions

- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates an `Adapter`
  configured for the Gemini API. Sets `MaxTokens` to 8192. Uses the
  `x-goog-api-key` header for authentication.

### Methods

- **`(*Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)`**
  -- Sends a conversation to the Gemini API and returns the assistant's reply.
  Tools available for this call are passed directly as a parameter. Token usage
  is accumulated in `adapter.Usage`.

## Usage

```go
adapter := gemini.New("https://generativelanguage.googleapis.com", apiKey, "gemini-3.1-pro-preview")
adapter.MaxTokens = 16384 // override default if needed

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
