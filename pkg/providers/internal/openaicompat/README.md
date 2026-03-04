# openaicompat

Internal package providing shared wire types, conversion logic, and batch infrastructure for providers that use OpenAI-compatible chat completion APIs.

## What's Shared

- **Wire types** (`types.go`): `Request`, `Response`, `Message`, `ToolCall`, `ToolDef`, `Usage`, and batch-specific types
- **Conversion functions** (`convert.go`): `BuildRequest`, `ConvertMessages`, `ConvertTools`, `ParseMessage`, `ParseUsage`, `MarshalToolDef`
- **Batch operations** (`batch.go`): `BatchHelper` with `SubmitBatch`, `PollBatch`, `CancelBatch`, file upload/download, and JSONL parsing

## Usage

Provider adapters compose with these shared functions:

```go
func (a *Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
    req := openaicompat.BuildRequest(a.Config, c, tools)

    var resp openaicompat.Response
    if err := a.client.PostJSON(ctx, openaicompat.CompletionsPath, req, &resp); err != nil {
        return message.Message{}, fmt.Errorf("myprovider: %w", err)
    }

    a.usage.Add(openaicompat.ParseUsage(resp.Usage))
    return openaicompat.ParseMessage(resp.Choices[0].Message), nil
}
```

Batch submitters wrap `BatchHelper`:

```go
type BatchSubmitter struct {
    helper openaicompat.BatchHelper
    config modeladapter.ModelConfig
}

func (b *BatchSubmitter) SubmitBatch(ctx context.Context, reqs []batch.Request) (string, error) {
    return b.helper.SubmitBatch(ctx, b.config, reqs)
}
```

## Consumers

- `providers/openai` — OpenAI Chat Completions API
- `providers/grok` — xAI Grok API (OpenAI-compatible)
