# Model Adapter Abstraction Layer

## Overview

`pkg/modeladapter` defines the unified interface and shared infrastructure for LLM completion adapters. Concrete providers (Anthropic, OpenAI, Grok, Gemini) implement `Completer`. The package also provides rate limiting, usage tracking, token estimation, batch support, and a shared HTTP/WebSocket client.

**Dependencies:** `pkg/chats` (chat, message, content, role), `pkg/tools/toolbox`

## File Layout

```
modeladapter/
├── doc.go                  Package doc
├── completer.go            Completer, UsageReporter, RateLimitInfoReporter interfaces
├── modeladapter.go         Auth, ModelConfig, Client (HTTP + WebSocket)
├── ratelimit.go            RateLimitedCompleter (proactive + reactive rate limiting)
├── ratelimitinfo.go        RateLimitInfo struct, header parsing, RateLimitInfoReporter
├── error.go                RateLimitError, ParseRetryAfter
├── agent_usage_completer.go  AgentUsageCompleter (per-agent usage isolation)
├── tokenestimator.go       TokenEstimator (heuristic token counting)
├── usage/
│   ├── usage.go            TokenCount, Tracker (thread-safe accumulator)
│   ├── pricing.go          ModelPricing, LookupPricing (embedded + override YAML)
│   └── pricing.yaml        Default pricing data for all providers/models
└── batch/
    ├── batch.go            Submitter interface, Request/Result types
    └── collector.go        Collector (batching decorator for Completer)
```

## Core Interfaces

### Completer

The central interface every provider must implement. Signature:

```go
type Completer interface {
    Complete(ctx context.Context, ch *chat.Chat, tools []toolbox.Tool) (message.Message, error)
}
```

- Takes a full `*chat.Chat` (system prompt + message history) and available `[]toolbox.Tool`
- Returns a single `message.Message` (the assistant reply) and an error
- Providers translate chats/tools to their wire format, call the API, parse the response back

### UsageReporter

Separate interface for retrieving token usage from a completer:

```go
type UsageReporter interface {
    UsageTracker() *usage.Tracker
    ModelMaxTokens() int
}
```

- `UsageTracker()` returns a `*usage.Tracker` that accumulates all calls' token counts
- `ModelMaxTokens()` returns the model's context window size (from `ModelConfig.MaxTokens`)
- Not all Completers implement this — use type assertion to check

### RateLimitInfoReporter

Separate interface for accessing provider rate limit headers:

```go
type RateLimitInfoReporter interface {
    RateLimitInfo() RateLimitInfo
}
```

- `RateLimitInfo` holds: `RemainingRequests`, `RemainingTokens`, `RequestsReset`, `TokensReset`
- Populated from standard rate-limit response headers via `ParseRateLimitHeaders(http.Header)`
- Supports Anthropic-style (`anthropic-ratelimit-*`) and generic (`x-ratelimit-*`, `ratelimit-*`) header prefixes

## Client Type

`Client` provides shared HTTP and WebSocket infrastructure for all providers:

```go
type Client struct {
    Auth       Auth        // Key, Header ("Authorization"), Scheme ("Bearer")
    BaseURL    string
    HTTPClient *http.Client
    Headers    http.Header // Extra headers added to every request
}
```

**Key methods:**

| Method | Purpose |
|--------|---------|
| `Do(ctx, method, path, body) (*http.Response, error)` | HTTP request with auth + custom headers |
| `DoJSON(ctx, method, path, body, out) (*http.Response, error)` | Do + JSON decode response body |
| `Post(ctx, path, body, out) (*http.Response, error)` | Shorthand POST + JSON |
| `Stream(ctx, method, path, body) (*http.Response, error)` | HTTP streaming (returns open body) |
| `WebSocket(ctx, path) (*websocket.Conn, error)` | WebSocket dial with auth headers |

- Auth header defaults to `Authorization: Bearer <key>`; customizable via `Auth.Header` and `Auth.Scheme`
- Uses `sync.Once` to lazily initialize `http.Client` (30s timeout, no redirect follow)
- Thread-safe; shared across concurrent calls

## ModelConfig

```go
type ModelConfig struct {
    Name      string   // Model identifier (e.g., "claude-sonnet-4-20250514")
    MaxTokens int      // Context window size
    Thinking  bool     // Enable extended thinking (provider-specific)
    CacheTTL  string   // Cache time-to-live hint (e.g., "ephemeral")
    StopWords []string // Optional stop sequences
}
```

## Rate Limiting

### RateLimitedCompleter

Wraps any `Completer` with dual-layer rate limiting:

**Proactive (TPM sliding window):** Tracks token usage over a 60-second sliding window. Before each call, estimates request tokens via `TokenEstimator`. If adding them would exceed `TPMLimit`, sleeps until enough budget frees up.

**Reactive (429 retry):** On `RateLimitError`, applies exponential backoff with jitter up to `MaxRetries`. Honors `RetryAfter` from the error, using `max(retryAfter, calculatedBackoff)`.

```go
type RateLimitOpts struct {
    TPMLimit   int           // Tokens-per-minute limit; 0 = disabled
    MaxRetries int           // Max 429 retries (default 5)
    BaseDelay  time.Duration // Base delay for exponential backoff (default 5s)
}
```

**Constructor:** `NewRateLimitedCompleter(inner Completer, opts RateLimitOpts) *RateLimitedCompleter`

- Implements `Completer`, `UsageReporter`, and `RateLimitInfoReporter` by delegation
- Sliding window uses a `[]tokenEntry` ring with mutex protection
- Jitter formula: `baseDelay * 2^attempt * (0.5 + rand*0.5)`

