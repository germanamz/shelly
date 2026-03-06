# Providers Layer

> `pkg/providers/` — Namespace package grouping LLM provider adapters.  
> No Go source files of its own. Each sub-package implements `modeladapter.Completer`.

## Architecture Overview

```
pkg/providers/
├── README.md                   # Detailed provider docs
├── anthropic/                  # Claude (native API)
│   ├── anthropic.go            # Adapter, Complete(), wire types, conversion
│   └── batch.go                # BatchSubmitter (native Anthropic batches)
├── openai/                     # OpenAI GPT models
│   ├── openai.go               # Thin wrapper around openaicompat
│   └── batch.go                # BatchSubmitter via openaicompat.BatchHelper
├── grok/                       # xAI Grok (OpenAI-compatible API)
│   ├── grok.go                 # Thin wrapper + custom HTTP transport
│   └── batch.go                # BatchSubmitter via openaicompat.BatchHelper
├── gemini/                     # Google Gemini (native API)
│   ├── gemini.go               # Adapter, Complete(), wire types, conversion
│   └── batch.go                # BatchSubmitter (synchronous inline batch)
└── internal/
    └── openaicompat/           # Shared code for OpenAI-compatible providers
        ├── types.go            # Wire types (Request, Response, Message, ToolCall, etc.)
        ├── convert.go          # BuildRequest, ConvertMessages, ParseMessage, ParseUsage
        └── batch.go            # BatchHelper (file upload, JSONL, polling)
```

## The Completer Contract

Every provider implements this interface from `pkg/modeladapter`:

```go
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)
    ModelMaxTokens() int
}
```

Additionally, all providers implement `UsageReporter`:

```go
type UsageReporter interface {
    UsageTracker() *usage.Tracker
}
```

Each adapter embeds a `modeladapter.Client` for HTTP transport (with auth, rate-limit handling, 429 retry with backoff) and a `modeladapter.ModelConfig` for model name, temperature, and max tokens.

## Factory Pattern — `New()` Constructors

Each provider exports a `New()` function returning the concrete `*Adapter` (which satisfies `Completer`):

| Provider | Constructor | Base URL |
|----------|------------|----------|
| **Anthropic** | `New(baseURL, apiKey, model string) *Adapter` | `https://api.anthropic.com` |
| **OpenAI** | `New(baseURL, apiKey, model string) *Adapter` | `https://api.openai.com` |
| **Grok** | `New(baseURL, apiKey, model string) *Adapter` | `https://api.x.ai` |
| **Gemini** | `New(baseURL, apiKey, model string) *Adapter` | `https://generativelanguage.googleapis.com` |

The engine (`pkg/engine`) wires providers by matching model names from `config.yaml` to the appropriate constructor.

## Two Provider Families

### 1. OpenAI-Compatible (OpenAI, Grok)

These are **thin wrappers** around `internal/openaicompat`. Their `Complete()` method is essentially:

```go
func (a *Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
    req := openaicompat.BuildRequest(a.config, c, tools)
    var resp openaicompat.Response
    err := a.client.PostJSON(ctx, openaicompat.CompletionsPath, req, &resp)
    // ... error handling, usage tracking ...
    return openaicompat.ParseMessage(resp.Choices[0].Message), nil
}
```

**Grok specifics:** Uses a custom `http.Transport` for request/response body capture (logging/debugging). Otherwise identical in wire format to OpenAI.

### 2. Native API (Anthropic, Gemini)

These define their own wire types and conversion logic inline, as their APIs differ substantially from OpenAI's format.

## Message & Content Translation

### OpenAI-Compatible (via `openaicompat`)

| `chats` type | OpenAI wire format |
|---|---|
| `role.System` / `role.User` | `{"role": "system"/"user", "content": "..."}` |
| `role.Assistant` | `{"role": "assistant", "content": "...", "tool_calls": [...]}` |
| `role.Tool` | `{"role": "tool", "content": "...", "tool_call_id": "..."}` — one message per `ToolResult` |
| `content.Image` | Multi-modal `content` array with `image_url` part (data URI) |
| `content.Document` | Multi-modal `content` array with `file` part (data URI) |
| `content.ToolCall` | `tool_calls[].function.{name, arguments}` |
| `content.ToolResult` | Separate `role: "tool"` message with `tool_call_id` |

Key functions: `ConvertMessages()`, `ParseMessage()`, `ConvertTools()`, `MarshalToolDef()`.

### Anthropic

| `chats` type | Anthropic wire format |
|---|---|
| `role.System` | Extracted to top-level `system` field (not in messages array) |
| `role.User` | `{"role": "user", "content": [{"type": "text", ...}]}` |
| `role.Assistant` | `{"role": "assistant", "content": [{"type": "text"}, {"type": "tool_use"}]}` |
| `role.Tool` | Mapped to `role: "user"` with `{"type": "tool_result", "tool_use_id": "..."}` |
| `content.Image` | `{"type": "image", "source": {"type": "base64", ...}}` |
| `content.Document` | `{"type": "document", "source": {"type": "base64", ...}}` |
| `content.ToolCall` | `{"type": "tool_use", "id": "...", "name": "...", "input": {...}}` |
| `content.ToolResult` | `{"type": "tool_result", "tool_use_id": "...", "content": "..."}` |

Notable: Anthropic requires `tool_result` blocks within `user` role messages (not a separate `tool` role). The adapter merges adjacent same-role messages when needed.

### Gemini

| `chats` type | Gemini wire format |
|---|---|
| `role.System` | Extracted to top-level `systemInstruction` field |
| `role.User` / `role.Tool` | `{"role": "user", "parts": [...]}` |
| `role.Assistant` | `{"role": "model", "parts": [...]}` |
| `content.Image` / `content.Document` | `{"inlineData": {"mimeType": "...", "data": "..."}}` |
| `content.ToolCall` | `{"functionCall": {"name": "...", "args": {...}}}` |
| `content.ToolResult` | `{"functionResponse": {"name": "...", "response": {...}}}` |

