# Model Adapter Abstraction Layer

## Overview

The `pkg/modeladapter` package defines the abstraction layer for LLM completion adapters. It provides the unified interface that concrete provider implementations (OpenAI, Anthropic, Grok, Gemini) must implement, along with shared infrastructure for configuration, authentication, usage tracking, and rate limiting.

## Core Interface

### Completer Interface
The central abstraction that all provider adapters must implement:

```go
type Completer interface {
    Complete(ctx context.Context, chat *chat.Chat, tools *toolbox.Toolbox) (message.Message, *usage.Usage, error)
}
```

**Purpose**: Send a conversation to an LLM and return the assistant's response
**Parameters**:
- `ctx`: Context for cancellation and timeouts
- `chat`: The conversation history
- `tools`: Available tools for this completion call
**Returns**: Assistant message, usage statistics, and any error

## Configuration Types

### Auth Configuration
```go
type Auth struct {
    APIKey    string `yaml:"api_key"`
    BaseURL   string `yaml:"base_url,omitempty"`
    OrgID     string `yaml:"org_id,omitempty"`
}
```

### Model Configuration
```go
type Config struct {
    Provider     string             `yaml:"provider"`
    Model        string             `yaml:"model"`
    Auth         Auth               `yaml:"auth"`
    RateLimit    *RateLimitConfig   `yaml:"rate_limit,omitempty"`
    BatchConfig  *BatchConfig       `yaml:"batch,omitempty"`
    Options      map[string]any     `yaml:"options,omitempty"`
}
```

**Key Fields**:
- `Provider`: String identifier ("openai", "anthropic", "grok", "gemini")
- `Model`: Specific model name (e.g., "gpt-4o", "claude-3-5-sonnet")
- `Auth`: Authentication settings
- `RateLimit`: Request throttling configuration
- `Options`: Provider-specific parameters (temperature, max_tokens, etc.)

## Usage Tracking

### Usage Statistics
```go
type Usage struct {
    PromptTokens     int           `json:"prompt_tokens"`
    CompletionTokens int           `json:"completion_tokens"`
    TotalTokens     int           `json:"total_tokens"`
    RequestTime     time.Duration `json:"request_time"`
    Cached          bool          `json:"cached,omitempty"`
}
```

### Token Estimation
```go
type TokenEstimator interface {
    EstimateTokens(content string) int
}
```

Providers can implement this interface to provide accurate token counting before API calls.

## Rate Limiting

### Rate Limit Configuration
```go
type RateLimitConfig struct {
    RequestsPerMinute *int `yaml:"requests_per_minute,omitempty"`
    TokensPerMinute   *int `yaml:"tokens_per_minute,omitempty"`
    Concurrent        *int `yaml:"concurrent,omitempty"`
}
```

### Built-in Rate Limiter
```go
type RateLimiter struct {
    // Token bucket implementation for both requests and tokens
    // Thread-safe with proper synchronization
}
```

**Features**:
- Dual limiting: requests per minute AND tokens per minute
- Concurrent request limiting
- Token bucket algorithm with refill rates
- Context-aware waiting with cancellation support

## Error Handling

### Structured Errors
```go
type CompletionError struct {
    Type     string `json:"type"`
    Message  string `json:"message"`
    Code     string `json:"code,omitempty"`
    Param    string `json:"param,omitempty"`
}

func (e *CompletionError) Error() string
func (e *CompletionError) Temporary() bool
func (e *CompletionError) RateLimited() bool
```

**Error Types**:
- Authentication errors
- Rate limit exceeded
- Invalid request parameters
- Model capacity issues
- Network timeouts

## HTTP Infrastructure

### HTTP Configuration
```go
type HTTPConfig struct {
    Client      *http.Client        `yaml:"-"`
    Timeout     *time.Duration      `yaml:"timeout,omitempty"`
    Headers     map[string]string   `yaml:"headers,omitempty"`
    Retry       *RetryConfig        `yaml:"retry,omitempty"`
    WebSocket   *WebSocketConfig    `yaml:"websocket,omitempty"`
}
```

