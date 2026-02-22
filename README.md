# Shelly

A provider-agnostic Go framework for building LLM chat applications. Shelly provides the foundational data model and abstractions needed to work with any large-language-model provider (OpenAI, Anthropic, etc.) without coupling to a specific API.

## Architecture

```
cmd/shelly/           CLI entry point
pkg/
├── chats/            Provider-agnostic chat data model
│   ├── role/           Conversation roles (system, user, assistant, tool)
│   ├── content/        Multi-modal content parts (text, image, tool call/result)
│   ├── message/        Messages composed of a sender, role, and content parts
│   └── chat/           Mutable conversation container
├── modeladapter/     LLM adapter abstraction (Completer, ToolAware, ModelAdapter base, usage tracking)
├── providers/        LLM provider implementations
│   ├── anthropic/      Anthropic Messages API adapter
│   ├── openai/         OpenAI Chat Completions API adapter
│   └── grok/           xAI Grok API adapter
├── tools/            Tool execution and MCP integration
│   ├── toolbox/        Tool type, ToolBox collection, and handlers
│   ├── mcpclient/      MCP client (connects to external MCP servers)
│   └── mcpserver/      MCP server (exposes tools over MCP protocol)
├── skill/            Folder-based skill loading (SKILL.md entry point + supplementary files)
├── agent/            Unified agent with ReAct loop, registry, delegation, and middleware
├── state/            Key-value state store for inter-agent data sharing
└── engine/           Composition root — wires everything from config, exposes Engine/Session/EventBus
```

### chats — Chat Data Model

The `chats` package defines a complete, provider-agnostic data model for LLM conversations. It is the foundation layer that everything else builds on.

- **role** defines `Role` (`System`, `User`, `Assistant`, `Tool`) with validation.
- **content** defines the `Part` interface and four implementations: `Text`, `Image`, `ToolCall`, and `ToolResult`. Custom content types can be added by implementing the single-method `Part` interface.
- **message** combines a `Sender`, `Role`, content `Parts`, and arbitrary `Metadata` into a single value type. The `Sender` field enables multi-agent tracking.
- **chat** is a mutable, ordered collection of messages with filtering (`BySender`), iteration (`Each`), and convenience accessors (`SystemPrompt`, `Last`).

See [`pkg/chats/README.md`](pkg/chats/README.md) for detailed examples.

### providers — Provider Abstraction

The `providers` package defines the interface and shared configuration for LLM completion providers.

- **model** holds provider-agnostic LLM configuration (`Name`, `Temperature`, `MaxTokens`). The zero value is valid and means "use provider defaults". It is designed to be embedded in provider-specific config structs.
- **provider** defines the `Provider` interface:

```go
type Provider interface {
    Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}
```

Concrete adapters (OpenAI, Anthropic, local models, etc.) implement this interface. The rest of the codebase programs against it, staying decoupled from any single API.

See [`pkg/providers/README.md`](pkg/providers/README.md) for detailed examples.

### engine — Composition Root

The `engine` package is the top-level wiring layer that assembles all framework components from a YAML configuration and exposes them through a frontend-agnostic API.

- **Engine** creates provider adapters, connects MCP servers, loads skills, registers agent factories, and manages sessions.
- **Session** represents one interactive conversation — call `Send()` to run the agent loop and get a reply.
- **EventBus** provides a channel-based push model for observing engine activity (agent start/end, tool calls, errors).

Frontends (CLI, web, desktop) interact with `Engine` and `Session` types and never import lower-level packages directly.

See [`pkg/engine/README.md`](pkg/engine/README.md) for configuration details and integration patterns.

## Use Cases

### Basic Conversation

```go
c := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("user", role.User, "What is Go?"),
)

reply, err := myProvider.Complete(ctx, c)
if err != nil {
    log.Fatal(err)
}
c.Append(reply)
```

### Tool Use

```go
// Assistant requests a tool call
assistantMsg := message.New("bot", role.Assistant,
    content.Text{Text: "Let me look that up."},
    content.ToolCall{ID: "call_1", Name: "search", Arguments: `{"q":"golang"}`},
)
c.Append(assistantMsg)

// Feed the tool result back
toolMsg := message.New("", role.Tool,
    content.ToolResult{ToolCallID: "call_1", Content: "Go is a statically typed language..."},
)
c.Append(toolMsg)

// Get the assistant's final response
reply, _ := myProvider.Complete(ctx, c)
```

