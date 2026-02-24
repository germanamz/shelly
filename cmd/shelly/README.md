# shelly

Interactive terminal chat for the Shelly agent framework. Connects to the engine, creates a session with the configured entry agent, and runs a read-eval-print loop with real-time reasoning chain visibility.

## Usage

```bash
shelly [-config path] [-agent name] [-verbose]
shelly init [-shelly-dir path] [-template name]
shelly config [-config path] [-shelly-dir path]
```

### Running

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `shelly.yaml` | Path to YAML configuration file |
| `-agent` | *(entry_agent from config)* | Agent to start the session with |
| `-verbose` | `false` | Show tool results and thinking text |

### Initializing (`shelly init`)

Creates a `.shelly/` directory with a generated `config.yaml`. Without `--template`, runs the full interactive wizard. With `--template`, uses a pre-built agent structure and only prompts for providers and slot mapping.

| Flag | Default | Description |
|------|---------|-------------|
| `-shelly-dir` | `.shelly` | Path to `.shelly` directory |
| `-template` | *(none)* | Config template name, or `list` to show available templates |

```bash
# Interactive wizard (full control)
shelly init

# List available templates
shelly init --template list

# Bootstrap from a template
shelly init --template simple-assistant
shelly init --template dev-team
```

**Built-in templates:**

| Template | Description | Slots | Agents |
|----------|-------------|-------|--------|
| `simple-assistant` | Single agent with all toolboxes | `primary` | `assistant` |
| `dev-team` | Multi-agent dev workflow | `primary`, `fast` | `orchestrator`, `planner`, `coder` |

Templates define **provider slots** (e.g. "primary", "fast") that you map to your configured providers during setup. This lets the same template work with any combination of providers.

The `dev-team` template assigns per-agent **skills** (`orchestrator-workflow`, `planner-workflow`, `coder-workflow`) that define workflow protocols for task handoff, planning, and coding. These skills are loaded from `.shelly/skills/` and available on-demand via the `load_skill` tool.

## Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/quit` | Exit the chat (also `/exit`) |

Ctrl+C gracefully interrupts the current request and exits.

## Message Display

Messages are rendered using a compartmentalized, typed display system. Each message kind has its own visual treatment with emoji prefixes and distinct styling.

### Display Item Types

| Type | Visual | Description |
|------|--------|-------------|
| User message | `🧑 You >` (blue) | User input text |
| Thinking | `🤖 agent >` (gray) | Agent reasoning text with optional timing footer |
| Spinner | `⣾ 🤖 agent >` (magenta) | "Thinking..." placeholder while waiting |
| Tool call | `🔧 tool name` (bold) | Single tool invocation with result underneath |
| Tool group | `🔧 tool name` (bold) | Parallel calls of the same tool, windowed (max 4) |
| Agent answer | `🤖 agent >` (cyan) | Final answer rendered as markdown |
| Sub-agent | `🦾 sub-agent` (magenta) | Sub-agent container, windowed (max 4 items) |

### Agent Prefix

Each agent can have a configurable display prefix (set via `prefix:` in YAML config). The default is `"🤖"`. Examples: `"📝"` for a planner, `"🦾"` for a worker. The prefix appears in thinking messages, answers, and agent containers.

### Example Output

```
🧑 You > What's the weather in Berlin?

  🤖 assistant > I need to search for current weather data.
  🔧 Searching for "weather Berlin today"
  └ {"temp": 18, "condition": "partly cloudy", ...}
  🔧 Calling parse_result {"format": "summary"}
  └ 18°C, partly cloudy
  🤖 assistant > 0.8s

🤖 assistant > The current weather in Berlin is 18°C and partly cloudy.
```

### Sub-Agent Display

When agents delegate to sub-agents, the TUI displays sub-agent activity via events from the engine. Sub-agent containers show the child agent's prefix and are windowed to show at most 4 items at a time.

### Tool Groups

When an agent makes multiple parallel calls to the same tool in one message, they are grouped together with windowing (max 4 visible). Older calls scroll out as new ones arrive.

### Tool Results

Tool results appear underneath their call with a `└` prefix. Errors are highlighted in red, successful results in dim gray.

## Usage Metrics

After each agent response, usage metrics are displayed in dim text:

```
 5.3k total, 1.5k tokens last message, 2.3s total time
```

- `total`: Cumulative tokens across the entire session
- `tokens last message`: Tokens used in the most recent exchange
- `total time`: Duration of the agent's reasoning and response generation

## Architecture

```
cmd/shelly/
├── main.go              Entry point, flag parsing, subcommand dispatch
├── model.go             Root bubbletea model, state machine, event dispatch
├── chatview.go          Chat view with agent containers, message routing
├── agentcontainer.go    Agent container with windowing, spinner animation, tool grouping
├── displayitems.go      Display item interface + concrete types (thinking, tool, spinner, sub-agent)
├── styles.go            Centralized lipgloss style definitions
├── bridge.go            Engine event → bubbletea message bridge
├── messages.go          Bubbletea message types
├── statusbar.go         Token usage and timing display
├── helpers.go           Shared utilities (markdown, truncation, formatting)
├── toolformat.go        Human-readable tool call formatting
├── initwizard.go        Interactive config wizard (providers, agents, YAML marshaling)
├── templates.go         Config templates, registry, slot-mapping wizard, template application
├── templates_test.go    Template unit tests
├── configeditor.go      Interactive config editor for existing configs
├── README.md            This file
```

### Flow

1. `main()` parses flags and dispatches subcommands (`init`, `config`) or delegates to `run()`
2. `runInit()` branches on `--template`: `list` prints templates, a name runs `runTemplateWizard()`, no template runs the full `runWizard()`
3. `run()` loads the YAML config, creates the engine and session, starts the bubbletea TUI
4. `bridge.go` starts two goroutines: one watches engine events (agent start/end, ask user), one watches chat messages via `Wait()/Since()`
5. `model.go` routes bubbletea messages to the chat view, which manages agent containers
6. `chatview.go` creates agent containers on `agentStartMsg`, routes assistant/tool messages to the active container, and collapses containers on `agentEndMsg`
7. `agentcontainer.go` manages display items (thinking, tool calls, tool groups) with windowing and spinner animation

### Message Routing

| Role | Rendering |
|------|-----------|
| `system` | Hidden |
| `user` | Hidden (already printed at submit time) |
| `assistant` with tool calls | Routed to agent container: thinking items + tool call items (grouped for parallel calls) |
| `assistant` text-only | Final answer printed to scrollback with agent prefix |
| `tool` | Tool result matched to pending call in agent container |

## Configuration

See [engine README](../../pkg/engine/README.md) for the full YAML configuration reference. Minimal example:

```yaml
providers:
  - name: default
    kind: anthropic
    api_key: sk-xxx
    model: claude-sonnet-4-20250514

agents:
  - name: assistant
    description: A helpful assistant
    provider: default

entry_agent: assistant
```
