# Session Persistence — Implementation Plan

## Goal
Allow users to save and resume previous conversations. Sessions are persisted to disk and can be browsed/selected via a `/sessions` command in the TUI.

---

## Architecture Overview

```
.shelly/local/sessions/
  ├── {uuid}.json          # One file per session
  └── ...
```

Each session file contains metadata + the full message history as JSON. The system prompt (index 0) is **not persisted** — it's rebuilt fresh on resume so config changes take effect.

---

## New Package: `pkg/sessions/`

### `serialize.go` — Message JSON serialization
- `content.Part` is an interface, so we need a discriminated-union JSON envelope (`jsonPart`) with a `kind` field
- Supported kinds: `text`, `image`, `tool_call`, `tool_result`
- `MarshalMessages([]message.Message) ([]byte, error)` — encode messages to JSON
- `UnmarshalMessages([]byte) ([]message.Message, error)` — decode JSON to messages

### `store.go` — File-based session store
```go
type SessionInfo struct {
    ID        string       `json:"id"`
    Agent     string       `json:"agent"`
    Provider  ProviderMeta `json:"provider"`
    CreatedAt time.Time    `json:"created_at"`
    UpdatedAt time.Time    `json:"updated_at"`
    Preview   string       `json:"preview"`   // First ~100 chars of last user message
    MsgCount  int          `json:"msg_count"`
}

type ProviderMeta struct {
    Kind  string `json:"kind"`
    Model string `json:"model"`
}

type PersistedSession struct {
    SessionInfo
    Messages []message.Message `json:"-"` // Serialized separately via MarshalMessages
}
```

**Store methods:**
- `New(dir string) *Store` — creates store rooted at a directory
- `Save(info SessionInfo, msgs []message.Message) error` — write session to `{id}.json`
- `Load(id string) (SessionInfo, []message.Message, error)` — read session from disk
- `List() ([]SessionInfo, error)` — list all sessions (sorted by UpdatedAt desc)
- `Delete(id string) error` — remove a session file

**File format** (single JSON file per session):
```json
{
  "id": "abc123",
  "agent": "assistant",
  "provider": {"kind": "anthropic", "model": "claude-sonnet-4"},
  "created_at": "2026-03-05T14:30:00Z",
  "updated_at": "2026-03-05T15:45:00Z",
  "preview": "Help me refactor the auth module",
  "msg_count": 47,
  "messages": [...]
}
```

### `README.md` — Package documentation

---

## Modified Package: `pkg/shellydir/`

### `shellydir.go`
- Add `SessionsDir() string` — returns `filepath.Join(d.root, "local", "sessions")`

### `init.go`
- Add `os.MkdirAll(d.SessionsDir(), 0o750)` to `EnsureStructure()` so the directory is auto-created

---

## Modified Package: `pkg/engine/`

### `engine.go` — Add `ResumeSession` and auto-save

#### New method: `ResumeSession(id string) (*Session, error)`
1. Load session from store: `info, msgs, err := e.sessionStore.Load(id)`
2. Look up agent factory: `factory, ok := e.registry.Get(info.Agent)`
3. Create agent: `a := factory()` → `a.SetRegistry(e.registry)` → `a.Init()`
4. Skip the system prompt from persisted messages (index 0), take `msgs[1:]`
5. Append persisted non-system messages to agent's fresh chat
6. Create Session, register in engine
7. Return session

#### New field: `sessionStore *sessions.Store`
- Initialized in `New()` using `sessions.New(dir.SessionsDir())`

#### New method: `SessionStore() *sessions.Store`
- Exposes store so TUI can list sessions

#### Auto-save hook
- In `Session.SendParts()`, after successful `agent.Run()`, save the session:
  ```go
  e.sessionStore.Save(sessionInfo, chat.Messages())
  ```
- Also save after `Compact()` completes
- The session needs access to the store — either pass it during construction or have Engine do the save

**Decision**: Engine owns the save. Add a `postSend` callback to Session that Engine sets when creating it. This avoids Session importing the sessions package.

```go
// In session.go
type Session struct {
    ...
    onSendComplete func() // Called after successful Send/Compact
}
```

Engine sets this callback:
```go
s.onSendComplete = func() {
    _ = e.saveSession(s)
}
```

#### New helper: `saveSession(s *Session)`
- Builds SessionInfo from session metadata
- Gets messages via `s.Chat().Messages()`
- Extracts preview from the last user message
- Calls `e.sessionStore.Save(info, msgs)`

### `session.go` — Minor additions
- Add `createdAt time.Time` field (set at creation time)
- Add `onSendComplete func()` field
- Call `onSendComplete()` at end of successful `SendParts()` and `Compact()`

---

## Modified: TUI (`cmd/shelly/`)

### New file: `cmd/shelly/internal/input/sessionpicker.go`

A picker component similar to `cmdpicker.go` and `filepicker.go`:

```go
type SessionPickerModel struct {
    Active   bool
    sessions []sessions.SessionInfo
    filtered []sessions.SessionInfo
    query    string
    cursor   int
    maxShow  int
    Width    int
}
```

