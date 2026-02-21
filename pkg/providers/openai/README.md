# openai

Package `openai` provides a `modeladapter.Completer` implementation for the
OpenAI Chat Completions API.

## Architecture

`Adapter` embeds `modeladapter.ModelAdapter` and adds OpenAI-specific request and
response mapping. It uses `ModelAdapter.PostJSON` for HTTP calls with the default
`Authorization: Bearer` header scheme.

Key mapping details:

- System prompts are sent as `"system"` role messages in the messages array.
- Assistant tool calls are serialized as `tool_calls` array on the assistant
  message.
- Tool results are sent as individual `"tool"` role messages with `tool_call_id`.

## Usage

```go
adapter := openai.New("https://api.openai.com", apiKey, "gpt-4")
adapter.Tools = tools // optional []toolbox.Tool

msg, err := adapter.Complete(ctx, myChat)
```

Token usage is tracked via `adapter.Usage`.
