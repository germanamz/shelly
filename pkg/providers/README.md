# providers

An abstraction layer for LLM completion providers. The providers package defines the shared configuration and interface that concrete provider adapters (OpenAI, Anthropic, local models, etc.) must implement. It depends on the `chatty` package for its chat, message, and content types.

## Architecture

```
providers/
├── model/      Provider-agnostic model configuration (name, temperature, max tokens)
├── provider/   Completer interface + embeddable Provider base struct with HTTP helpers
└── usage/      Thread-safe token usage tracker
```

### `model` — Model Configuration

`Model` holds provider-agnostic LLM settings shared by all adapters:

| Field         | Type      | Description                       |
|---------------|-----------|-----------------------------------|
| `Name`        | `string`  | Model identifier (e.g. `"gpt-4"`) |
| `Temperature` | `float64` | Sampling temperature               |
| `MaxTokens`   | `int`     | Maximum tokens in the response     |

The zero value is valid — zero fields mean "use provider defaults". `Model` is designed to be **embedded** in provider-specific config structs so that shared settings are always available without duplication.

### `provider` — Completer Interface & Provider Base

Defines the `Completer` interface that all provider adapters must satisfy:

```go
type Completer interface {
    Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}
```

`Complete` sends a conversation to an LLM and returns the assistant's reply as a `message.Message`. It accepts the full `chat.Chat` so the provider has access to the entire conversation history, system prompt, and any tool-call context.

`Provider` is an embeddable base struct that provides HTTP helpers, authentication, custom headers, and usage tracking. It implements `Completer` with a stub that returns an error — concrete types embed `Provider` and define their own `Complete` method to shadow the stub:

| Field     | Type                | Description                                    |
|-----------|---------------------|------------------------------------------------|
| `Model`   | `model.Model`       | Embedded model configuration                   |
| `Auth`    | `Auth`              | API key, header name, and scheme               |
| `BaseURL` | `string`            | API base URL (no trailing slash)               |
| `Client`  | `*http.Client`      | HTTP client (falls back to `http.DefaultClient`) |
| `Headers` | `map[string]string` | Extra headers applied to every request         |
| `Usage`   | `usage.Tracker`     | Token usage tracker                            |

Key methods:
- `NewRequest` — builds an `*http.Request` with base URL, auth, and custom headers applied
- `PostJSON` — marshals payload, sends POST, checks 2xx, unmarshals response into dest
- `Do` — low-level passthrough to `Client.Do`

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

Embed `provider.Provider` to inherit HTTP helpers, auth, and usage tracking. Define your own `Complete` method to shadow the base stub:

```go
// OpenAI is a concrete provider that calls the OpenAI chat completions API.
type OpenAI struct {
    provider.Provider
}

// NewOpenAI creates an OpenAI provider with the given API key and model config.
func NewOpenAI(apiKey string, m model.Model) *OpenAI {
    return &OpenAI{
        Provider: provider.NewProvider(
            "https://api.openai.com",
            provider.Auth{Key: apiKey},  // defaults to Authorization: Bearer <key>
            m,
            nil,  // uses http.DefaultClient
        ),
    }
}

// Complete shadows the Provider stub — converts the chat to the OpenAI wire
// format, calls the API via PostJSON, tracks usage, and returns the reply.
func (o *OpenAI) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
    req := toOpenAIRequest(c, o.Model)

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

### Using a Provider with the Agent

The `Completer` interface lets the Agent accept any concrete provider:

```go
// NewOpenAI returns a *OpenAI, which satisfies provider.Completer
p := NewOpenAI(os.Getenv("OPENAI_API_KEY"), model.Model{
    Name:        "gpt-4",
    MaxTokens:   1024,
})

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
    provider.Provider
}

func NewAnthropic(apiKey string, m model.Model) *Anthropic {
    p := &Anthropic{
        Provider: provider.NewProvider(
            "https://api.anthropic.com",
            provider.Auth{Key: apiKey, Header: "x-api-key"},
            m,
            nil,
        ),
    }
    p.Headers = map[string]string{"anthropic-version": "2024-01-01"}

    return p
}
```

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
p := NewOpenAI(apiKey, m)

p.Complete(ctx, chat1)
p.Complete(ctx, chat2)

fmt.Println(p.Usage.Count())              // 2
fmt.Println(p.Usage.Total().InputTokens)  // sum of both calls
fmt.Println(p.Usage.Total().Total())      // total input + output

last, _ := p.Usage.Last()
fmt.Println(last.OutputTokens)            // output tokens from chat2
```
