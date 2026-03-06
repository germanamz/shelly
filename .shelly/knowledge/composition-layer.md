# Composition Layer: Skills and Engine

The composition layer of Shelly brings together all components into a cohesive, configurable system. This layer consists of two key packages:

- **`pkg/skill/`** - Folder-based skill loading with YAML frontmatter for procedural knowledge
- **`pkg/engine/`** - Composition root that wires everything from YAML config with Engine/Session/EventBus API

## Skills System (`pkg/skill/`)

### Overview

The skills system provides a way to define reusable procedural knowledge that agents can access on-demand. Skills are markdown-based instructional content stored in dedicated folders with optional YAML frontmatter for metadata.

### Skill Structure

Each skill consists of:

```
skill_folder/
├── SKILL.md          # Required entry point with instructions
├── templates/        # Optional: template files
├── scripts/         # Optional: helper scripts  
└── docs/           # Optional: additional documentation
```

### Skill Data Model

```go
type Skill struct {
    Name        string            // Skill identifier (folder name)
    Content     string            // Full markdown content from SKILL.md
    Dir         string            // Absolute path to skill folder
    Frontmatter map[string]any    // Parsed YAML frontmatter (optional)
}
```

### YAML Frontmatter System

Skills can include YAML frontmatter for metadata:

```markdown
---
description: "Step-by-step guide for project indexing"
tags: ["documentation", "analysis"]
estimated_time: "10-15 minutes"
prerequisites: ["filesystem access", "search tools"]
---

# Project Indexing Skill

This skill teaches agents how to systematically index a codebase...
```

The frontmatter is parsed into a `map[string]any` and made available to agents, allowing for rich metadata-driven skill selection and usage.

### Skill Loading

**`skill.Load(path string) (Skill, error)`**
- Loads a single skill from a folder containing `SKILL.md`
- Parses optional YAML frontmatter if present
- Returns error if `SKILL.md` is missing or YAML is invalid
- Sets `Dir` field to absolute path of folder

**`skill.LoadDir(rootPath string) ([]Skill, error)`**  
- Recursively discovers and loads all skills under a root directory
- Each immediate subdirectory containing `SKILL.md` becomes a skill
- Skill name derived from folder name
- Handles loading errors gracefully (logs warnings, continues)

### Skill Store

The `Store` type manages loaded skills and exposes them via a tool interface:

```go
type Store struct {
    skills  map[string]Skill
    workDir string  // For path sanitization
}
```

**Key Features:**
- **Tool Integration**: Exposes `load_skill` tool for on-demand skill retrieval
- **Path Sanitization**: Converts absolute paths to relative ones to avoid machine-specific leakage
- **Agent Access**: Agents can dynamically load skills during execution using the `load_skill` tool

**Tool Interface:**
```json
{
    "name": "load_skill",
    "description": "Load the full content of a skill by name.",
    "input_schema": {
        "type": "object", 
        "properties": {
            "name": {"type": "string", "description": "Name of the skill to load"}
        },
        "required": ["name"]
    }
}
```

## Engine System (`pkg/engine/`)

### Architecture Overview

The engine serves as the **composition root** - the central place where all Shelly components are wired together from configuration. It provides a frontend-agnostic API that shields clients from implementation details.

**Core Components:**
- **Engine**: Main orchestrator, manages configuration, providers, toolboxes
- **Session**: Individual conversation contexts with agents
- **EventBus**: Observable activity stream for monitoring and debugging
- **BatchSession**: Parallel task execution for high-throughput scenarios

### Configuration System

The engine is configured via YAML with comprehensive environment variable expansion:

```yaml
# .shelly/config.yaml
entry_agent: "assistant"

providers:
  - name: "claude"
    kind: "anthropic" 
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-3-5-sonnet-20241022"

agents:
  - name: "assistant"
    description: "General purpose coding assistant"
    provider: "claude"
    toolboxes: ["coding", "skills"]
    skills: []  # Empty means all loaded skills
    
mcp_servers:
  - name: "filesystem"
    command: "npx"
    args: ["@modelcontextprotocol/server-filesystem", "/path/to/project"]
```

### Configuration Structures