### Multi-Agent Orchestration

```go
c := chat.New(
    message.NewText("", role.System, "Collaborative session."),
    message.NewText("user", role.User, "Summarise Go's concurrency model."),
)

for _, agent := range []string{"researcher", "critic", "writer"} {
    resp := dispatch(agent, c) // each agent sees the full conversation
    c.Append(message.NewText(agent, role.Assistant, resp))
}

// Inspect a single agent's contributions
for _, m := range c.BySender("critic") {
    fmt.Println(m.TextContent())
}
```

### Conversation Branching

```go
shared := chat.New(
    message.NewText("", role.System, "You are a helpful assistant."),
    message.NewText("user", role.User, "Propose a name for a Go testing library."),
)

// Fork into independent branches
creative := chat.New(shared.Messages()...)
practical := chat.New(shared.Messages()...)
```

### Writing a Provider Adapter

```go
type myAdapter struct {
    model.Model // embed shared config
    apiKey string
}

func (a *myAdapter) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
    // Convert c to the provider's wire format, call the API,
    // and return the result as a message.Message.
}
```

## Prerequisites

- [Go](https://go.dev/dl/) 1.25+
- [go-task](https://taskfile.dev/installation/) (task runner)

### Installing tools

Install all development tools at once:

```bash
go install mvdan.cc/gofumpt@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install gotest.tools/gotestsum@latest
go install github.com/go-task/task/v3/cmd/task@latest
```

Make sure `$(go env GOPATH)/bin` is in your `PATH`:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Quick Start

```bash
task build   # Build binary to ./bin/shelly
task run     # Run without building
./bin/shelly # Run the built binary
```

## Development

### Available tasks

| Task | Description |
|------|-------------|
| `task build` | Build binary to `./bin/shelly` |
| `task run` | Run the application directly |
| `task fmt` | Format code with gofumpt |
| `task fmt:check` | Check formatting without writing (CI-friendly) |
| `task lint` | Run golangci-lint |
| `task lint:fix` | Run golangci-lint with auto-fix |
| `task test` | Run tests with gotestsum |
| `task test:coverage` | Run tests and print coverage by function |
| `task test:coverage:html` | Generate `coverage.html` report |
| `task check` | Run all checks: format, lint, test |

### Typical workflow

```bash
task fmt       # Format your changes
task lint:fix  # Fix lint issues automatically
task test      # Run tests

# Or run everything at once before pushing:
task check
```

## Tooling

| Tool | Purpose | Config |
|------|---------|--------|
| [gofumpt](https://github.com/mvdan/gofumpt) | Code formatting (strict superset of `gofmt`) | - |
| [golangci-lint v2](https://golangci-lint.run/) | Linting (50+ linters) | `.golangci.yml` |
| [testify](https://github.com/stretchr/testify) | Test assertions (`assert`, `require`) | - |
| [gotestsum](https://github.com/gotestyourself/gotestsum) | Formatted test output | - |
| [go-task](https://taskfile.dev/) | Task runner | `Taskfile.yml` |

### Linter configuration

The project uses golangci-lint v2 with the `standard` preset plus these additional linters:

- **gosec** — security-focused analysis
- **gocritic** — opinionated style/performance/bug checks
- **gocyclo** — cyclomatic complexity (threshold: 15)
- **unconvert** — unnecessary type conversions
- **misspell** — common spelling errors
- **modernize** — suggests modern Go idioms
- **testifylint** — catches testify misuse

See `.golangci.yml` for the full configuration.

## Writing Tests

Tests use the standard `testing` package with [testify](https://github.com/stretchr/testify) for assertions:

```go
func TestExample(t *testing.T) {
    got := doSomething()
    assert.Equal(t, "expected", got)
    assert.NoError(t, err)
}
```

Use `require` instead of `assert` when a failure should stop the test immediately:

```go
func TestExample(t *testing.T) {
    result, err := doSomething()
    require.NoError(t, err) // stops here if err != nil
    assert.Equal(t, "expected", result)
}
```
