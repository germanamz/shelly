# Shelly Foundation Layer Overview

## Layer Architecture

The Shelly foundation layer consists of four core packages that provide zero-dependency abstractions upon which all other system components are built:

```
Foundation Layer (Layer 1)
├── pkg/chats/        # Provider-agnostic chat data model
├── pkg/agentctx/     # Agent identity context helpers  
└── pkg/shellydir/    # .shelly/ directory path management

Abstraction Layer (Layer 2) 
└── pkg/modeladapter/ # LLM completion interface & infrastructure
```

## Design Principles

### Zero Dependencies at Foundation
The foundational packages (`chats`, `agentctx`, `shellydir`) have **zero dependencies** on other `pkg/` packages, only importing from the Go standard library. This enables:

- **Clean Architecture**: Higher layers depend on lower layers, never the reverse
- **No Import Cycles**: Foundation packages can be imported by any other package
- **Minimal Coupling**: Core abstractions remain stable and focused
- **Easy Testing**: Foundation types can be tested in isolation

### Provider Agnostic Design
All foundation types are designed to work uniformly across different LLM providers:

- **chats**: Data model works with OpenAI, Anthropic, Grok, Gemini APIs
- **modeladapter**: Common interface abstracts provider differences  
- **agentctx**: Agent identity works regardless of provider or model
- **shellydir**: Directory structure independent of provider configuration

### Value-Oriented APIs
Foundation packages favor value types and immutable data structures:

- **chats.Message**: Value type that copies cheaply, immutable after creation
- **shellydir.Dir**: Immutable path resolver, safe to pass by value
- **agentctx**: Stateless functions operating on context values
- **usage.TokenCount**: Simple value type for accumulating statistics

## Inter-Package Relationships

### Dependency Flow
```
┌─────────────────────────────────┐
│         Higher Layers           │  
│   (agent, engine, providers)    │
└─────────────┬───────────────────┘
              │ depends on
┌─────────────▼───────────────────┐
│      pkg/modeladapter/          │
│   (completion abstraction)      │  
└─────────────┬───────────────────┘
              │ depends on
┌─────────────▼───────────────────┐
│     Foundation Layer            │
│ chats + agentctx + shellydir    │
│    (zero dependencies)          │
└─────────────────────────────────┘
```

### Data Flow Patterns

**Chat Conversations**: 
`chats/chat.Chat` → `modeladapter.Completer` → `providers/*` → LLM APIs

**Agent Identity**:
`agentctx.WithAgentName()` → Context propagation → `agentctx.AgentName()` in any package

**Project Structure**:
`shellydir.FindFromCwd()` → Path resolution → All file operations use `Dir` paths

**Usage Tracking**:
LLM API responses → `usage.TokenCount` → `usage.Tracker` → Aggregated statistics

## Key Abstractions

### Chat Data Model (pkg/chats)

**Core Types**:
- `role.Role` - Four standard conversation roles (system, user, assistant, tool)  
- `content.Part` - Multi-modal content interface (text, images, tool calls/results)
- `message.Message` - Single conversation message with role, parts, metadata
- `chat.Chat` - Thread-safe mutable conversation container

**Key Pattern**: **Extensible Content System**
```go
type Part interface {
    PartKind() string  // Enables serialization/routing
}
```

Custom content types can implement this interface to extend the system while maintaining provider compatibility.

### Completion Abstraction (pkg/modeladapter)

**Core Interface**:
```go
type Completer interface {
    Complete(ctx context.Context, chat *chat.Chat, tools *toolbox.Toolbox) (message.Message, *usage.Usage, error)
}
```

**Key Pattern**: **Unified Provider Interface**
- All LLM providers implement the same interface
- Consistent error handling and usage reporting
- Built-in rate limiting and retry logic
- Configuration-driven behavior

### Agent Identity (pkg/agentctx)

**Core Functions**:
```go
func WithAgentName(ctx context.Context, name string) context.Context
func AgentName(ctx context.Context) string
```

**Key Pattern**: **Context-Based Identity Propagation**
- Zero-dependency enables universal import
- Type-safe context keys prevent collisions  
- Enables agent attribution across all system operations

### Directory Management (pkg/shellydir)

**Core Type**:
```go
type Dir struct { root string }  // Immutable path resolver
```

**Key Pattern**: **Centralized Path Knowledge**
- Single source of truth for `.shelly/` structure
- Consistent paths across all components
- Supports project discovery and initialization

## Advanced Features

### Multi-Modal Content Support
The foundation layer supports rich content from the ground up:
- **Text**: Plain text messages and responses
- **Images**: Base64-encoded or URL-referenced images
- **Tool Calls**: Structured function calls with JSON arguments
- **Tool Results**: Execution results linked to specific calls
- **Custom Types**: Extensible content system via `Part` interface

