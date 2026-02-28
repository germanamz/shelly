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

## Architecture

All providers follow the same pattern:

1. **Embed `modeladapter.ModelAdapter`** -- Provides HTTP helpers (`PostJSON`,
   `NewRequest`, `Do`), authentication, custom headers, rate limit handling,
   and token usage tracking.
2. **Implement `modeladapter.Completer`** -- The single-method interface
   `Complete(ctx, chat, tools) (message, error)` that the agent's ReAct loop
   calls on each iteration.
3. **Convert messages** -- Transform `pkg/chats` types (Text, ToolCall,
   ToolResult, roles) into the provider's API format and parse responses back
   into `message.Message` with `content.Part` slices.
4. **Track usage** -- Accumulate input/output token counts via
   `adapter.Usage.Add()` after each successful API call.

### Provider Differences

- **Anthropic** sends the system prompt as a top-level `system` field and places
  tool results in `"user"` role messages. Tool schemas use `input_schema`.
  Default max tokens: 4096. Uses `ParseAnthropicRateLimitHeaders` for rate
  limit handling. Sends a custom `anthropic-version: 2023-06-01` header.
- **OpenAI** sends the system prompt as a `"system"` role message in the messages
  array. Tool definitions use the `{"type":"function","function":{...}}` wrapper
  with `parameters`. Default max tokens: 4096. Uses
  `ParseOpenAIRateLimitHeaders` for rate limit handling.
- **Grok** follows the OpenAI-compatible format (same message structure and tool
  definitions). Its constructor accepts an `*http.Client` and uses
  `modeladapter.New()` rather than direct field assignment. The model name is
  set after construction. Default base URL: `https://api.x.ai`. Uses
  `ParseOpenAIRateLimitHeaders`. Exports `MarshalToolDef` as a convenience for
  building tool definitions in the OpenAI-compatible format.
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

## Exported Types and Constructors

### `anthropic`

- **`Adapter`** -- Embeds `modeladapter.ModelAdapter`. Implements `Completer`.
- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates a configured adapter.

### `openai`

- **`Adapter`** -- Embeds `modeladapter.ModelAdapter`. Implements `Completer`.
- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates a configured adapter.

### `grok`

- **`GrokAdapter`** -- Embeds `modeladapter.ModelAdapter`. Implements `Completer`.
- **`New(apiKey string, client *http.Client) *GrokAdapter`** -- Creates a configured adapter. A nil client falls back to `http.DefaultClient`.
- **`MarshalToolDef(name, description string, schema json.RawMessage) apiTool`** -- Convenience helper to build an OpenAI-compatible tool definition.
- **`DefaultBaseURL`** -- Constant: `https://api.x.ai`.

### `gemini`

- **`Adapter`** -- Embeds `modeladapter.ModelAdapter`. Implements `Completer`.
- **`New(baseURL, apiKey, model string) *Adapter`** -- Creates a configured adapter.

## Dependencies

- `pkg/chats/` -- Provider-agnostic chat data model
- `pkg/modeladapter/` -- Base adapter struct, `Completer` interface, HTTP helpers, usage tracking
- `pkg/tools/toolbox/` -- Tool definition type

## Use Cases

- Adding a new LLM provider: create a new sub-package that embeds
  `modeladapter.ModelAdapter`, implements `Complete`, and handles the
  provider's API format.
- The engine (`pkg/engine`) selects the appropriate provider sub-package based
  on YAML config and injects it into agents.
