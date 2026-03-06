# Agent Context Identity System

## Overview

The `pkg/agentctx` package provides shared context key helpers for propagating agent identity across package boundaries in Shelly's multi-agent system. This zero-dependency package solves the critical problem of making agent identity available throughout the system without creating import cycles.

## Core Problem

In a multi-agent system, many packages need to know **which agent** is currently executing for:
- **Logging**: Attributing log entries to specific agents
- **Task Attribution**: Tracking which agent performed what work  
- **Event Routing**: Sending events to appropriate agent handlers
- **Resource Management**: Agent-specific resource allocation
- **Debugging**: Tracing execution through multiple agents

The natural Go solution is storing the agent name in `context.Context`, but the context key must be accessible to all consumers without creating circular dependencies.

## Architecture

### Zero Dependencies
```go
// No imports from other pkg/ packages
// Only standard library dependencies
import "context"
```

This design allows both `pkg/agent` (which creates agents) and `pkg/engine` (which orchestrates agents) to import `agentctx` without circular dependencies.

### Type-Safe Context Keys
```go
type agentNameCtxKey struct{}

// Private type ensures no collisions with external context keys
```

## Core API

### Setting Agent Identity
```go
func WithAgentName(ctx context.Context, name string) context.Context
```

**Purpose**: Create a new context carrying the specified agent name
**Usage**: Called when spawning agent routines or delegating to sub-agents

```go
// Example: Starting an agent
ctx = agentctx.WithAgentName(ctx, "code-reviewer")
go agent.Run(ctx, task)

// Example: Delegating to sub-agent  
subCtx := agentctx.WithAgentName(ctx, "file-analyzer")
result := subAgent.Process(subCtx, files)
```

### Retrieving Agent Identity
```go
func AgentName(ctx context.Context) string
```

**Purpose**: Extract the current agent name from context
**Returns**: Agent name string, or empty string if not set
**Usage**: Called by any package needing to know the current agent

```go
// Example: Logging with agent attribution
logger.Info("Processing file", 
    "agent", agentctx.AgentName(ctx),
    "file", filename)

// Example: Event routing
eventBus.Send(Event{
    Agent: agentctx.AgentName(ctx),
    Type:  "task_completed",
    Data:  result,
})
```

### Agent Name Sanitization
```go
func SanitizeName(name string) string
```

**Purpose**: Clean agent names for safe use in filenames, logs, etc.
**Transformations**:
- Convert to lowercase
- Replace spaces and special chars with hyphens  
- Remove consecutive hyphens
- Trim leading/trailing hyphens

```go
// Examples:
SanitizeName("Code Reviewer")     // -> "code-reviewer" 
SanitizeName("AI Agent #1")       // -> "ai-agent-1"
SanitizeName("Multi::Scope::Bot") // -> "multi-scope-bot"
```

## Usage Patterns

### Agent Lifecycle Management
```go
func (a *Agent) Start(ctx context.Context) {
    // Inject agent identity into context
    ctx = agentctx.WithAgentName(ctx, a.name)
    
    // All operations now carry agent identity
    a.processTaskQueue(ctx)
}
```

### Cross-Package Agent Awareness
```go
// In pkg/state - state store operations
func (s *Store) Set(ctx context.Context, key, value string) {
    agent := agentctx.AgentName(ctx) 
    s.logOperation(agent, "set", key)
    // ... actual storage logic
}

// In pkg/tools - tool execution tracking  
func (t *Tool) Execute(ctx context.Context, args map[string]any) {
    agent := agentctx.AgentName(ctx)
    metrics.RecordToolUse(agent, t.name)
    // ... tool execution logic
}
```

### Event System Integration
```go
// Event publishers include agent context
func (e *EventBus) PublishTaskComplete(ctx context.Context, result TaskResult) {
    e.Send(Event{
        Agent:     agentctx.AgentName(ctx),
        Type:      "task_completed", 
        Timestamp: time.Now(),
        Data:      result,
    })
}

// Event handlers can filter by agent
func (h *Handler) HandleEvent(event Event) {
    if event.Agent == "security-scanner" {
        h.processSecurityResult(event.Data)
    }
}
```

### Logging and Observability
```go
// Structured logging with agent context
func logWithAgent(ctx context.Context, level, msg string, fields ...any) {
    agent := agentctx.AgentName(ctx)
    if agent != "" {
        fields = append(fields, "agent", agent)
    }
    logger.Log(level, msg, fields...)
}

// Usage throughout the codebase
logWithAgent(ctx, "info", "Starting file analysis", "file", path)
// Output: {"level":"info","msg":"Starting file analysis","agent":"file-analyzer","file":"/src/main.go"}
```

## Multi-Agent Coordination

### Parent-Child Relationships
```go
func (parent *Agent) delegateTask(ctx context.Context, task Task) {
    // Child inherits parent context but gets new identity
    childName := fmt.Sprintf("%s-worker-%d", parent.name, task.ID)
    childCtx := agentctx.WithAgentName(ctx, childName)
    
    child := NewAgent(childName)
    return child.Execute(childCtx, task)
}
```

### Context Switching
```go
func (orchestrator *Orchestrator) coordinateAgents(ctx context.Context) {
    // Different contexts for different agents
    analyzerCtx := agentctx.WithAgentName(ctx, "analyzer")
    reviewerCtx := agentctx.WithAgentName(ctx, "reviewer")
    writerCtx := agentctx.WithAgentName(ctx, "writer")
    
    // Parallel execution with proper identity
    go analyzer.Process(analyzerCtx, input)
    go reviewer.Process(reviewerCtx, draft)  
    go writer.Process(writerCtx, outline)
}
```

## Design Benefits

### Dependency Inversion
- Higher-level packages (agent, engine) depend on lower-level (agentctx)
- Prevents circular dependencies in complex multi-agent systems
- Enables clean architecture with proper layering

### Type Safety
- Private context key type prevents accidental collisions
- Compile-time safety for context operations
- No risk of string-based key conflicts

### Zero Allocation
- Context values use efficient storage mechanisms
- Minimal runtime overhead for agent identity propagation
- No memory allocation in steady-state operations

### Consistent Identity
- Single source of truth for agent names throughout system
- Sanitization ensures safe usage across all contexts (files, logs, metrics)
- Uniform representation across different system components

## Integration Points

The agentctx package enables agent identity awareness in:

- **pkg/agent**: Sets identity context when creating/running agents
- **pkg/engine**: Manages agent lifecycles with proper context
- **pkg/state**: Attributes state changes to specific agents
- **pkg/tasks**: Tracks task ownership and completion
- **pkg/tools**: Records tool usage per agent
- **pkg/codingtoolbox**: Logs file operations with agent attribution
- **External packages**: Any code needing agent identity for logging/routing

## Thread Safety

All operations in agentctx are thread-safe:
- Context operations are inherently safe for concurrent use
- No mutable global state
- No synchronization required in consumer code

This package forms a critical part of Shelly's foundational layer, enabling clean separation of concerns while maintaining agent identity throughout the execution flow.