# LLM Providers Layer

> `pkg/providers/` — Concrete LLM adapters that implement `modeladapter.Completer`, `UsageReporter`, and `RateLimitInfoReporter`.

## Architecture

```
pkg/providers/
├── anthropic/              Anthropic Messages API (custom wire format)
├── openai/                 OpenAI Chat Completions API (delegates to openaicompat)
├── grok/                   xAI Grok API (delegates to openaicompat)
├── gemini/                 Google Gemini API (custom wire format, Vertex AI support)
└── internal/openaicompat/  Shared types, conversion, and batch logic for OpenAI-compatible APIs
```

**Namespace package** — `pkg/providers/` has no Go source files of its own; it groups sub-packages.

## Interfaces Implemented

Every provider adapter satisfies three interfaces from `modeladapter`:

| Interface | Method | Purpose |
|-----------|--------|---------|
| `Completer` | `Complete(ctx, *chat.Chat, []toolbox.Tool) (message.Message, error)` | Send conversation → get assistant reply |
| `UsageReporter` | `UsageTracker() *usage.Tracker` / `ModelMaxTokens() int` | Token usage tracking |
| `RateLimitInfoReporter` | `LastRateLimitInfo() *RateLimitInfo` | Rate limit info from response headers |

All adapters compose a `modeladapter.Client` (HTTP/WebSocket transport with auth, custom headers, rate limit header parsing). The adapter does **not** inherit `Complete` from `Client` — it implements `Complete` itself using the client for HTTP calls.

## Shared Internal Package: `internal/openaicompat`

Providers with OpenAI-compatible APIs (OpenAI, Grok) share conversion and batch logic.

### Files

| File | Contents |
|------|----------|
| `types.go` | Wire types: `Request`, `Response`, `Message`, `ToolCall`, `ToolDef`, `Usage`, `ContentPart`, batch types (`BatchFileRequest`, `BatchFileResponse`, `BatchStatus`, `FileObject`) |
| `convert.go` | `BuildRequest()` — builds an OpenAI `Request` from `chat.Chat` + tools + config. `ConvertMessages()` — translates `[]message.Message` → OpenAI `[]Message`. `ConvertTools()` — translates `[]toolbox.Tool` → `[]ToolDef`. `ToMessage()` — converts OpenAI `Response` → `message.Message` + `usage.TokenCount` |
| `batch.go` | `BatchHelper` struct with `SubmitBatch`, `PollBatch`, `CancelBatch` — shared batch API implementation using JSONL file upload + batch creation + polling |

### Key Constants

- `CompletionsPath = "/v1/chat/completions"`
- `FilesPath = "/v1/files"`, `BatchesPath = "/v1/batches"`

### Conversion Details

- **System messages** → extracted as the first user-role system block (OpenAI format with `role: "system"`)
- **Tool calls** → mapped to `tool_calls` array with `type: "function"`, JSON-stringified arguments
- **Tool results** → `role: "tool"` with `tool_call_id`
- **Image content** → `image_url` content parts with base64 data URIs
- **Thinking blocks** → content parts with `type: "thinking"` (provider-specific)
- **Token usage** → parsed from `response.Usage` into `usage.TokenCount{Input, Output, CacheRead, CacheCreation}`

## Provider Details

### Anthropic (`providers/anthropic`)

**Native implementation** — does NOT use `openaicompat` (Anthropic has its own wire format).

**Adapter struct:**
```go
type Adapter struct {
    Config modeladapter.ModelConfig
    client *modeladapter.Client  // base URL: https://api.anthropic.com
    usage  usage.Tracker
}
```

**Constructor:** `New(apiKey string, config modeladapter.ModelConfig, opts ...modeladapter.ClientOption) *Adapter`
- Default headers: `anthropic-version: 2023-06-01`, `anthropic-beta: interleaved-thinking-2025-05-14`
- Uses `modeladapter.ParseAnthropicRateLimitHeaders` for rate limit parsing
- Auth type: `XAPIKey` (sends `x-api-key` header)

**Complete flow:**
1. Builds native Anthropic request with `model`, `max_tokens`, `temperature`, `system` (content blocks), `messages`, `tools`
2. System prompt → array of `{"type":"text","text":...}` blocks with optional cache control
3. Messages → translates roles, content blocks (text, images, tool_use, tool_result, thinking, redacted_thinking)
4. Images → base64 `source` blocks with `media_type`
5. Thinking content → `{"type":"thinking","thinking":...}` with budget_tokens (`max(1024, maxTokens-1)`)
6. Tool definitions → `{"name","description","input_schema"}` with optional cache control
7. POSTs to `/v1/messages`
8. Parses response content blocks back to `message.Message`

**Batch:** `BatchSubmitter` — native Anthropic batches API (`/v1/messages/batches`).
- `SubmitBatch` → POST JSONL body with array of `{custom_id, params}` requests
- `PollBatch` → GET `batchesPath/<id>`, checks for `ended` status, fetches results from `results_url` streaming JSONL
- `CancelBatch` → POST `batchesPath/<id>/cancel`
- Response parsing handles `succeeded`/`errored`/`expired`/`canceled` per-item results

### OpenAI (`providers/openai`)

**Thin wrapper** around `openaicompat`.

