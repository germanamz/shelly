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

## Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/quit` | Exit the chat (also `/exit`) |

Ctrl+C gracefully interrupts the current request and exits.

## Reasoning Chain

While the agent processes a request, intermediate reasoning steps are streamed to the terminal in real time using the chat's `Wait`/`Since` API:

**Default mode** shows tool invocations with arguments:

```
you> What's the weather in Berlin?
  [calling web_search] {"query": "weather Berlin today"}
  [calling parse_result] {"format": "summary"}

assistant> The current weather in Berlin is 18°C and partly cloudy.
```

**Verbose mode** (`-verbose`) additionally shows tool results and thinking text:

```
you> What's the weather in Berlin?
  [thinking] I need to search for current weather data.
  [calling web_search] {"query": "weather Berlin today"}
  [result] {"temp": 18, "condition": "partly cloudy", ...}
  [calling parse_result] {"format": "summary"}
  [result] 18°C, partly cloudy

assistant> The current weather in Berlin is 18°C and partly cloudy.
```

Errors from tool execution are highlighted in red:

```
  [error] connection timeout: api.weather.com
```

## Usage Metrics

After each agent response, usage metrics are displayed in dim text:

```
  [last: ↑1.2k ↓0.3k · total: ↑5.3k ↓2.1k · limit: 200k · 2.3s]
```

- `last`: Tokens used in the most recent LLM call (input ↑, output ↓)
- `total`: Cumulative tokens across the entire session
- `limit`: Maximum context window for the model
- Time: Total duration of the agent's reasoning and response generation

## Architecture

```
cmd/shelly/
├── main.go              Entry point, flag parsing, subcommand dispatch
├── initwizard.go        Interactive config wizard (providers, agents, YAML marshaling)
├── templates.go         Config templates, registry, slot-mapping wizard, template application
├── templates_test.go    Template unit tests
├── configeditor.go      Interactive config editor for existing configs
├── README.md            This file
```

### Flow

1. `main()` parses flags and dispatches subcommands (`init`, `config`) or delegates to `run()`
2. `runInit()` branches on `--template`: `list` prints templates, a name runs `runTemplateWizard()`, no template runs the full `runWizard()`
3. `run()` loads the YAML config, creates the engine and session, prints the banner, and enters the chat loop
4. The chat loop reads stdin, dispatches commands, and calls `sendAndStream()` for user messages
5. `sendAndStream()` records the chat cursor, starts a background goroutine running `streamChat()`, calls `session.Send()`, then cancels the watcher
6. `streamChat()` uses `chat.Wait()/Since()` to observe new messages as the agent's ReAct loop appends them

### Message Display

| Role | Default | Verbose |
|------|---------|---------|
| `system` | Hidden | Hidden |
| `user` | Hidden (already shown at prompt) | Hidden |
| `assistant` with tool calls | Tool names + arguments | + thinking text |
| `assistant` text-only | Final answer | Final answer |
| `tool` | Hidden | Result or error content |

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
