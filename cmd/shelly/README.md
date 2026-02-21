# shelly

Interactive terminal chat for the Shelly agent framework. Connects to the engine, creates a session with the configured entry agent, and runs a read-eval-print loop with real-time reasoning chain visibility.

## Usage

```bash
shelly [-config path] [-agent name] [-verbose]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `shelly.yaml` | Path to YAML configuration file |
| `-agent` | *(entry_agent from config)* | Agent to start the session with |
| `-verbose` | `false` | Show tool arguments, results, and thinking text |

## Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/quit` | Exit the chat (also `/exit`) |

Ctrl+C gracefully interrupts the current request and exits.

## Reasoning Chain

While the agent processes a request, intermediate reasoning steps are streamed to the terminal in real time using the chat's `Wait`/`Since` API:

**Default mode** shows tool invocations:

```
you> What's the weather in Berlin?
  [calling web_search]
  [calling parse_result]

assistant> The current weather in Berlin is 18°C and partly cloudy.
```

**Verbose mode** (`-verbose`) additionally shows tool arguments, results, and thinking text:

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

## Architecture

```
cmd/shelly/
├── shelly.go       Main entry point, chat loop, reasoning chain streaming
├── shelly_test.go  Unit tests
├── README.md       This file
```

### Flow

1. `main()` parses flags and delegates to `run()`
2. `run()` loads the YAML config, creates the engine and session, prints the banner, and enters `chatLoop()`
3. `chatLoop()` reads stdin line by line, dispatches commands, and calls `sendAndStream()` for user messages
4. `sendAndStream()` records the chat cursor, starts a background goroutine running `streamChat()`, calls `session.Send()`, then cancels the watcher
5. `streamChat()` uses `chat.Wait()/Since()` to observe new messages as the agent's ReAct loop appends them, and prints each one via `printMessage()`

### Message Display

| Role | Default | Verbose |
|------|---------|---------|
| `system` | Hidden | Hidden |
| `user` | Hidden (already shown at prompt) | Hidden |
| `assistant` with tool calls | Tool names | + arguments, thinking text |
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
