# modeladapter

An abstraction layer for LLM completion adapters. The modeladapter package defines the shared configuration and interface that concrete provider adapters (OpenAI, Anthropic, Grok, etc.) must implement. It depends on the `chats` package for its chat, message, and content types, and on `tools/toolbox` for tool declarations.

## Architecture

```
modeladapter/
├── doc.go               Package documentation
├── modeladapter.go      Completer interface, UsageReporter interface, RateLimitError,
│                        ParseRetryAfter, embeddable ModelAdapter base struct with
│                        HTTP/WebSocket helpers, auth, and custom headers
├── ratelimitinfo.go     RateLimitInfo struct, RateLimitInfoReporter interface,
│                        RateLimitHeaderParser type, Anthropic/OpenAI header parsers
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

Completers that embed `ModelAdapter` implement this interface automatically:

```go
type UsageReporter interface {
    UsageTracker() *usage.Tracker
    ModelMaxTokens() int
}
```

`UsageTracker` returns a pointer to the adapter's token usage tracker. `ModelMaxTokens` returns the configured maximum output tokens per response. This interface is consumed by `RateLimitedCompleter` to track token consumption for throttling, and by any code that needs usage statistics from a completer.

### `ModelAdapter` — Embeddable Base Struct

`ModelAdapter` is an embeddable base struct that provides HTTP helpers, authentication, custom headers, rate limit header parsing, and usage tracking. It implements `Completer` with a stub that returns an error -- concrete types embed `ModelAdapter` and define their own `Complete` method to shadow the stub.

| Field          | Type                     | Description                                      |
|----------------|--------------------------|--------------------------------------------------|
| `Name`         | `string`                 | Model identifier (e.g. `"gpt-4"`)               |
| `Temperature`  | `float64`                | Sampling temperature                             |
| `MaxTokens`    | `int`                    | Maximum tokens in the response                   |
| `Auth`         | `Auth`                   | API key, header name, and scheme                 |
| `BaseURL`      | `string`                 | API base URL (no trailing slash)                 |
| `Client`       | `*http.Client`           | HTTP client (nil uses a cached default with 10 min timeout) |
| `Headers`      | `map[string]string`      | Extra headers applied to every request           |
| `Usage`        | `usage.Tracker`          | Token usage tracker                              |
| `HeaderParser` | `RateLimitHeaderParser`  | Optional parser for rate limit response headers  |

Key methods:

| Method               | Description                                                                               |
|----------------------|-------------------------------------------------------------------------------------------|
| `NewRequest`         | Builds an `*http.Request` with base URL, auth, and custom headers applied                 |
| `PostJSON`           | Marshals payload, sends POST, checks 2xx, unmarshals response into dest                   |
| `Do`                 | Low-level passthrough to `Client.Do`                                                       |
| `DialWS`             | Establishes a WebSocket connection with auth and custom headers (scheme auto-converted)    |
| `UsageTracker`       | Returns a pointer to the embedded `Usage` tracker (implements `UsageReporter`)             |
| `ModelMaxTokens`     | Returns the configured `MaxTokens` value (implements `UsageReporter`)                      |
| `LastRateLimitInfo`  | Returns the most recently observed `RateLimitInfo`, or nil                                 |

`PostJSON` automatically returns a `*RateLimitError` on HTTP 429 responses (with `RetryAfter` parsed from the response header). On successful 2xx responses, if `HeaderParser` is set, it parses rate limit headers and stores the resulting `RateLimitInfo` atomically.

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

`ModelAdapter` implements this interface. `RateLimitedCompleter` consumes it for adaptive pre-emptive throttling.

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

Test hooks for deterministic testing: `SetNowFunc`, `SetSleepFunc`, `SetRandFunc`.

### `TokenEstimator` — Pre-Call Token Estimation

`TokenEstimator` estimates token counts for chat messages and tool definitions before sending a request. It uses a character-to-token heuristic (approximately 1 token per 4 characters for English text) with per-message and per-tool structural overhead.

| Method               | Description                                       |
|----------------------|---------------------------------------------------|
| `EstimateChat(c)`    | Estimates input tokens for a chat conversation    |
| `EstimateTools(tools)` | Estimates token cost of tool definitions        |
| `EstimateTotal(c, tools)` | Combined estimate (chat + tools)             |

The estimator is intentionally simple -- accuracy within ~20% is sufficient for threshold-based decisions like compaction triggers. The zero value is ready to use.

```go
estimator := &modeladapter.TokenEstimator{}
tokens := estimator.EstimateTotal(chat, tools)
if tokens > contextWindow * 0.8 {
    // trigger compaction
}
```

### `usage` — Token Usage Tracker

`Tracker` accumulates `TokenCount` entries across multiple LLM calls. It is thread-safe via `sync.Mutex`. The zero value is ready to use.

`TokenCount` holds input and output token counts for a single LLM call:

```go
type TokenCount struct {
    InputTokens  int
    OutputTokens int
}
```

`TokenCount.Total()` returns the sum of input and output tokens.

| Method    | Description                                      |
|-----------|--------------------------------------------------|
| `Add`     | Records a token count entry                      |
| `Last`    | Returns the most recent entry (and a bool)       |
| `Total`   | Returns aggregate input/output counts            |
| `Count`   | Returns the number of recorded entries            |
| `Reset`   | Clears all entries                                |

## Examples

### Implementing a Concrete Provider

Embed `modeladapter.ModelAdapter` to inherit HTTP helpers, auth, and usage tracking. Define your own `Complete` method to shadow the base stub:

```go
// OpenAI is a concrete provider that calls the OpenAI chat completions API.
type OpenAI struct {
    modeladapter.ModelAdapter
}

