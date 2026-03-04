# modeladapter

An abstraction layer for LLM completion adapters. The modeladapter package defines the shared configuration and interface that concrete provider adapters (OpenAI, Anthropic, Grok, etc.) must implement. It depends on the `chats` package for its chat, message, and content types, and on `tools/toolbox` for tool declarations.

## Architecture

```
modeladapter/
├── doc.go               Package documentation
├── modeladapter.go      Client type with HTTP/WebSocket helpers, auth, and custom headers;
│                        Auth struct; ModelConfig struct; ClientOption functional options
├── completer.go         Completer, UsageReporter, and RateLimitInfoReporter interfaces
├── error.go             RateLimitError and ParseRetryAfter
├── ratelimitinfo.go     RateLimitInfo struct, RateLimitHeaderParser type,
│                        Anthropic/OpenAI header parsers
├── ratelimit.go         RateLimitedCompleter — proactive TPM/RPM throttling with
│                        reactive 429 retry, exponential backoff, and jitter
├── tokenestimator.go    Pre-call token estimation using character-to-token heuristics
└── usage/               Thread-safe token usage tracker (TokenCount, Tracker)
```

### `Completer` — Core Interface

All provider adapters must satisfy the `Completer` interface:

```go
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)
}
```

`Complete` sends a conversation to an LLM and returns the assistant's reply as a `message.Message`. It accepts the full `chat.Chat` (conversation history, system prompt, tool-call context) and the list of available tools for this call.

### `UsageReporter` — Token Usage Interface

Providers that track token usage implement this interface:

```go
type UsageReporter interface {
    UsageTracker() *usage.Tracker
    ModelMaxTokens() int
}
```

`UsageTracker` returns a pointer to the adapter's token usage tracker. `ModelMaxTokens` returns the configured maximum output tokens per response. This interface is consumed by `RateLimitedCompleter` to track token consumption for throttling, and by any code that needs usage statistics from a completer.

### `Client` — HTTP/WebSocket Transport

`Client` provides HTTP and WebSocket transport with auth, custom headers, rate limit header parsing, and rate limit info storage. It does NOT implement `Completer` — concrete providers compose a `*Client` and implement `Complete` themselves.

```go
client := modeladapter.NewClient(baseURL, auth, opts...)
```

Construction uses functional options:

| Option              | Description                                              |
|---------------------|----------------------------------------------------------|
| `WithHTTPClient`    | Use a custom `*http.Client` (nil uses a cached default)  |
| `WithHeaders`       | Extra headers applied to every request                   |
| `WithHeaderParser`  | Parser for rate limit response headers                   |

Key methods:

| Method               | Description                                                                               |
|----------------------|-------------------------------------------------------------------------------------------|
| `NewRequest`         | Builds an `*http.Request` with base URL, auth, and custom headers applied                 |
| `PostJSON`           | Marshals payload, sends POST, checks 2xx, unmarshals response into dest                   |
| `Do`                 | Low-level passthrough to the underlying HTTP client                                        |
| `DialWS`             | Establishes a WebSocket connection with auth and custom headers (scheme auto-converted)    |
| `LastRateLimitInfo`  | Returns the most recently observed `RateLimitInfo`, or nil                                 |

`PostJSON` automatically returns a `*RateLimitError` on HTTP 429 responses (with `RetryAfter` parsed from the response header). On successful 2xx responses, if a header parser is set, it parses rate limit headers and stores the resulting `RateLimitInfo` atomically.

### `ModelConfig` — Model Settings

```go
type ModelConfig struct {
    Name        string  // Model identifier (e.g. "gpt-4").
    Temperature float64 // Sampling temperature.
    MaxTokens   int     // Maximum tokens in the response.
}
```

### `Auth` — Authentication Settings

```go
type Auth struct {
    Key    string // API key value.
    Header string // Header name (default: "Authorization").
    Scheme string // Scheme prefix (default: "Bearer" when Header is "Authorization").
}
```

When `Header` is empty, it defaults to `"Authorization"` with a `"Bearer"` scheme prefix. Custom headers like `"x-api-key"` are set directly without a scheme prefix unless `Scheme` is explicitly provided.

### `RateLimitError` — HTTP 429 Error Type

```go
type RateLimitError struct {
    RetryAfter time.Duration
    Body       string
}
```

Returned by `PostJSON` when the API responds with HTTP 429 (Too Many Requests). Carries the optional `RetryAfter` duration parsed from the `Retry-After` header via `ParseRetryAfter`, which supports both integer seconds and RFC 7231 HTTP-date formats.

### `RateLimitInfo` — Provider Rate Limit State

```go
type RateLimitInfo struct {
    RemainingRequests int
    RemainingTokens   int
    RequestsReset     time.Time
    TokensReset       time.Time
}
```

Parsed from provider-specific response headers after each successful API call. Two built-in parsers are provided:

