# sessions

File-based session persistence for Shelly conversations. Provides JSON serialization of chat messages and a file store for saving, loading, listing, and deleting sessions.

## Architecture

```
sessions/
├── serialize.go      Message JSON serialization (discriminated-union envelope)
├── store.go          File-based session store with atomic writes
├── serialize_test.go
└── store_test.go
```

## Serialization

Messages are serialized using a discriminated-union JSON envelope. Each `content.Part` is mapped to a `kind` string (`text`, `image`, `tool_call`, `tool_result`). Unknown part kinds are skipped gracefully with a log warning.

**Public API:**

- `MarshalMessages([]message.Message) ([]byte, error)` -- serializes messages to JSON
- `UnmarshalMessages([]byte) ([]message.Message, error)` -- deserializes JSON to messages

## Store

The `Store` manages session files in a directory. Each session is stored as a single JSON file (`{id}.json`) containing both metadata and messages.

**Types:**

- `SessionInfo` -- metadata about a persisted session (ID, agent, provider, timestamps, preview, message count)
- `ProviderMeta` -- provider kind and model
- `Store` -- file-based session store

**Public API:**

- `New(dir string) *Store` -- creates a store for the given directory
- `Save(info SessionInfo, msgs []message.Message) error` -- writes atomically (temp file + rename)
- `Load(id string) (SessionInfo, []message.Message, error)` -- reads and deserializes
- `List() ([]SessionInfo, error)` -- returns all sessions sorted by UpdatedAt descending
- `Delete(id string) error` -- removes a session file

## Dependencies

Depends only on `pkg/chats/` (content, message, role) and the Go standard library.