**Core Configuration:**
```go
type Config struct {
    ShellyDir    string           // Set by CLI
    Providers    []ProviderConfig // LLM provider configurations  
    MCPServers   []MCPServerConfig // External tool servers
    Agents       []AgentConfig    // Agent definitions
    EntryAgent   string          // Default agent name
    Filesystem   FilesystemConfig // Built-in tool settings
    Git         GitConfig        // Git integration settings
    // ... other settings
}
```

**Agent Configuration:**
```go
type AgentConfig struct {
    Name         string       // Unique agent identifier
    Description  string       // Human-readable description
    Instructions string       // System prompt/instructions
    Provider     string       // Which LLM provider to use
    Toolboxes    []ToolboxRef // Available tool collections
    Skills       []string     // Skill names (empty = all skills)
    Effects      []EffectConfig // Middleware for message processing
    Options      AgentOptions  // Behavior settings
    Prefix       string       // Display prefix (e.g. "🤖")
}
```

**Toolbox References:**
```go
type ToolboxRef struct {
    Name  string   // Toolbox identifier
    Tools []string // Optional tool filter (empty = all tools)
}
```

### Engine Initialization

The engine follows a structured initialization process:

1. **Configuration Loading & Validation**
   - Parse YAML config file
   - Expand environment variables
   - Validate required fields and references

2. **Directory Setup** 
   - Ensure `.shelly/` directory structure exists
   - Migrate legacy permission files
   - Create required subdirectories

3. **Parallel Initialization** (`parallelInit`)
   - **Skills Loading**: Discover and load skills from `.shelly/skills/` and project-level directories
   - **Project Context**: Build structural project index for enhanced agent context  
   - **MCP Servers**: Connect to external tool servers concurrently
   - All three operations run in parallel to minimize startup latency

4. **Provider Initialization**
   - Create configured LLM provider instances
   - Set up rate limiting and batch processing capabilities
   - Build provider registry for agent access

5. **Toolbox Wiring**  
   - Initialize built-in toolboxes (filesystem, git, http, etc.)
   - Connect MCP server tools
   - Wire skill store as `load_skill` tool
   - Apply tool filtering per agent configuration

6. **Agent Registry Setup**
   - Create agent factory functions from configuration
   - Set up effect middleware chains  
   - Register agents in global registry

### Skills Integration in Engine

Skills are deeply integrated into the engine's configuration and runtime:

**Configuration:**
```yaml
agents:
  - name: "indexer"  
    skills: ["project-indexer", "documentation-writer"]  # Specific skills
  - name: "assistant"
    skills: []  # Empty = all available skills
```

**Runtime Integration:**
1. **Skill Loading**: Engine discovers skills from multiple sources:
   - `.shelly/skills/` (project-specific skills)
   - Built-in skill directories
   - Additional paths from configuration

2. **Skill Store Creation**: All loaded skills are placed in a centralized store

3. **Tool Exposure**: Skills are exposed to agents via the `load_skill` tool in configured toolboxes

4. **Dynamic Access**: Agents can query available skills and load them on-demand during execution

### Session Management

**Session Lifecycle:**
```go
type Session struct {
    ID       string    // Unique session identifier
    AgentID  string    // Which agent handles this session  
    Chat     *chat.Chat // Conversation history
    EventBus *EventBus // Activity monitoring
    // ... internal state
}
```

**Key Operations:**
- **`SendMessage(content)`**: Send user message, trigger agent processing
- **`GetMessages()`**: Retrieve conversation history
- **`Close()`**: Clean up session resources

**Session Features:**
- **Context Isolation**: Each session maintains independent conversation state
- **Event Streaming**: All activity (messages, tool calls, agent spawns) emitted via EventBus
- **Agent Delegation**: Sessions can spawn child agents for specialized tasks
- **Error Recovery**: Graceful handling of agent failures and timeouts

### Event System

The EventBus provides observable streams of engine activity:

**Event Types:**
```go
const (
    EventMessageAdded       EventKind = "message_added"
    EventToolCallStart      EventKind = "tool_call_start" 
    EventToolCallEnd        EventKind = "tool_call_end"
    EventAgentStart         EventKind = "agent_start"
    EventAgentEnd          EventKind = "agent_end"
    EventSessionStart      EventKind = "session_start"
    EventSessionEnd        EventKind = "session_end"
)
```

