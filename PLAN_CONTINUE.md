# Session Persistence — Phased Implementation Plan

Based on [CONTINUE.md](CONTINUE.md). Detailed implementation phases with specific code changes.

---

## Phase 1: Foundation — `pkg/sessions/` package ✅ COMPLETED

**Goal**: Build the standalone sessions package with serialization and file store. No integration with anything else yet. Fully testable in isolation.

**Status**: Completed. All files created, 13 tests passing, lint clean.

### Step 1.1: `pkg/sessions/serialize.go` — Message JSON serialization

Create a discriminated-union JSON envelope for `content.Part` (which is an interface):

```go
type jsonPart struct {
    Kind       string            `json:"kind"`                 // text | image | tool_call | tool_result
    Text       string            `json:"text,omitempty"`
    URL        string            `json:"url,omitempty"`
    Data       []byte            `json:"data,omitempty"`
    MediaType  string            `json:"media_type,omitempty"`
    ID         string            `json:"id,omitempty"`
    Name       string            `json:"name,omitempty"`
    Arguments  string            `json:"arguments,omitempty"`
    Metadata   map[string]string `json:"metadata,omitempty"`
    ToolCallID string            `json:"tool_call_id,omitempty"`
    Content    string            `json:"content,omitempty"`
    IsError    bool              `json:"is_error,omitempty"`
}

type jsonMessage struct {
    Sender   string         `json:"sender"`
    Role     string         `json:"role"`
    Parts    []jsonPart     `json:"parts"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

Public API:
- `MarshalMessages([]message.Message) ([]byte, error)`
- `UnmarshalMessages([]byte) ([]message.Message, error)`

Key decisions:
- Map each `content.Part` concrete type to a `kind` string
- Use `json.Marshal`/`json.Unmarshal` with the envelope types
- Skip unknown part kinds gracefully (log warning, continue)
- Message `Metadata` is `map[string]any` — `encoding/json` handles this natively

### Step 1.2: `pkg/sessions/store.go` — File-based session store

Types (as specified in CONTINUE.md):
```go
type SessionInfo struct {
    ID        string       `json:"id"`
    Agent     string       `json:"agent"`
    Provider  ProviderMeta `json:"provider"`
    CreatedAt time.Time    `json:"created_at"`
    UpdatedAt time.Time    `json:"updated_at"`
    Preview   string       `json:"preview"`
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

type Store struct { dir string }
```

Methods:
- `New(dir string) *Store`
- `Save(info SessionInfo, msgs []message.Message) error` — writes `{dir}/{id}.json` atomically (write to temp file, rename)
- `Load(id string) (SessionInfo, []message.Message, error)` — reads and unmarshals
- `List() ([]SessionInfo, error)` — glob `*.json`, unmarshal each file, sort by `UpdatedAt` desc
- `Delete(id string) error`

Key decisions:
- Atomic writes via `os.CreateTemp` + `os.Rename` to prevent corruption
- `List()` reads all files and unmarshals full content. Start simple — optimize later if needed.
- One file per session, metadata and messages colocated.

### Step 1.3: `pkg/sessions/serialize_test.go` + `store_test.go`

- Round-trip test: create messages with all 4 part types -> marshal -> unmarshal -> assert equal
- Edge cases: empty messages, empty parts, nil metadata
- Store tests: Save -> Load round trip, List ordering, Delete removes file
- Use `t.TempDir()` for test isolation

### Step 1.4: `pkg/sessions/README.md`

Package documentation per project conventions.

**Deliverable**: A fully tested, standalone package with no dependencies beyond `pkg/chats/`.

---

## Phase 2: Infrastructure — `shellydir` + `engine` plumbing ✅ COMPLETED

**Goal**: Wire sessions into the engine layer. Sessions auto-save after each send. No TUI changes yet.

**Status**: Completed. `SessionsDir()` added to shellydir, sessions dir created in `EnsureStructure()`, `persistID`/`createdAt`/`onSendComplete` added to Session, auto-save wired in Engine, `ResumeSession()` implemented. 3 new tests passing, all 1111 tests passing, lint clean.

### Step 2.1: `pkg/shellydir/shellydir.go` — Add `SessionsDir()`

Add method to `Dir`:
```go
func (d Dir) SessionsDir() string {
    return filepath.Join(d.Root(), "local", "sessions")
}
```