**Behavior:**
- Shows a scrollable list of previous sessions
- Each entry shows: preview text, agent name, relative timestamp, message count
- Filter by typing (matches against preview)
- Enter selects → sends `SessionPickerSelectionMsg{ID: "..."}`
- Esc dismisses

### New messages in `cmd/shelly/internal/msgs/msgs.go`

```go
type SessionPickerActivateMsg struct{}
type SessionPickerDismissMsg struct{}
type SessionPickerSelectionMsg struct{ ID string }
```

### Modified: `cmd/shelly/internal/input/cmdpicker.go`
- Add `/sessions` to `AvailableCommands`

### Modified: `cmd/shelly/internal/app/commands.go`

#### New command: `/sessions`
```go
case "/sessions":
    return commandResult{cmd: m.executeSessions(), handled: true}
```

#### `executeSessions()`:
1. Load session list from `eng.SessionStore().List()`
2. If empty, show "No previous sessions" message
3. Otherwise, activate the session picker overlay

#### `executeResumeSession(id string)`:
1. Cancel current send if active
2. Cancel bridge
3. Remove current session
4. Call `eng.ResumeSession(id)` → get new session
5. Render historical messages into chatview
6. Restart bridge
7. Show "Resumed session" note

### Modified: `cmd/shelly/internal/app/app.go`

#### New field: `sessionPicker *sessionpicker.Model` (or inline in AppModel)

#### Update handling:
- `SessionPickerSelectionMsg` → call `executeResumeSession(msg.ID)`
- Render session picker as overlay (similar to config wizard)

### History rendering on resume
When resuming, iterate through persisted messages and render them in the chatview:
- `role.User` → `ChatViewCommitUserMsg{Text: textContent}`
- `role.Assistant` with no tool calls → commit as assistant reply (rendered markdown)
- Skip `role.System`, `role.Tool`, and assistant messages with tool calls (intermediate reasoning)

This gives the user a clean view of the conversation history.

---

## Modified: `cmd/shelly/main.go`

### Pass `SessionsDir` to AppModel
- `NewAppModel` gains a `sessionsDir` parameter (or receives the store directly)
- The app needs access to session store for listing/loading

**Alternative**: Pass `eng.SessionStore()` into `NewAppModel`.

---

## Data Flow Summary

### Save flow
```
Session.SendParts() completes successfully
  → Session.onSendComplete()
    → Engine.saveSession(s)
      → sessions.Store.Save(info, messages)
        → Write JSON to .shelly/local/sessions/{id}.json
```

### Resume flow
```
User types /sessions
  → TUI loads session list from Engine.SessionStore().List()
  → Session picker overlay shown
  → User selects session
  → Engine.ResumeSession(id)
    → sessions.Store.Load(id) → info + messages
    → Create agent via factory
    → Populate chat with persisted messages (fresh system prompt + old messages)
  → TUI renders history in chatview
  → Bridge starts watching from current cursor
  → User can continue chatting
```

---

## Edge Cases & Considerations

1. **System prompt evolution**: Persisted sessions use a fresh system prompt on resume, so config/context changes apply automatically.

2. **Session ID**: Use UUID (or `time.Now().UnixNano()` formatted) to avoid collisions across restarts. The engine's internal `session-N` IDs are separate from the persisted IDs.

3. **Concurrent access**: Only one Shelly instance should write to a session file at a time. Since sessions are single-user, this is safe.

4. **Large sessions**: A session with many tool calls can get large. For now, persist everything. Future optimization: prune tool results or compress.

5. **`/clear` behavior**: Should `/clear` save the current session before clearing? Yes — the session is auto-saved after each send, so the latest state is already on disk. `/clear` creates a fresh session (which gets a new ID and starts saving on first send).

6. **Delete old sessions**: Not in v1, but the store has a `Delete` method for future use.

7. **First save timing**: A session is first saved after the first successful `SendParts()`. Empty sessions (no user messages yet) are never persisted.

---

## File Change Summary

| File | Change |
|------|--------|
| `pkg/sessions/serialize.go` | **NEW** — Message JSON serialization |
| `pkg/sessions/store.go` | **NEW** — File-based session store |
| `pkg/sessions/serialize_test.go` | **NEW** — Serialization tests |
| `pkg/sessions/store_test.go` | **NEW** — Store tests |
| `pkg/sessions/README.md` | **NEW** — Package docs |
| `pkg/shellydir/shellydir.go` | Add `SessionsDir()` |
| `pkg/shellydir/init.go` | Create sessions dir in `EnsureStructure()` |
| `pkg/engine/engine.go` | Add `sessionStore`, `ResumeSession()`, `saveSession()`, `SessionStore()` |
| `pkg/engine/session.go` | Add `createdAt`, `onSendComplete`, call after Send/Compact |
| `cmd/shelly/internal/msgs/msgs.go` | Add session picker messages |
| `cmd/shelly/internal/input/sessionpicker.go` | **NEW** — Session picker component |
| `cmd/shelly/internal/input/cmdpicker.go` | Add `/sessions` command |
| `cmd/shelly/internal/app/commands.go` | Add `/sessions` dispatch + resume logic |
| `cmd/shelly/internal/app/app.go` | Session picker overlay handling + history rendering |
| `cmd/shelly/main.go` | Pass session store to AppModel |