### RateLimitError

Returned by providers on HTTP 429:

```go
type RateLimitError struct {
    RetryAfter time.Duration
    Body       string
}
```

`ParseRetryAfter(val string) time.Duration` — parses seconds (int) or HTTP-date (RFC 7231).

## Usage Tracking

### usage.TokenCount

```go
type TokenCount struct {
    InputTokens              int
    OutputTokens             int
    CacheCreationInputTokens int
    CacheReadInputTokens     int
}
```

- `Total()` → `InputTokens + OutputTokens`
- `CacheSavings()` → `CacheReadInputTokens + CacheCreationInputTokens`

### usage.Tracker

Thread-safe accumulator (`sync.Mutex`-protected):

| Method | Description |
|--------|-------------|
| `Add(tc TokenCount)` | Appends to history, updates totals |
| `Last() (TokenCount, bool)` | Most recent call's token counts |
| `Totals() TokenCount` | Cumulative totals across all calls |
| `CallCount() int` | Number of recorded calls |
| `Merge(other *Tracker)` | Absorbs another tracker's full history |
| `CostUSD(provider, model string) float64` | Calculates total cost via pricing table |

### usage.ModelPricing & LookupPricing

- `ModelPricing` — per-token prices in USD per 1M tokens: input, output, cache-read, cache-creation
- `CalculateCost(tc TokenCount, p ModelPricing) float64` — computes cost for a single call
- `LookupPricing(providerKind, modelID string) (ModelPricing, bool)` — longest-prefix match in pricing table
- Pricing loaded lazily from embedded `pricing.yaml`; user overrides via `SetOverridePath()` (typically `.shelly/local/pricing.yaml`)

## AgentUsageCompleter

Wraps a shared `Completer` to isolate per-agent usage tracking:

```go
type AgentUsageCompleter struct {
    inner   Completer
    tracker usage.Tracker   // per-agent accumulation
    mu      sync.Mutex      // protects Complete+Last() atomicity
}
```

- Implements both `Completer` and `UsageReporter`
- `Complete()` delegates to inner, then copies `Last()` from inner's tracker into its own
- Uses mutex to ensure the `Complete → UsageTracker().Last()` pair is atomic (no interleaving from concurrent agents sharing the same inner completer)
- Created via `NewAgentUsageCompleter(inner Completer) *AgentUsageCompleter`

## Token Estimation

`TokenEstimator` provides heuristic token counting for rate limiting and context management:

```go
type TokenEstimator struct{}
```

| Method | Description |
|--------|-------------|
| `EstimateChat(ch *chat.Chat, tools []toolbox.Tool) int` | Total: system + messages + tools |
| `EstimateMessages(msgs []message.Message) int` | Sum of message estimates |
| `EstimateMessage(msg message.Message) int` | Per-message overhead (4) + content blocks |
| `EstimateTools(tools []toolbox.Tool) int` | Tool definitions (name + description + JSON schema) |

**Heuristic:** 1 token ≈ 4 characters. Constants: `perMessageOverhead = 4`, `perToolOverhead = 10`.

Content blocks are estimated by type: text → chars/4, tool-use → name + JSON args, tool-result → content, thinking → text, image → fixed 1000, document → fixed 2000.

## Batch Support (`batch/` subpackage)

### Submitter Interface

Provider-specific batch API abstraction:

```go
type Submitter interface {
    SubmitBatch(ctx context.Context, reqs []Request) (batchID string, err error)
    PollBatch(ctx context.Context, batchID string) (results map[string]Result, done bool, err error)
    CancelBatch(ctx context.Context, batchID string) error
}
```

- `Request` — `{ID string, Chat *chat.Chat, Tools []toolbox.Tool}`
- `Result` — `{Message message.Message, Usage usage.TokenCount, Err error}`

### Collector

Batching decorator that collects concurrent `Complete()` calls and submits them as a single batch:

```go
collector := batch.NewCollector(innerCompleter, submitter, batch.CollectorOpts{
    CollectWindow: 500 * time.Millisecond, // Wait for more requests before flushing
    PollInterval:  5 * time.Second,        // Batch status poll interval
    Timeout:       time.Hour,              // Max wait for batch result
    MaxBatchSize:  100,                    // Auto-flush threshold
})
```

**Behavior:**
- Implements `Completer` and `UsageReporter`
- Each `Complete()` call enqueues a request and blocks on a per-request result channel
- Requests accumulate during `CollectWindow`; flush triggers on timer expiry or `MaxBatchSize` reached
- After submission, polls via `Submitter.PollBatch()` until done
- On any error (submit or poll), **falls back to synchronous** completion via inner completer
- Fallback serialized via `fallbackMu` to keep per-call usage tracking correct
- Lifecycle managed via `Stop()` which cancels all in-flight batch operations
- Event callbacks via `SetEventHandler()`: `batch_submitted`, `batch_polling`, `batch_completed`, `batch_fallback`

## Helper

`ContextSleep(ctx context.Context, d time.Duration) error` — sleeps for `d` or returns early if context is cancelled. Used by `RateLimitedCompleter` and `batch.Collector`.

## Decorator Composition Pattern

In practice, completers are layered:

```
Provider Completer (e.g., anthropic.Completer)
  └─ RateLimitedCompleter (proactive TPM + reactive 429 retry)
      └─ AgentUsageCompleter (per-agent usage isolation)
          └─ [optional] batch.Collector (batch cost optimization)
```

All layers implement `Completer`; higher layers delegate to the wrapped inner.
