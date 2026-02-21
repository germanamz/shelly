# state

Thread-safe key-value store for inter-agent structured data sharing (blackboard pattern). Agents write structured findings; others read or watch for them. The store can expose its operations as `toolbox.Tool` entries so agents interact with shared state through their normal tool-calling loop.

## Architecture

```
state/
├── store.go        Store type, Watch, Tools integration
└── store_test.go   Tests including concurrency and tool integration
```

### Store

```go
s := &state.Store{} // zero value is ready to use

s.Set("key", value)
v, ok := s.Get("key")
s.Delete("key")
keys := s.Keys()         // sorted
snap := s.Snapshot()      // shallow copy
v, err := s.Watch(ctx, "key") // blocks until key exists or ctx done
```

All methods are safe for concurrent use (sync.RWMutex internally).

### Watch

`Watch(ctx, key)` blocks until the key exists in the store or the context is cancelled. It uses a signal channel pattern (close + recreate on Set/Delete) identical to `chat.Chat.Wait`.

### Tool integration

```go
tb := s.Tools("myns")
// Registers 3 tools: myns_state_get, myns_state_set, myns_state_list
```

| Tool | Input | Output |
|---|---|---|
| `{ns}_state_get` | `{"key":"..."}` | JSON-encoded value or error |
| `{ns}_state_set` | `{"key":"...","value":...}` | `"ok"` |
| `{ns}_state_list` | `{}` | JSON array of keys |

Values stored via tools are `json.RawMessage`, keeping them in their original JSON form for easy agent consumption.

## Examples

### Agent sharing findings

```go
store := &state.Store{}

// Agent A writes
store.Set("analysis", json.RawMessage(`{"sentiment":"positive"}`))

// Agent B reads
v, ok := store.Get("analysis")

// Agent C waits for result
v, err := store.Watch(ctx, "analysis")
```

### Exposing state to agent tools

```go
store := &state.Store{}
tb := store.Tools("shared")

base := agents.NewAgentBase("bot", adapter, c, tb)
// Agent can now call shared_state_get, shared_state_set, shared_state_list
```