// NewOpenAI creates an OpenAI provider with the given API key and model config.
func NewOpenAI(apiKey string) *OpenAI {
    o := &OpenAI{
        ModelAdapter: modeladapter.New(
            "https://api.openai.com",
            modeladapter.Auth{Key: apiKey},  // defaults to Authorization: Bearer <key>
            nil,                        // uses default client (10 min timeout)
        ),
    }
    o.Name = "gpt-4"
    o.MaxTokens = 1024
    o.HeaderParser = modeladapter.ParseOpenAIRateLimitHeaders

    return o
}

// Complete shadows the ModelAdapter stub -- converts the chat to the OpenAI wire
// format, calls the API via PostJSON, tracks usage, and returns the reply.
func (o *OpenAI) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
    req := toOpenAIRequest(c, o.Name, o.Temperature, o.MaxTokens, tools)

    var resp openAIResponse
    if err := o.PostJSON(ctx, "/v1/chat/completions", req, &resp); err != nil {
        return message.Message{}, err
    }

    o.Usage.Add(usage.TokenCount{
        InputTokens:  resp.Usage.PromptTokens,
        OutputTokens: resp.Usage.CompletionTokens,
    })

    return toMessage(resp), nil
}
```

### Using a ModelAdapter with the Agent

The `Completer` interface lets the Agent accept any concrete model adapter:

```go
// NewOpenAI returns a *OpenAI, which satisfies modeladapter.Completer
p := NewOpenAI(os.Getenv("OPENAI_API_KEY"))

c := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("user", role.User, "Explain goroutines."),
)

a := agent.New("bot", p, c)
reply, err := a.Complete(ctx)
```

### Wrapping with Rate Limiting

Wrap any `Completer` with `RateLimitedCompleter` to add TPM/RPM throttling and 429 retry:

```go
p := NewOpenAI(os.Getenv("OPENAI_API_KEY"))

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

### Custom Auth and Headers (Anthropic Example)

Providers with non-standard auth (e.g. `x-api-key` header instead of Bearer) configure it through `Auth`:

```go
type Anthropic struct {
    modeladapter.ModelAdapter
}

func NewAnthropic(apiKey string) *Anthropic {
    a := &Anthropic{
        ModelAdapter: modeladapter.New(
            "https://api.anthropic.com",
            modeladapter.Auth{Key: apiKey, Header: "x-api-key"},
            nil,
        ),
    }
    a.Name = "claude-sonnet-4-6-20250514"
    a.Headers = map[string]string{"anthropic-version": "2024-01-01"}
    a.HeaderParser = modeladapter.ParseAnthropicRateLimitHeaders

    return a
}
```

### WebSocket Streaming

For providers that offer WebSocket-based APIs (e.g. realtime or streaming endpoints), use `DialWS` to establish a connection with auth and headers pre-applied:

```go
func (o *OpenAI) StreamRealtime(ctx context.Context) error {
    conn, _, err := o.DialWS(ctx, "/v1/realtime?model=gpt-4o-realtime")
    if err != nil {
        return err
    }
    defer conn.CloseNow()

    // Send and receive messages over the WebSocket...
    err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"session.update"}`))
    if err != nil {
        return err
    }

    _, msg, err := conn.Read(ctx)
    if err != nil {
        return err
    }

    fmt.Println(string(msg))

    return conn.Close(websocket.StatusNormalClosure, "done")
}
```

The URL scheme is derived from `BaseURL` automatically: `https` becomes `wss`, `http` becomes `ws`.

### Low-Level HTTP Access

For APIs that don't fit `PostJSON`, use `NewRequest` and `Do` directly:

```go
func (o *OpenAI) ListModels(ctx context.Context) ([]string, error) {
    req, err := o.NewRequest(ctx, http.MethodGet, "/v1/models", nil)
    if err != nil {
        return nil, err
    }

    resp, err := o.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    // ... decode response
}
```

### Tracking Token Usage

The embedded `Usage` tracker accumulates token counts across calls:

```go
p := NewOpenAI(apiKey)

p.Complete(ctx, chat1, tools)
p.Complete(ctx, chat2, tools)

fmt.Println(p.Usage.Count())              // 2
fmt.Println(p.Usage.Total().InputTokens)  // sum of both calls
fmt.Println(p.Usage.Total().Total())      // total input + output

last, _ := p.Usage.Last()
fmt.Println(last.OutputTokens)            // output tokens from chat2
```

## Dependencies

- `pkg/chats/chat` -- `Chat` type (conversation container)
- `pkg/chats/message` -- `Message` type (individual messages)
- `pkg/chats/content` -- `Text`, `ToolCall`, `ToolResult` content parts (used by `TokenEstimator`)
- `pkg/chats/role` -- `Role` constants (used by `TokenEstimator` to identify system messages)
- `pkg/tools/toolbox` -- `Tool` type (tool declarations passed to `Complete`)
- `github.com/coder/websocket` -- WebSocket client for `DialWS`