### Step 2.2: `pkg/shellydir/init.go` — Create sessions dir

Add to `EnsureStructure()`:
```go
os.MkdirAll(d.SessionsDir(), 0o750)
```

### Step 2.3: `pkg/engine/session.go` — Add fields and callback

Add to `Session` struct:
```go
persistID      string       // UUID for persistence (distinct from engine's "session-N" ID)
createdAt      time.Time
onSendComplete func()       // called after successful Send/Compact
```

- Set `createdAt` in construction
- Generate `persistID` via `crypto/rand` hex (8 bytes = 16 hex chars) at construction time
- Call `onSendComplete()` at end of successful `SendParts()` and `Compact()` (guarded by nil check)
- Add public accessors: `PersistID() string`, `CreatedAt() time.Time`

### Step 2.4: `pkg/engine/engine.go` — Add session store and auto-save

New field:
```go
sessionStore *sessions.Store
```

Initialize in `New()`:
```go
e.sessionStore = sessions.New(cfg.Dir.SessionsDir())
```

New public method:
```go
func (e *Engine) SessionStore() *sessions.Store { return e.sessionStore }
```

Wire `onSendComplete` callback when creating sessions (both `NewSession` and `ResumeSession`):
```go
s.onSendComplete = func() {
    _ = e.saveSession(s)
}
```

New private method `saveSession(s *Session)`:
1. Build `SessionInfo` from session fields:
   - `ID` = `s.PersistID()`
   - `Agent` = `s.AgentName()`
   - `Provider` = `ProviderMeta{Kind: s.ProviderInfo().Kind, Model: s.ProviderInfo().Model}`
   - `CreatedAt` = `s.CreatedAt()`
   - `UpdatedAt` = `time.Now()`
   - `Preview` = first ~100 chars of last user message's text content
   - `MsgCount` = `s.Chat().Len()`
2. Call `e.sessionStore.Save(info, s.Chat().Messages())`

### Step 2.5: `pkg/engine/engine.go` — Add `ResumeSession`

```go
func (e *Engine) ResumeSession(persistID string) (*Session, error)
```

1. `info, msgs, err := e.sessionStore.Load(persistID)`
2. Look up agent factory: `factory, ok := e.registry.Get(info.Agent)`
3. Create agent: `a := factory()` -> `a.SetRegistry(e.registry)` -> `a.Init()`
4. Agent already has a fresh system prompt in its chat (from Init). Take `msgs[1:]` (skip persisted system prompt)
5. Append persisted non-system messages to agent's chat via `a.Chat().Append(msgs[1:]...)`
6. Build `Session` with:
   - `persistID` = `info.ID` (reuse same ID so subsequent saves overwrite)
   - `createdAt` = `info.CreatedAt`
   - New engine-internal `id` = `session-{nextID}` (as usual)
7. Wire `onSendComplete` callback
8. Register in `e.sessions`
9. Return session

Key decision: The `persistID` is reused so the same session file gets updated on subsequent saves. The engine's internal `session-N` ID is separate and ephemeral.

**Deliverable**: Engine auto-saves sessions to disk. `ResumeSession` can reload them. Testable via engine integration tests.

---

## Phase 3: TUI — Session picker and `/sessions` command

**Goal**: Users can browse and resume sessions via the TUI.

### Step 3.1: `cmd/shelly/internal/msgs/msgs.go` — New message types

```go
type SessionPickerActivateMsg struct{ Sessions []sessions.SessionInfo }
type SessionPickerDismissMsg struct{}
type SessionPickerSelectionMsg struct{ ID string }
```

### Step 3.2: `cmd/shelly/internal/input/sessionpicker.go` — Session picker component

Model similar to `CmdPickerModel`:
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

Behavior:
- Shows scrollable list of sessions
- Each entry: preview text (truncated), agent name, relative timestamp, message count
- Filter by typing (substring match against preview)
- Enter -> `SessionPickerSelectionMsg{ID}`
- Esc -> `SessionPickerDismissMsg`
- Rendering: similar style to cmdpicker popup

Messages it handles:
- `SessionPickerActivateMsg` -> populate and activate
- `SessionPickerDismissMsg` -> deactivate
- `SessionPickerQueryMsg{Query}` -> filter
- Key presses -> navigate/select/dismiss