**Gemini-specific challenges:**
- **Role alternation required:** Gemini requires strict user/model alternation. The adapter merges consecutive same-role messages.
- **No tool call IDs:** Gemini doesn't return call IDs, so the adapter synthesizes them via `generateCallID()` using random bytes.
- **ToolResult → functionResponse mapping:** `ToolResult` only carries `ToolCallID`, but Gemini needs the function name. The adapter builds a `callNameMap` by scanning history for `ToolCall` parts.
- **Schema sanitization:** `sanitizeSchema()` recursively strips `$schema` and `additionalProperties` keys that Gemini rejects.
- **thoughtSignature:** Preserved in `ToolCall.Metadata` for Gemini's thinking feature.

## Tool Call Mapping

All providers translate `toolbox.Tool` definitions to their native format:

```go
// toolbox.Tool fields used:
type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage  // JSON Schema for parameters
}
```

- **OpenAI/Grok:** `{"type": "function", "function": {"name": "...", "description": "...", "parameters": <schema>}}`
- **Anthropic:** `{"name": "...", "description": "...", "input_schema": <schema>}`
- **Gemini:** `{"functionDeclarations": [{"name": "...", "description": "...", "parameters": <sanitized-schema>}]}`

## Usage & Token Tracking

Each adapter embeds a `usage.Tracker` (thread-safe atomic counter) and calls `tracker.Add()` after each successful completion:

| Provider | Input tokens field | Output tokens field | Cache tokens field |
|---|---|---|---|
| **Anthropic** | `usage.input_tokens` | `usage.output_tokens` | `usage.cache_read_input_tokens` |
| **OpenAI/Grok** | `usage.prompt_tokens` | `usage.completion_tokens` | `usage.prompt_tokens_details.cached_tokens` |
| **Gemini** | `usageMetadata.promptTokenCount` | `usageMetadata.candidatesTokenCount` | `usageMetadata.cachedContentTokenCount` |

All map to the unified `usage.TokenCount{InputTokens, OutputTokens, CacheReadInputTokens}`.

## Batch Processing

All four providers implement `batch.Submitter` for asynchronous batch completions:

```go
type Submitter interface {
    SubmitBatch(ctx context.Context, reqs []Request) (batchID string, err error)
    PollBatch(ctx context.Context, batchID string) (results map[string]Result, done bool, err error)
    CancelBatch(ctx context.Context, batchID string) error
}
```

| Provider | Batch strategy | Implementation |
|---|---|---|
| **OpenAI** | Async JSONL file upload → poll | `openaicompat.BatchHelper` (shared) |
| **Grok** | Async JSONL file upload → poll | `openaicompat.BatchHelper` (shared) |
| **Anthropic** | Native `/v1/messages/batches` API | Custom SSE streaming for results |
| **Gemini** | Synchronous inline `batchGenerateContent` | Processes all requests in one call, returns immediately |

The `openaicompat.BatchHelper` handles: JSONL encoding, multipart file upload, batch creation, status polling, result download and parsing — shared between OpenAI and Grok.

## Streaming

Anthropic uses SSE (Server-Sent Events) for batch result streaming — the `batch.go` reads `event: result` lines from an SSE stream. Standard `Complete()` calls across all providers use synchronous request/response (no streaming for regular completions at this layer).

## Provider-Specific Auth

| Provider | Auth header | Auth scheme |
|---|---|---|
| **Anthropic** | `x-api-key` | No scheme (raw key) |
| **OpenAI** | `Authorization` | `Bearer <key>` |
| **Grok** | `Authorization` | `Bearer <key>` |
| **Gemini** | `x-goog-api-key` | No scheme (raw key) |

Anthropic also sends additional headers: `anthropic-version: 2023-06-01` and optional `anthropic-beta` for features like prompt caching and extended output.

## The `internal/openaicompat` Package

This internal package eliminates duplication between OpenAI and Grok (and any future OpenAI-compatible provider). It provides:

- **Wire types** (`types.go`): Full request/response structs with custom JSON marshaling for the polymorphic `content` field (string or array).
- **Conversion** (`convert.go`): `BuildRequest()`, `ConvertMessages()`, `ConvertTools()`, `ParseMessage()`, `ParseUsage()`.
- **Batch infra** (`batch.go`): `BatchHelper` with `SubmitBatch()`, `PollBatch()`, `CancelBatch()`, file upload/download, JSONL parsing.

## Adding a New Provider

1. Create `pkg/providers/<name>/` with `<name>.go` and `batch.go`
2. Define `Adapter` struct embedding `*modeladapter.Client`, `modeladapter.ModelConfig`, `usage.Tracker`
3. Implement `Complete()`, `ModelMaxTokens()`, `UsageTracker()`
4. Export `New(baseURL, apiKey, model string) *Adapter`
5. If OpenAI-compatible, delegate to `internal/openaicompat`; otherwise define custom wire types
6. Add `README.md` per project conventions
7. Register in `pkg/engine/` config wiring

## Key Patterns

- **Interface compliance guards:** All adapters use `var _ modeladapter.Completer = (*Adapter)(nil)` to catch breakage at compile time.
- **Error wrapping:** All errors are prefixed with the provider name (e.g., `fmt.Errorf("anthropic: %w", err)`).
- **Nil schema fallback:** All providers default to `{"type":"object"}` when a tool has no input schema.
- **Graceful degradation:** Anthropic's `parseResponse` and Gemini's `appendContent` skip unrecognized content types rather than failing.
