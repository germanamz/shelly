# sessions

File-based session persistence for Shelly conversations. Provides JSON serialization of chat messages and a directory-per-session file store for saving, loading, listing, and deleting sessions.

## Architecture

```
sessions/
├── serialize.go      Message JSON serialization (discriminated-union envelope)
├── store.go          Directory-per-session store with atomic writes
├── serialize_test.go
└── store_test.go
```

## Storage Layout (v2)

Each session is stored in its own directory:

```
{sessionsDir}/
  {id}/
    meta.json       SessionInfo metadata (~500 bytes)
    messages.json   Message array (serialized parts)
```

This layout separates metadata from messages so `List()` only reads small `meta.json` files.

### V1 Migration

Legacy single-file sessions (`{id}.json`) are supported transparently:
- `Load()` falls back to reading the v1 file if no v2 directory exists
- `List()` picks up both v2 directories and v1 `.json` files
- `Save()` writes v2 format and removes any leftover v1 file
- `Delete()` handles both layouts

## Serialization

Messages are serialized using a discriminated-union JSON envelope. Each `content.Part` is mapped to a `kind` string (`text`, `image`, `tool_call`, `tool_result`). Unknown part kinds are skipped gracefully with a log warning.

**Public API:**

- `MarshalMessages([]message.Message) ([]byte, error)` -- serializes messages to JSON
- `UnmarshalMessages([]byte) ([]message.Message, error)` -- deserializes JSON to messages

## Store

The `Store` manages session directories under a root directory.

**Types:**

- `SessionInfo` -- metadata about a persisted session (ID, agent, provider, timestamps, preview, message count)
- `ProviderMeta` -- provider kind and model
- `Store` -- directory-per-session store

**Public API:**

- `New(dir string) *Store` -- creates a store for the given directory
- `Save(info SessionInfo, msgs []message.Message) error` -- writes atomically (temp file + rename) to `{id}/meta.json` and `{id}/messages.json`
- `Load(id string) (SessionInfo, []message.Message, error)` -- reads and deserializes (v2 or v1 fallback)
- `List() ([]SessionInfo, error)` -- returns all sessions sorted by UpdatedAt descending (reads only metadata)
- `Delete(id string) error` -- removes the session directory (or v1 file)

## Dependencies

Depends only on `pkg/chats/` (content, message, role) and the Go standard library.