### Step 3.3: `cmd/shelly/internal/input/cmdpicker.go` — Add `/sessions` command

Add to `AvailableCommands`:
```go
{Name: "/sessions", Desc: "Browse and resume previous sessions"},
```

### Step 3.4: `cmd/shelly/internal/app/commands.go` — Add `/sessions` dispatch

```go
case "/sessions":
    return commandResult{cmd: m.executeSessions(), handled: true}
```

`executeSessions()`:
1. Load list: `sessions, err := m.eng.SessionStore().List()`
2. If empty -> append "No previous sessions" note to chatview, return nil
3. Return `tea.Cmd` that sends `SessionPickerActivateMsg{Sessions: sessions}`

### Step 3.5: `cmd/shelly/internal/app/app.go` — Session picker integration

New field on `AppModel`:
```go
sessionPicker input.SessionPickerModel
```

Update routing in `Update()`:
- `msgs.SessionPickerActivateMsg` -> activate picker
- `msgs.SessionPickerDismissMsg` -> deactivate picker
- `msgs.SessionPickerSelectionMsg` -> call `executeResumeSession(msg.ID)`

Update `View()`:
- Render session picker as overlay when active (similar to config wizard overlay)

Update `handleKey()`:
- If session picker active, route keys to it first (Esc dismisses)

`executeResumeSession(id string)`:
1. Cancel current send if active (`m.cancelSend()`)
2. Cancel bridge (`m.cancelBridge()`)
3. Remove current session (`m.eng.RemoveSession(m.sess.ID())`)
4. Resume: `newSess, err := m.eng.ResumeSession(id)`
5. Set `m.sess = newSess`
6. Clear chatview
7. Render historical messages into chatview:
   - `role.User` -> `ChatViewCommitUserMsg{Text: textContent}`
   - `role.Assistant` (no tool calls) -> commit as assistant reply
   - Skip `role.System`, `role.Tool`, and assistant messages with tool calls
8. Restart bridge (start new event subscription goroutine)
9. Show "Resumed session" note in chatview
10. Set state to `StateIdle`

**Deliverable**: Full end-to-end session persistence and resume via `/sessions` command.

---

## Phase 4: Polish and edge cases

**Goal**: Handle edge cases, improve UX.

### Step 4.1: `/clear` interaction with sessions

Current `/clear` creates a fresh session. After Phase 2, this already works correctly because:
- The old session was auto-saved after last send
- The new session gets a new `persistID`
- First save happens after first successful send

No changes needed — just verify behavior.

### Step 4.2: Session ID generation

Use `crypto/rand` to generate a short unique ID (8-byte hex = 16 chars). Avoid adding a UUID dependency if not already in `go.mod`.

### Step 4.3: Resume with different agent/provider

If the persisted agent no longer exists in config, `ResumeSession` should return a clear error. The TUI should show this to the user gracefully.

### Step 4.4: List performance optimization (deferred)

If session count grows large, `List()` becomes slow. For now, acceptable. Future: cache metadata in an index file, or only read file headers.

### Step 4.5: Token count display on resume

After resuming, the token count in the status bar should reflect the loaded session. This happens naturally if the bridge recalculates from the completer's usage tracker. Verify this works.

---

## Implementation Order Summary

| Phase | Steps | Files Changed/Created | Dependencies |
|-------|-------|-----------------------|-------------|
| **1** | 1.1-1.4 | `pkg/sessions/serialize.go`, `store.go`, `*_test.go`, `README.md` | None (standalone) |
| **2** | 2.1-2.5 | `pkg/shellydir/shellydir.go`, `init.go`, `pkg/engine/session.go`, `engine.go` | Phase 1 |
| **3** | 3.1-3.5 | `cmd/shelly/internal/msgs/msgs.go`, `input/sessionpicker.go`, `input/cmdpicker.go`, `app/commands.go`, `app/app.go` | Phase 2 |
| **4** | 4.1-4.5 | Verification + minor fixes across existing files | Phase 3 |

Each phase is independently testable and shippable. Phase 1 can be reviewed/merged without any behavior change. Phase 2 adds invisible auto-save. Phase 3 adds the user-facing feature. Phase 4 hardens.
