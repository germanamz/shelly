# grok

Package `grok` provides a `modeladapter.Completer` implementation for xAI's
Grok models using the OpenAI-compatible chat completions API.

## Architecture

`GrokAdapter` embeds `modeladapter.ModelAdapter` and adds Grok-specific request
and response mapping. It uses `ModelAdapter.PostJSON` for HTTP calls with Bearer
token authentication.

Key mapping details:

- System prompts stay in the messages array as `"system"` role entries (same as
  OpenAI).
- Tool results are sent as individual `"tool"` role messages with a
  `tool_call_id`.
- Provides `MarshalToolDef` for converting tool definitions into the API format.

## Usage

```go
adapter := grok.New(apiKey, nil) // nil uses http.DefaultClient
adapter.Name = "grok-3"

msg, err := adapter.Complete(ctx, myChat)
```

Token usage is tracked via `adapter.Usage`.