### Retry Logic
```go
type RetryConfig struct {
    MaxRetries  int           `yaml:"max_retries"`
    InitialWait time.Duration `yaml:"initial_wait"`
    MaxWait     time.Duration `yaml:"max_wait"`
    Multiplier  float64       `yaml:"multiplier"`
}
```

**Features**:
- Exponential backoff with jitter
- Configurable retry conditions
- Context-aware cancellation
- Provider-specific retry policies

## Batch Processing

### Batch Configuration
```go
type BatchConfig struct {
    Enabled        bool          `yaml:"enabled"`
    MaxBatchSize   int           `yaml:"max_batch_size"`
    MaxWaitTime    time.Duration `yaml:"max_wait_time"`
    CheckInterval  time.Duration `yaml:"check_interval"`
}
```

**Capabilities**:
- Automatic request batching for efficiency
- Provider-specific batch limits
- Timeout-based batch flushing
- Cost optimization for high-volume scenarios

## WebSocket Support

For streaming completions and real-time interactions:

```go
type WebSocketConfig struct {
    Enabled             bool          `yaml:"enabled"`
    HandshakeTimeout    time.Duration `yaml:"handshake_timeout"`
    ReadTimeout         time.Duration `yaml:"read_timeout"`
    WriteTimeout        time.Duration `yaml:"write_timeout"`
    PingInterval        time.Duration `yaml:"ping_interval"`
    MaxMessageSize      int64         `yaml:"max_message_size"`
}
```

## Provider Integration Patterns

### Adapter Implementation
Providers implement the `Completer` interface with this general pattern:

```go
type Provider struct {
    config     Config
    client     *http.Client
    rateLimiter *RateLimiter
    estimator   TokenEstimator
}

func (p *Provider) Complete(ctx context.Context, chat *chat.Chat, tools *toolbox.Toolbox) (message.Message, *usage.Usage, error) {
    // 1. Wait for rate limit clearance
    // 2. Convert chats/messages to provider format
    // 3. Include tool declarations if provided
    // 4. Make HTTP request with retries
    // 5. Parse response and usage statistics
    // 6. Convert response back to chats/message format
    // 7. Return message, usage, error
}
```

### Tool Integration
Providers handle tools through the `toolbox.Toolbox` parameter:
- Convert tool declarations to provider-specific format
- Handle tool calls in assistant responses
- Support tool result injection for continued conversation

### Usage Reporting
All providers must return accurate `usage.Usage` statistics:
- Token counts (prompt, completion, total)
- Request latency timing
- Cache hit indicators where supported

## Key Design Principles

### Provider Agnostic
- Common interface works across all LLM providers
- Provider-specific details encapsulated in implementations
- Consistent error handling and retry logic

### Performance Oriented
- Built-in rate limiting prevents API quota exhaustion
- Batch processing support for high-volume scenarios  
- Token estimation reduces unnecessary API calls
- Connection pooling and keep-alive support

### Configuration Driven
- YAML-based configuration with environment variable overrides
- Sensible defaults with full customization capability
- Runtime configuration updates for dynamic behavior

### Observability Ready
- Structured error reporting with categorization
- Comprehensive usage statistics collection
- Context propagation for distributed tracing
- Metrics-friendly interfaces

## Integration with Foundation Layer

The modeladapter package serves as the bridge between:

- **chats**: Consumes/produces the provider-agnostic data model
- **tools/toolbox**: Integrates available tools into completions
- **Agent System**: Provides the completion capability for ReAct loops
- **Engine Layer**: Configuration and lifecycle management

This abstraction enables Shelly to support multiple LLM providers through a unified interface while providing enterprise-grade reliability features like rate limiting, retries, and comprehensive error handling.