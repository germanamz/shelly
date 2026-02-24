# modeladapter

An abstraction layer for LLM completion adapters. The modeladapter package defines the shared configuration and interface that concrete provider adapters (OpenAI, Anthropic, local models, etc.) must implement. It depends on the `chats` package for its chat, message, and content types.

## Architecture

```
modeladapter/
├── modeladapter.go      Completer interface + embeddable ModelAdapter base struct with HTTP/WebSocket helpers
├── tokenestimator.go    Pre-call token estimation using character-to-token heuristics
├── toolaware.go         ToolAware interface for providers that accept tool declarations
└── usage/               Thread-safe token usage tracker
```

### `ModelAdapter` — Completer Interface & ModelAdapter Base

Defines the `Completer` interface that all provider adapters must satisfy:

```go
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}
```

`Complete` sends a conversation to an LLM and returns the assistant's reply as a `message.Message`. It accepts the full `chat.Chat` so the provider has access to the entire conversation history, system prompt, and any tool-call context.

`ModelAdapter` is an embeddable base struct that provides HTTP helpers, authentication, custom headers, and usage tracking. It implements `Completer` with a stub that returns an error — concrete types embed `ModelAdapter` and define their own `Complete` method to shadow the stub:

| Field         | Type                | Description                                      |
|---------------|---------------------|--------------------------------------------------|
| `Name`        | `string`            | Model identifier (e.g. `"gpt-4"`)               |
| `Temperature` | `float64`           | Sampling temperature                             |
| `MaxTokens`   | `int`               | Maximum tokens in the response                   |
| `Auth`        | `Auth`              | API key, header name, and scheme                 |
| `BaseURL`     | `string`            | API base URL (no trailing slash)                 |
| `Client`      | `*http.Client`      | HTTP client (falls back to `http.DefaultClient`) |
| `Headers`     | `map[string]string` | Extra headers applied to every request           |
| `Usage`       | `usage.Tracker`     | Token usage tracker                              |

Key methods:
- `NewRequest` — builds an `*http.Request` with base URL, auth, and custom headers applied
- `PostJSON` — marshals payload, sends POST, checks 2xx, unmarshals response into dest
- `Do` — low-level passthrough to `Client.Do`
- `DialWS` — establishes a WebSocket connection with auth and custom headers applied (scheme auto-converted from http/https to ws/wss)

### `ToolAware` — Optional Tool Declaration Interface

`ToolAware` is an optional interface that `Completer` implementations can satisfy to receive tool declarations from the engine:

```go
type ToolAware interface {
    SetTools(tools []toolbox.Tool)
}
```

The engine calls `SetTools` before creating agents so the provider knows which tools to declare in API requests. Both the `anthropic.Adapter` and `openai.Adapter` implement this interface. Providers that don't support tools (or manage them differently) can simply not implement it — the engine checks with a type assertion.

### `TokenEstimator` — Pre-Call Token Estimation

`TokenEstimator` estimates token counts for chat messages and tool definitions before sending a request. It uses a character-to-token heuristic (approximately 1 token per 4 characters for English text) with per-message and per-tool structural overhead.

| Method | Description |
|--------|-------------|
| `EstimateChat(c)` | Estimates input tokens for a chat conversation |
| `EstimateTools(tools)` | Estimates token cost of tool definitions |
| `EstimateTotal(c, tools)` | Combined estimate (chat + tools) |

The estimator is intentionally simple — accuracy within ~20% is sufficient for threshold-based decisions like compaction triggers. The zero value is ready to use.

```go
estimator := &modeladapter.TokenEstimator{}
tokens := estimator.EstimateTotal(chat, tools)
if tokens > contextWindow * 0.8 {
    // trigger compaction
}
```

### `usage` — Token Usage Tracker

`Tracker` accumulates `TokenCount` entries across multiple LLM calls. It is thread-safe via `sync.Mutex`.

| Method    | Description                                      |
|-----------|--------------------------------------------------|
| `Add`     | Records a token count entry                      |
| `Last`    | Returns the most recent entry                    |
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
            nil,                        // uses http.DefaultClient
        ),
    }
    o.Name = "gpt-4"
    o.MaxTokens = 1024

    return o
}

// Complete shadows the ModelAdapter stub — converts the chat to the OpenAI wire
// format, calls the API via PostJSON, tracks usage, and returns the reply.
func (o *OpenAI) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
    req := toOpenAIRequest(c, o.Name, o.Temperature, o.MaxTokens)

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

p.Complete(ctx, chat1)
p.Complete(ctx, chat2)

fmt.Println(p.Usage.Count())              // 2
fmt.Println(p.Usage.Total().InputTokens)  // sum of both calls
fmt.Println(p.Usage.Total().Total())      // total input + output

last, _ := p.Usage.Last()
fmt.Println(last.OutputTokens)            // output tokens from chat2
```
