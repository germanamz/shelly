# /clear Command Implementation Plan

## Objective
Add `/clear` slash command to the Shelly TUI. Behavior:
- Stops the current session (removes from engine, cancels bridge).
- Clears UI state (chat view, status bar, input).
- Optionally clears shared state/tasks stores if enabled (propose adding `Engine.ClearState()` and `Engine.ClearTasks()` methods).
- Starts a fresh new `Session` using the entry agent from config.
- Restarts the event bridge.
- Preserves `.shelly/` (skills, notes, permissions, reflections).
- Prints feedback like \"Starting fresh session...\".

`/clear` resets runtime ephemeral state (chats, UI, optionally tasks/state) but leaves terminal scrollback intact (committed answers stay).

## Analysis Summary

### Session Lifecycle
- **Engine**: Top-level, loads config, creates providers/toolboxes/state/tasks/skills, manages sessions map.
  - `eng.NewSession(agentName)`: creates ID, new agent via factory, new `Session` with agent.Chat().
  - `eng.RemoveSession(id)`: removes from map.
  - `eng.State()` / `eng.Tasks()`: return stores (shared across sessions, in-memory maps).
- **Session**: owns agent + chat. `sess.Send(text)`: appends user msg, runs agent synchronously, publishes events.
- **Chat**: per-agent, append-only messages (user/assistant/tool).
- **State/Tasks**: engine.store (*state.Store), engine.taskStore (*tasks.Store). In-memory, no persistence. Handlers capture store at engine init, so resetting requires recreating stores + toolboxes (complex).

### Slash Commands
- Handled in `cmd/shelly/model.go` `handleSubmit(inputSubmitMsg)`:
  ```go
  if text == \"/help\" { show help }
  if text == \"/quit\" || \"/exit\" { quit }
  else { sess.Send(text) }
  ```
- `/help` prints command list to status/help view.

### TUI Structure (Bubble Tea)
- `appModel`: root model with `sess *engine.Session`, `events *engine.EventBus`, `chatView *chatViewModel`, `inputBox inputModel`, `statusBar statusModel`, `cancelBridge context.CancelFunc`.
- `Init()`: `startBridge(ctx, sess.Chat(), events)` (watches chat/events → bubbletea msgs).
- `handleSubmit()`: processes input on Enter.
- Committed content: `tea.Println()` to scrollback (can't clear).
- Live view: `chatView` manages `agentContainer`s (windowed display items: thinking, tools, sub-agents).

## Proposed Changes

### 1. Add Engine to appModel (access NewSession/RemoveSession)
**File**: `cmd/shelly/model.go`
- Add field: `eng *engine.Engine`
- Update `newAppModel`: `func newAppModel(ctx context.Context, sess *engine.Session, events *engine.EventBus, eng *engine.Engine, verbose bool) appModel`
  - `m.eng = eng`
- Update `appModel.Init()`: pass `m.eng` if needed (no).

**File**: `cmd/shelly/main.go`
- In `run()`: `model := newAppModel(ctx, sess, eng.Events(), eng, verbose)`

### 2. Add Engine.ClearState/ClearTasks (Optional but Recommended)
**File**: `pkg/engine/engine.go`
- Add methods:
  ```go
  func (e *Engine) ClearState() {
  	if e.store != nil {
  		e.store = &state.Store{}
  		if tb, ok := e.toolboxes[\"state\"]; ok {
  			e.toolboxes[\"state\"] = e.store.Tools(\"shared\")
  		}
  	}
  }
  func (e *Engine) ClearTasks() {
  	if e.taskStore != nil {
  		e.taskStore = &tasks.Store{}
  		if tb, ok := e.toolboxes[\"tasks\"]; ok {
  			e.toolboxes[\"tasks\"] = e.taskStore.Tools(\"shared\")
  		}
  	}
  }
  ```
- Recreates empty stores + toolboxes. New agents will use new factories? No, registry factories capture old tbs.
  - **Issue**: Factory closures in `registerAgent` capture `e.toolboxes[name]` at init time.
  - **Fix**: To fully reset, recreate agents factories post-clear (complex, reload config).
  - **Recommendation**: Skip store clear for v1 (state/tasks intentionally shared). New session has fresh chat/agent.

### 3. Implement /clear in handleSubmit
**File**: `cmd/shelly/model.go`
- In `handleSubmit(text string)`:
  ```go
  case \"/clear\":
  	// Feedback
  	m.chatView.AddItem(&displayitems.Thinking{Content: \"Starting fresh session...\", Agent: \"system\", Timestamp: time.Now()})
  	m.statusBar.SetMessage(\"Clearing session and starting fresh...\")
  	
  	// Stop old
  	if m.cancelBridge != nil {
  		m.cancelBridge()
  	}
  	m.eng.RemoveSession(m.sess.ID())
  	m.inputBox.Reset()
  	m.chatView.Clear() // Add Clear() method if needed: m.agents = {}, m.subAgents = {}
  	
  	// Optionally: m.eng.ClearState(); m.eng.ClearTasks()
  	
  	// New session
  	newSess, err := m.eng.NewSession(\"\") // entry agent
  	if err != nil {
  		// handle error, add error item
  		return m, nil
  	}
  	m.sess = newSess
  	m.statusBar = newStatusBar(m.sess.Completer()) // if uses Completer
  	
  	// Restart bridge
  	m.startBridge()
  	
  	m.state = stateIdle
  	return m, nil
  ```
- Add `/clear` to `/help` text:
  ```
  /clear         Close current session, clear UI, start fresh session
  ```

### 4. Add chatView.Clear() (Reset Live View)
**File**: `cmd/shelly/chatview.go`
- Add method:
  ```go
  func (m *chatViewModel) Clear() {
  	m.agents = make(map[string]*agentContainer)
  	m.subAgents = make(map[string]*subAgentMessage)
  	// reset other state
  }
  ```

### 5. Update README
**File**: `cmd/shelly/README.md`
- Add to Commands table: `/clear | Close session, reset UI, start fresh`

## Edge Cases
- Config without entry_agent: uses first agent.
- Error creating new session: show error item, keep old.
- No state/tasks: noop.
- MCP clients/skills: preserved (engine-level).
- Terminal scrollback: uncleared (intentional, chat history).

## Tests
- Add `cmd/shelly/model_test.go`: mock engine/sess, test /clear transitions state, resets views.
- Integration: manual `task run`, type `/clear`, verify new session (new agent start event).

## Follow-up
- If full state/tasks reset needed: implement Engine.ReloadConfig() to recreate factories/toolboxes.
- Enhance: `/clear --hard` to restart engine (load config again).