### Usage Tracking & Analytics
Comprehensive token usage tracking at the foundation level:
- **Real-time Tracking**: `usage.Tracker` with O(1) aggregation
- **Cache Analytics**: Tracks cache hit ratios for cost optimization
- **Ring Buffer**: Recent entries for trend analysis
- **Thread-Safe**: Concurrent usage tracking across multiple agents

### Rate Limiting & Resilience
Built into the model adapter abstraction:
- **Dual Limits**: Requests per minute AND tokens per minute
- **Token Bucket**: Efficient rate limiting algorithm
- **Exponential Backoff**: Configurable retry strategies
- **Context Awareness**: Respects cancellation and timeouts

### Batch Processing Support
Optional batching for high-volume scenarios:
- **Automatic Batching**: Transparent request aggregation
- **Configurable Limits**: Max batch size and wait time
- **Cost Optimization**: Reduces API call overhead
- **Provider Specific**: Adapts to each provider's batch capabilities

## Integration Patterns

### Engine Integration
```go
// Engine uses all foundation packages
dir, _ := shellydir.FindFromCwd()
config := loadConfig(dir.Config())
completer := createCompleter(config.Model)
ctx = agentctx.WithAgentName(ctx, "main-agent")

chat := &chat.Chat{}
chat.System("You are a helpful assistant")
response, usage, err := completer.Complete(ctx, chat, tools)
```

### Agent Integration
```go
// Agents leverage identity and conversations
func (a *Agent) process(ctx context.Context, task Task) {
    ctx = agentctx.WithAgentName(ctx, a.name)
    
    chat := a.conversation // *chat.Chat
    chat.User(task.Description)
    
    response, _, err := a.completer.Complete(ctx, chat, a.tools)
    if err != nil {
        return fmt.Errorf("completion failed for agent %s: %w", 
            agentctx.AgentName(ctx), err)
    }
    
    chat.Add(response)
    return a.processResponse(ctx, response)
}
```

### Provider Integration  
```go
// Providers implement the foundation abstractions
type AnthropicProvider struct {
    config modeladapter.Config
    client *http.Client
}

func (p *AnthropicProvider) Complete(ctx context.Context, chat *chat.Chat, tools *toolbox.Toolbox) (message.Message, *usage.Usage, error) {
    // Convert chats.Message → Anthropic API format
    // Make HTTP request with rate limiting
    // Convert Anthropic response → chats.Message
    // Return usage statistics
}
```

## Evolution and Extensibility

### Content Type Extensions
New content types can be added without breaking existing code:
```go
type VideoContent struct {
    URL      string
    Duration time.Duration
}

func (v VideoContent) PartKind() string { return "video" }
```

### Provider Extensions
New LLM providers integrate by implementing the `Completer` interface:
```go
type NewProvider struct { /* ... */ }

func (p *NewProvider) Complete(ctx, chat, tools) (message.Message, *usage.Usage, error) {
    // Provider-specific implementation
}
```

### Metadata Extensions
Extensible metadata supports provider-specific data:
```go
msg := message.Message{
    Role: role.Assistant,
    Parts: []content.Part{content.Text{Text: "Hello"}},
    Metadata: map[string]any{
        "model_version": "claude-3-5-sonnet-20241022",
        "cached": true,
        "latency_ms": 250,
    },
}
```

## Testing Strategy

### Foundation Package Testing
- **Unit Tests**: Each package has comprehensive test coverage
- **Integration Tests**: Cross-package interactions tested
- **Mock Implementations**: Test doubles for expensive operations
- **Property Tests**: Validate invariants across all operations

### Example Test Patterns
```go
// chats: Test conversation building
func TestChatOperations(t *testing.T) {
    chat := &chat.Chat{}
    chat.System("test system")
    chat.User("test user")
    
    assert.Len(t, chat.Messages(), 2)
    assert.Equal(t, role.System, chat.Messages()[0].Role)
}

// modeladapter: Test completer interface  
func TestCompleterInterface(t *testing.T) {
    completer := &mockCompleter{}
    chat := &chat.Chat{}
    chat.User("Hello")
    
    response, usage, err := completer.Complete(ctx, chat, nil)
    assert.NoError(t, err)
    assert.Equal(t, role.Assistant, response.Role)
    assert.Greater(t, usage.Total(), 0)
}
```

This foundation layer provides the robust, extensible base that enables Shelly's sophisticated multi-agent orchestration capabilities while maintaining clean architecture principles and zero-dependency design at the core.