# state

Package `state` provides a thread-safe key-value store for inter-agent structured data sharing (blackboard pattern). Agents write structured findings; others read or watch for them. The store can expose its operations as `toolbox.Tool` entries so agents interact with shared state through their normal tool-calling loop.

## Architecture

```
state/
  store.go        Store type, Get/Set/Delete/Keys/Snapshot/Watch, Tools integration
  store_test.go   Tests including concurrency, deep-copy, and tool integration
```

## Exported Types

### Store

```go
type Store struct { /* unexported fields */ }
```

A thread-safe key-value store. The zero value is ready to use (no constructor needed). Internal structures (data map and signal channel) are lazily allocated on first use via `sync.Once`. Uses `sync.RWMutex` for concurrency safety and a signal channel pattern for `Watch` notifications.

## Store Methods

### Core Operations

- **`Get(key string) (any, bool)`** -- returns the value for key and whether it was found. Values of type `json.RawMessage` or `[]byte` are deep-copied to prevent callers from mutating stored data.
- **`Set(key string, value any)`** -- stores a value under key and notifies any goroutines blocked in `Watch`.
- **`Delete(key string)`** -- removes a key and notifies any goroutines blocked in `Watch`.
- **`Keys() []string`** -- returns a sorted slice of all keys in the store.
- **`Snapshot() map[string]any`** -- returns a copy of the entire store. Values of type `json.RawMessage` or `[]byte` are deep-copied to prevent aliasing.

### Blocking Watch

```go
func (s *Store) Watch(ctx context.Context, key string) (any, error)
```

Blocks until the specified key exists in the store or the context is cancelled. Uses a signal channel pattern (close + recreate on `Set`/`Delete`) for efficient notification without polling.

### Tool Integration

```go
func (s *Store) Tools(namespace string) *toolbox.ToolBox
```

Returns a `ToolBox` with three tools namespaced under the given prefix:

| Tool | Input | Output |
|------|-------|--------|
| `{ns}_state_get` | `{"key":"..."}` | JSON-encoded value or error |
| `{ns}_state_set` | `{"key":"...","value":...}` | `"ok"` |
| `{ns}_state_list` | `{}` | JSON array of sorted keys |

Values stored via tools are `json.RawMessage`, keeping them in their original JSON form for easy agent consumption.

## Usage

### Direct API

```go
store := &state.Store{} // zero value is ready to use

store.Set("key", "value")
v, ok := store.Get("key")
store.Delete("key")
keys := store.Keys()          // sorted
snap := store.Snapshot()       // shallow copy (byte slices deep-copied)
v, err := store.Watch(ctx, "key") // blocks until key exists or ctx done
```

### Agent Sharing Findings

```go
store := &state.Store{}

// Agent A writes
store.Set("analysis", json.RawMessage(`{"sentiment":"positive"}`))

// Agent B reads
v, ok := store.Get("analysis")

// Agent C waits for result
v, err := store.Watch(ctx, "analysis")
```

### Exposing State to Agent Tools

```go
store := &state.Store{}
tb := store.Tools("shared")
// Agent can now call shared_state_get, shared_state_set, shared_state_list
```

## Dependencies

- `pkg/tools/toolbox` -- for the `Tools()` method that exposes store operations as agent-callable tools.