**Adapter struct:**
```go
type Adapter struct {
    Config modeladapter.ModelConfig
    client *modeladapter.Client  // base URL: https://api.openai.com
    usage  usage.Tracker
}
```

**Constructor:** `New(apiKey string, config modeladapter.ModelConfig, opts ...modeladapter.ClientOption) *Adapter`
- Auth type: `BearerToken`
- Uses `modeladapter.ParseOpenAIRateLimitHeaders`

**Complete flow:**
1. Calls `openaicompat.BuildRequest(chat, tools, adapter.Config)` to build the request
2. POSTs to `openaicompat.CompletionsPath` via `client.PostJSON`
3. Calls `openaicompat.ToMessage(response)` to convert back

**Batch:** `BatchSubmitter` — delegates entirely to `openaicompat.BatchHelper`.

### Grok (`providers/grok`)

**Thin wrapper** around `openaicompat` — nearly identical to OpenAI.

**Adapter struct:**
```go
type Adapter struct {
    Config modeladapter.ModelConfig
    client *modeladapter.Client  // base URL: https://api.x.ai
    usage  usage.Tracker
}
```

**Constructor:** `New(apiKey string, config modeladapter.ModelConfig, opts ...modeladapter.ClientOption) *Adapter`
- Auth type: `BearerToken`
- Uses `modeladapter.ParseOpenAIRateLimitHeaders`

**Complete flow:** Same as OpenAI — `BuildRequest` → POST → `ToMessage`.

**Batch:** `BatchSubmitter` — delegates entirely to `openaicompat.BatchHelper`.

### Gemini (`providers/gemini`)

**Native implementation** — Gemini has its own wire format (`generateContent` API).

**Adapter struct:**
```go
type Adapter struct {
    Config    modeladapter.ModelConfig
    ProjectID string  // Vertex AI project
    Location  string  // Vertex AI region
    client    *modeladapter.Client  // base URL varies (googleapis.com)
    usage     usage.Tracker
}
```

**Constructor:** `New(apiKey string, config modeladapter.ModelConfig, opts ...modeladapter.ClientOption) *Adapter`
- Supports both **API key** auth (`key=` query param) and **Vertex AI** (`NewVertexAI` constructor with `ProjectID`, `Location`, bearer token)
- Default base URL: `https://generativelanguage.googleapis.com`
- Vertex AI URL: `https://{location}-aiplatform.googleapis.com`

**Complete flow:**
1. Builds Gemini-native request: `contents` (conversation), `tools`, `systemInstruction`, `generationConfig`
2. System prompt → `systemInstruction` with `parts` array
3. Messages → role mapping: `user`→`user`, `assistant`→`model`; content blocks include text, function calls, function responses, inline images
4. Tool calls → `functionCall` parts with parsed JSON `args`
5. Tool results → `functionResponse` parts with `response.result` wrapping
6. Thinking → `thought: true` field on text parts; thinking budget via `thinkingConfig.thinkingBudget`
7. POSTs to `/v1beta/models/{model}:generateContent` (or Vertex AI path)
8. Parses `candidates[0].content.parts` back to message

**Batch:** `BatchSubmitter` — Gemini's `batchGenerateContent` is **synchronous** (inline, not async polling).
- `SubmitBatch` → sends all requests at once, returns synthetic batch ID, stores results in memory
- `PollBatch` → immediately returns stored results (always done)
- `CancelBatch` → no-op (already complete)
- Uses `sync.Map` for result storage, `atomic.Int64` for ID generation

## Common Patterns

1. **All adapters** store `Config modeladapter.ModelConfig` and `usage usage.Tracker` as fields
2. **All adapters** have `UsageTracker() *usage.Tracker` and `ModelMaxTokens() int` methods
3. **All adapters** get `LastRateLimitInfo()` via the embedded `*modeladapter.Client`
4. **Constructor pattern:** `New(apiKey, config, ...ClientOption)` — client options allow custom HTTP clients, base URLs, headers
5. **Error handling:** Providers check `response.StatusCode` and return descriptive errors with response body
6. **Cache control:** Anthropic supports `ephemeral` cache control on system prompt and last tool definition to enable prompt caching
7. **Thinking/reasoning:** Anthropic, Gemini, and OpenAI-compat all handle extended thinking content blocks

## Batch API Summary

| Provider | Approach | Submit | Poll | Cancel |
|----------|----------|--------|------|--------|
| Anthropic | Async (native JSONL) | POST requests array | GET status + stream results | POST cancel |
| OpenAI | Async (JSONL file upload) | Upload file → create batch | GET batch → download output file | POST cancel |
| Grok | Async (JSONL file upload) | Same as OpenAI (via `openaicompat`) | Same as OpenAI | Same as OpenAI |
| Gemini | **Synchronous** (inline) | Send all at once, store results in-memory | Return stored results immediately | No-op |

## Dependencies

- `pkg/chats/{chat,content,message,role}` — data model
- `pkg/modeladapter` — `Client`, `ModelConfig`, `Completer`, rate limit parsing
- `pkg/modeladapter/usage` — `Tracker`, `TokenCount`
- `pkg/modeladapter/batch` — `Submitter`, `Request`, `Result` interfaces
- `pkg/tools/toolbox` — `Tool` type for tool definitions