| Parser                            | Header prefix                        | Used by         |
|-----------------------------------|--------------------------------------|-----------------|
| `ParseAnthropicRateLimitHeaders`  | `anthropic-ratelimit-*`              | Anthropic       |
| `ParseOpenAIRateLimitHeaders`     | `x-ratelimit-*`                      | OpenAI, Grok    |

Both parsers support reset times as either RFC 3339 timestamps or Go duration strings (e.g. `"30s"`, `"1m30s"`) relative to the provided `now` parameter. Returns nil if no rate limit headers are present.

The `RateLimitInfoReporter` interface exposes the stored info:

```go
type RateLimitInfoReporter interface {
    LastRateLimitInfo() *RateLimitInfo
}
```

`Client` implements this interface. `RateLimitedCompleter` consumes it for adaptive pre-emptive throttling.

### `RateLimitedCompleter` — Rate-Limited Completer Wrapper

`RateLimitedCompleter` wraps any `Completer` with three layers of rate limiting:

1. **Proactive throttling** -- tracks input/output tokens and request counts in a 1-minute sliding window and blocks before calls that would exceed configured TPM/RPM limits.
2. **Reactive retry** -- catches `*RateLimitError` (HTTP 429) and retries with exponential backoff and +/-25% jitter. Uses `RetryAfter` from the error when it exceeds the computed backoff.
3. **Adaptive throttling** -- after a successful call, if the inner completer implements `RateLimitInfoReporter` and reports near-zero remaining capacity (requests or tokens <= 1), it pre-emptively sleeps until the provider's reset time.

Calls to the inner `Complete` are serialized with a mutex so that token usage diffs (before/after) are computed correctly when the inner completer implements `UsageReporter`.

```go
type RateLimitOpts struct {
    InputTPM   int           // Input tokens per minute (0 = no limit).
    OutputTPM  int           // Output tokens per minute (0 = no limit).
    RPM        int           // Requests per minute (0 = no limit).
    MaxRetries int           // Max retries on 429 (default 3).
    BaseDelay  time.Duration // Initial backoff delay (default 1s).
}

rl := modeladapter.NewRateLimitedCompleter(inner, modeladapter.RateLimitOpts{
    InputTPM:  100000,
    OutputTPM: 50000,
    RPM:       60,
})
```

`RateLimitedCompleter` also implements `UsageReporter`, forwarding `UsageTracker()` and `ModelMaxTokens()` to the inner completer if it implements `UsageReporter`, or falling back to a stable internal tracker.

Test hooks are provided as functional options: `WithNowFunc`, `WithSleepFunc`, `WithRandFunc`.

### `TokenEstimator` — Pre-Call Token Estimation

`TokenEstimator` estimates token counts for chat messages and tool definitions before sending a request. It uses a character-to-token heuristic (approximately 1 token per 4 characters for English text) with per-message and per-tool structural overhead.

| Method               | Description                                       |
|----------------------|---------------------------------------------------|
| `EstimateChat(c)`    | Estimates input tokens for a chat conversation    |
| `EstimateTools(tools)` | Estimates token cost of tool definitions        |
| `EstimateTotal(c, tools)` | Combined estimate (chat + tools)             |

The estimator is intentionally simple -- accuracy within ~20% is sufficient for threshold-based decisions like compaction triggers. The zero value is ready to use.

```go
var estimator modeladapter.TokenEstimator
tokens := estimator.EstimateTotal(chat, tools)
if tokens > contextWindow * 0.8 {
    // trigger compaction
}
```

### `usage` — Token Usage Tracker

`Tracker` accumulates `TokenCount` entries across multiple LLM calls. It is thread-safe via `sync.Mutex`. The zero value is ready to use.

`TokenCount` holds input and output token counts for a single LLM call, including provider cache metrics:

```go
type TokenCount struct {
    InputTokens              int
    OutputTokens             int
    CacheCreationInputTokens int // Tokens written to provider cache (Anthropic).
    CacheReadInputTokens     int // Tokens read from provider cache (all providers).
}
```

- `TokenCount.Total()` returns the sum of input and output tokens.
- `TokenCount.CacheSavings()` returns the ratio of cache-read tokens to total input tokens (`cache_read / (cache_read + cache_creation + input)`). Returns 0 if there are no input tokens. This is useful for displaying cache hit ratios in the TUI.

Cache fields are populated automatically by providers that support prompt caching:
- **Anthropic**: `CacheCreationInputTokens` and `CacheReadInputTokens` from `cache_creation_input_tokens` / `cache_read_input_tokens` response fields.
- **OpenAI**: `CacheReadInputTokens` from `prompt_tokens_details.cached_tokens`.
- **Gemini**: `CacheReadInputTokens` from `usageMetadata.cachedContentTokenCount`.

`Tracker` methods:

| Method    | Description                                      |
|-----------|--------------------------------------------------|
| `Add`     | Records a token count entry                      |
| `Last`    | Returns the most recent entry (and a bool)       |
| `Total`   | Returns aggregate counts (including cache fields) |
| `Count`   | Returns the number of recorded entries            |
| `Reset`   | Clears all entries                                |

## Examples

### Implementing a Concrete Provider

Compose a `*modeladapter.Client` for HTTP transport and a `modeladapter.ModelConfig` for model settings. Implement `Completer` directly:

```go
type OpenAI struct {
    client *modeladapter.Client
    Config modeladapter.ModelConfig
    usage  usage.Tracker
}

func NewOpenAI(apiKey, model string) *OpenAI {
    return &OpenAI{
        client: modeladapter.NewClient(
            "https://api.openai.com",
            modeladapter.Auth{Key: apiKey},
            modeladapter.WithHeaderParser(modeladapter.ParseOpenAIRateLimitHeaders),
        ),
        Config: modeladapter.ModelConfig{
            Name:      model,
            MaxTokens: 4096,
        },
    }
}

func (o *OpenAI) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
    req := toOpenAIRequest(c, o.Config.Name, o.Config.Temperature, o.Config.MaxTokens, tools)

    var resp openAIResponse
    if err := o.client.PostJSON(ctx, "/v1/chat/completions", req, &resp); err != nil {
        return message.Message{}, err
    }

    tc := usage.TokenCount{
        InputTokens:  resp.Usage.PromptTokens,
        OutputTokens: resp.Usage.CompletionTokens,
    }
    o.usage.Add(tc)

    return toMessage(resp), nil
}

func (o *OpenAI) UsageTracker() *usage.Tracker { return &o.usage }
func (o *OpenAI) ModelMaxTokens() int          { return o.Config.MaxTokens }
```

### Custom Auth and Headers (Anthropic Example)

Providers with non-standard auth (e.g. `x-api-key` header instead of Bearer) configure it through `Auth`:

```go
client := modeladapter.NewClient(
    "https://api.anthropic.com",
    modeladapter.Auth{Key: apiKey, Header: "x-api-key"},
    modeladapter.WithHeaders(map[string]string{"anthropic-version": "2023-06-01"}),
    modeladapter.WithHeaderParser(modeladapter.ParseAnthropicRateLimitHeaders),
)
```

### Wrapping with Rate Limiting

Wrap any `Completer` with `RateLimitedCompleter` to add TPM/RPM throttling and 429 retry:

```go
p := NewOpenAI(os.Getenv("OPENAI_API_KEY"), "gpt-4")

rl := modeladapter.NewRateLimitedCompleter(p, modeladapter.RateLimitOpts{
    InputTPM:   100000,
    OutputTPM:  50000,
    RPM:        60,
    MaxRetries: 3,
    BaseDelay:  time.Second,
})

// rl satisfies Completer and UsageReporter, so it can be used anywhere
// a Completer is expected.
msg, err := rl.Complete(ctx, myChat, myTools)
```

### WebSocket Streaming

For providers that offer WebSocket-based APIs, use `DialWS` to establish a connection with auth and headers pre-applied:

```go
conn, _, err := client.DialWS(ctx, "/v1/realtime?model=gpt-4o-realtime")
if err != nil {
    return err
}
defer conn.CloseNow()

// Send and receive messages over the WebSocket...
```

The URL scheme is derived from the base URL automatically: `https` becomes `wss`, `http` becomes `ws`.

### Low-Level HTTP Access

For APIs that don't fit `PostJSON`, use `NewRequest` and `Do` directly:

```go
req, err := client.NewRequest(ctx, http.MethodGet, "/v1/models", nil)
if err != nil {
    return nil, err
}

resp, err := client.Do(req)
if err != nil {
    return nil, err
}
defer resp.Body.Close()
// ... decode response
```

### Tracking Token Usage

The `usage.Tracker` accumulates token counts across calls:

```go
p := NewOpenAI(apiKey, "gpt-4")

p.Complete(ctx, chat1, tools)
p.Complete(ctx, chat2, tools)

tracker := p.UsageTracker()
fmt.Println(tracker.Count())              // 2
fmt.Println(tracker.Total().InputTokens)  // sum of both calls
fmt.Println(tracker.Total().Total())      // total input + output

last, _ := tracker.Last()
fmt.Println(last.OutputTokens)            // output tokens from chat2
```

## Dependencies

- `pkg/chats/chat` -- `Chat` type (conversation container)
- `pkg/chats/message` -- `Message` type (individual messages)
- `pkg/chats/content` -- `Text`, `ToolCall`, `ToolResult` content parts (used by `TokenEstimator`)
- `pkg/chats/role` -- `Role` constants (used by `TokenEstimator` to identify system messages)
- `pkg/tools/toolbox` -- `Tool` type (tool declarations passed to `Complete`)
- `github.com/coder/websocket` -- WebSocket client for `DialWS`