**Event Structure:**
```go
type Event struct {
    Kind      EventKind     // Event type
    Timestamp time.Time     // When event occurred
    SessionID string        // Which session generated event
    AgentID   string        // Which agent was involved
    Data      any          // Event-specific payload
}
```

**Usage:**
- **Monitoring**: Track agent activity and performance
- **Debugging**: Observe tool calls and decision making
- **Integration**: Stream events to external systems
- **UI Updates**: Real-time updates in TUI/web interfaces

### Batch Processing

For high-throughput scenarios, the engine supports batch processing:

**Batch Task Format:**
```json
{"id": "task1", "agent": "assistant", "task": "Review this code", "context": "optional"}
{"id": "task2", "agent": "indexer", "task": "Document this API", "context": "..."}
```

**Batch Features:**
- **Parallel Execution**: Configurable concurrency (default: 8 concurrent tasks)
- **Independent Sessions**: Each task gets its own session context
- **Cost Optimization**: Batch collector can group LLM requests for provider savings  
- **Result Streaming**: Results written to JSONL as tasks complete
- **Error Isolation**: Failed tasks don't affect others

### Toolbox Integration

The engine manages the complex task of wiring tools from multiple sources:

**Built-in Toolboxes:**
- `coding`: Filesystem, exec, search, git operations
- `http`: Web requests and API interactions  
- `notes`: Persistent note-taking across sessions
- `permissions`: File access control
- `skills`: Access to the skill store (`load_skill` tool)

**MCP Integration:**
- External tool servers connected via Model Context Protocol
- Both stdio and HTTP transport supported
- Tools automatically discovered and registered
- Lifecycle management (connect, disconnect, error recovery)

**Agent Tool Assignment:**
```yaml
agents:
  - name: "developer"
    toolboxes: 
      - name: "coding"  # All coding tools
      - name: "http" 
        tools: ["fetch", "post"]  # Filtered tool access
    skills: ["debugging", "testing"]
```

## Integration Patterns

### Skills ↔ Engine Integration

1. **Configuration-Driven Loading**: Skills specified in agent configs are automatically loaded and made available

2. **Dynamic Discovery**: Engine can discover skills from multiple directory sources

3. **Tool Wrapping**: Skills exposed as callable tools rather than static content

4. **Context-Aware Access**: Skills can include project-specific context and file references

### Engine ↔ Agent Integration  

1. **Factory Pattern**: Engine creates agent instances from configuration
2. **Dependency Injection**: Agents receive configured providers, toolboxes, and skills
3. **Registry Management**: Engine maintains agent registry for delegation
4. **Lifecycle Management**: Engine handles agent startup, cleanup, and error recovery

### Configuration ↔ Runtime Integration

1. **Environment Expansion**: All config strings support `${VAR}` environment variables
2. **Validation Pipeline**: Comprehensive validation before component initialization  
3. **Hot Reloading**: Some configuration changes can be applied without restart
4. **Override Support**: CLI flags and environment can override config file values

## Key Design Principles

### 1. **Composition Over Inheritance**
- Engine assembles components rather than extending base classes
- Skills are compositional building blocks rather than rigid templates
- Toolboxes combine independently developed tools

### 2. **Configuration as Code**
- Everything configurable via declarative YAML
- Environment variable expansion for deployment flexibility  
- Validation ensures configuration correctness

### 3. **Observable by Default**
- All significant activities emit events
- Comprehensive logging and monitoring built-in
- External integration points for custom observability

### 4. **Frontend Agnostic**
- Engine API doesn't assume specific UI framework
- Event-driven architecture supports real-time updates
- Batch processing for non-interactive use cases

### 5. **Extensibility**
- Plugin architecture for custom tools via MCP
- Skill system for custom procedural knowledge
- Effect system for custom agent behaviors

The composition layer represents the culmination of Shelly's architecture - where all the specialized components come together into a coherent, configurable, and extensible system that users can adapt to their specific needs while maintaining clean separation of concerns.