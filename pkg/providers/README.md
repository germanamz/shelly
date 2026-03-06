# providers

Package `providers` contains LLM provider implementations that satisfy the
`modeladapter.Completer` interface. Each sub-package translates between Shelly's
provider-agnostic chat model (`pkg/chats`) and a specific LLM API wire format.

## Purpose

This is a namespace package with no Go source files of its own. It groups the
concrete provider adapters that the engine (`pkg/engine`) wires into agents
based on YAML configuration. Each sub-package is self-contained and depends only
on the shared `pkg/chats`, `pkg/modeladapter`, and `pkg/tools/toolbox`
packages.

## Sub-packages

| Package       | Provider       | API Endpoint                                  | Auth Scheme            |
|---------------|----------------|-----------------------------------------------|------------------------|
| `anthropic`   | Anthropic      | `/v1/messages`                                | `x-api-key` header     |
| `openai`      | OpenAI         | `/v1/chat/completions`                        | `Authorization: Bearer` |
| `grok`        | xAI Grok       | `/v1/chat/completions`                        | `Authorization: Bearer` |
| `gemini`      | Google Gemini  | `/v1beta/models/{model}:generateContent`      | `x-goog-api-key` header |

Additionally, `internal/openaicompat/` provides shared wire types, message
conversion, and batch infrastructure used by the `openai` and `grok`
sub-packages (see below).

## Architecture

All providers follow the same composition pattern:

1. **Compose `modeladapter.Client` + `modeladapter.ModelConfig` +
   `usage.Tracker`** -- `Client` provides HTTP transport (`PostJSON`,
   `NewRequest`, `Do`), authentication, custom headers, and rate limit info.
   `ModelConfig` holds the model name, temperature, and max tokens.
   `usage.Tracker` accumulates token counts.
2. **Implement `modeladapter.Completer`** -- The single-method interface
   `Complete(ctx, chat, tools) (message, error)` that the agent's ReAct loop
   calls on each iteration.
3. **Convert messages** -- Transform `pkg/chats` types (Text, ToolCall,
   ToolResult, roles) into the provider's API format and parse responses back
   into `message.Message` with `content.Part` slices.
4. **Track usage** -- Accumulate input/output token counts (and cache metrics
   where available) via `usage.Tracker.Add()` after each successful API call.
5. **Optionally implement `modeladapter.UsageReporter`** -- Exposes
   `UsageTracker()` and `ModelMaxTokens()` for the engine and effects.
6. **Optionally implement `modeladapter.RateLimitInfoReporter`** -- Delegates
   to `Client.LastRateLimitInfo()` for proactive rate limit throttling (all
   providers except Gemini).

### Shared OpenAI-compatible base (`internal/openaicompat`)

The `openai` and `grok` providers share an OpenAI-compatible wire format. Rather
than duplicating the code, shared types and logic live in
`providers/internal/openaicompat/`:

- **`types.go`** -- Wire types: `Request`, `Message`, `ToolCall`, `ToolDef`,
  `Response`, `Choice`, `Usage`, etc. Also exports `CompletionsPath`.
- **`convert.go`** -- `BuildRequest`, `ConvertMessages`, `ConvertTools`,
  `MarshalToolDef`, `ParseMessage`, `ParseUsage`.
- **`batch.go`** -- `BatchHelper` struct with `SubmitBatch`, `PollBatch`,
  `CancelBatch` and supporting helpers (`UploadFile`, `DownloadResults`,
  `ParseResultsJSONL`, `ConvertResult`).

Both `openai.Adapter` and `grok.Adapter` delegate their `Complete` and batch
implementations to these shared functions, keeping only provider-specific config.

### Provider Differences

- **Anthropic** sends the system prompt as a top-level `system` field and places
  tool results in `"user"` role messages. Tool schemas use `input_schema`.
  Default max tokens: 4096. Uses `ParseAnthropicRateLimitHeaders` for rate
  limit handling. Sends a custom `anthropic-version: 2023-06-01` header.
  **Prompt caching**: The adapter attaches `cache_control: {type: "ephemeral"}`
  to the system message block and the last tool definition. The API caches the
  entire prefix up to the last block marked with `cache_control`. Cache metrics
  (`cache_creation_input_tokens`, `cache_read_input_tokens`) are tracked in the
  usage system.
