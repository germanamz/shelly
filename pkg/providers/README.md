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

| Package       | Provider  | API Endpoint              | Auth Scheme        |
|---------------|-----------|---------------------------|--------------------|
| `anthropic`   | Anthropic | `/v1/messages`            | `x-api-key` header |
| `openai`      | OpenAI    | `/v1/chat/completions`    | `Authorization: Bearer` |
| `grok`        | xAI Grok  | `/v1/chat/completions`    | `Authorization: Bearer` |

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
- **OpenAI** sends the system prompt as a `"system"` role message in the messages
  array. Tool definitions use the `{"type":"function","function":{...}}` wrapper
  with `parameters`.
- **Grok** follows the OpenAI-compatible format. Its constructor accepts an
  `*http.Client` and uses `modeladapter.New()` rather than direct field
  assignment. The model name is set after construction.

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
