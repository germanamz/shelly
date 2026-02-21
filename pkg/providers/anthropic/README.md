# anthropic

Package `anthropic` provides a `modeladapter.Completer` implementation for the
Anthropic Messages API (Claude models).

## Architecture

`Adapter` embeds `modeladapter.ModelAdapter` and adds Anthropic-specific request
and response mapping. It uses `ModelAdapter.PostJSON` for HTTP calls and sets the
`x-api-key` header (not Bearer) plus the required `anthropic-version` header.

`Adapter` implements `modeladapter.ToolAware` via `SetTools()`, allowing the
engine to inject tool declarations before agent execution.

Key mapping details:

- System prompts are extracted from the chat and sent as a top-level `system`
  parameter, not in the messages array.
- Tool results are always placed in `"user"` role messages, as required by the
  Anthropic API.
- Consecutive parts with the same effective role are merged into a single
  message.

## Usage

```go
adapter := anthropic.New("https://api.anthropic.com", apiKey, "claude-sonnet-4-20250514")
adapter.SetTools(tools) // or adapter.Tools = tools directly

msg, err := adapter.Complete(ctx, myChat)
```

Token usage is tracked via `adapter.Usage`.