- **OpenAI** sends the system prompt as a `"system"` role message in the messages
  array. Tool definitions use the `{"type":"function","function":{...}}` wrapper
  with `parameters`. Default max tokens: 4096. Uses
  `ParseOpenAIRateLimitHeaders` for rate limit handling.
  **Prompt caching**: OpenAI auto-caches prompts >= 1024 tokens. The adapter
  captures `prompt_tokens_details.cached_tokens` from responses and maps it to
  `CacheReadInputTokens` in the usage tracker.
- **Grok** follows the OpenAI-compatible format (same message structure and tool
  definitions) via the shared `openaicompat` package. Accepts an optional
  `*http.Client` in its constructor (falls back to `http.DefaultClient`).
  Default base URL: `https://api.x.ai`. Uses `ParseOpenAIRateLimitHeaders`.
- **Gemini** sends the system prompt as a top-level `systemInstruction` field
  and uses `contents` (not `messages`) with role alternation between `"user"`
  and `"model"`. Tool definitions use `functionDeclarations` grouped in a tool
  set. Tool results are sent as `functionResponse` parts (requiring the
  function name, resolved via a call-ID-to-name lookup map built from the
  conversation history). Gemini does not return tool call IDs, so the adapter
  synthesizes them using `generateCallID`. Supports `thoughtSignature`
  round-tripping on tool calls via `content.ToolCall.Metadata`. Sanitizes JSON
  schemas by recursively stripping `$schema` and `additionalProperties` keys
  that the Gemini API rejects. Default max tokens: 8192.
  **Prompt caching**: Gemini has implicit caching (90% discount on Gemini 2.5+).
  The adapter captures `cachedContentTokenCount` from `usageMetadata` and maps
  it to `CacheReadInputTokens` in the usage tracker. Does not implement
  `RateLimitInfoReporter`.

## Exported Types and Constructors

### `anthropic`

- **`Adapter`** -- Composes `*modeladapter.Client`, `modeladapter.ModelConfig`, `usage.Tracker`. Implements `Completer`, `UsageReporter`, `RateLimitInfoReporter`.
- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates a configured adapter.
- **`BatchSubmitter`** -- Batch processing. Created via `NewBatchSubmitter(adapter)`.

### `openai`

- **`Adapter`** -- Composes `*modeladapter.Client`, `modeladapter.ModelConfig`, `usage.Tracker`. Implements `Completer`, `UsageReporter`, `RateLimitInfoReporter`.
- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates a configured adapter.
- **`BatchSubmitter`** -- Batch processing via `openaicompat.BatchHelper`. Created via `NewBatchSubmitter(adapter)`.

### `grok`

- **`Adapter`** -- Composes `*modeladapter.Client`, `modeladapter.ModelConfig`, `usage.Tracker`. Implements `Completer`, `UsageReporter`, `RateLimitInfoReporter`.
- **`New(baseURL, apiKey, model string, httpClient *http.Client) *Adapter`** -- Creates a configured adapter. A nil client falls back to `http.DefaultClient`.
- **`DefaultBaseURL`** -- Constant: `https://api.x.ai`.
- **`BatchSubmitter`** -- Batch processing via `openaicompat.BatchHelper`. Created via `NewBatchSubmitter(adapter)`.

### `gemini`

- **`Adapter`** -- Composes `*modeladapter.Client`, `modeladapter.ModelConfig`, `usage.Tracker`. Implements `Completer`, `UsageReporter`.
- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates a configured adapter.
- **`BatchSubmitter`** -- Batch processing. Created via `NewBatchSubmitter(adapter)`.

## Dependencies

- `pkg/chats/` -- Provider-agnostic chat data model
- `pkg/modeladapter/` -- `Client` (HTTP transport), `ModelConfig`, `Completer` interface, rate limit handling
- `pkg/modeladapter/usage/` -- `Tracker` for token count accumulation
- `pkg/modeladapter/batch/` -- `Request`/`Result` types for batch processing
- `pkg/tools/toolbox/` -- Tool definition type

## Use Cases

- Adding a new LLM provider: create a new sub-package that composes
  `modeladapter.Client` + `modeladapter.ModelConfig` + `usage.Tracker`,
  implements `Complete`, and handles the provider's API format. If the provider
  uses an OpenAI-compatible API, delegate to `internal/openaicompat`.
- The engine (`pkg/engine`) selects the appropriate provider sub-package based
  on YAML config and injects it into agents.
