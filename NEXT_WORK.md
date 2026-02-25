# Deferred Fixes

Issues identified during code review that need design decisions before implementation.

## 1. `EventFunc` Not Wired in Engine

**File:** `pkg/engine/engine.go` (~line 502)

`agent.Options.EventFunc` is never set in `registerAgent`. Events like `tool_call_start`, `tool_call_end`, and `message_added` are never published to the engine's `EventBus`, even though they are declared in `event.go` and documented.

**Needs:** Design the event-kind mapping between agent-level string kinds (`"tool_call_start"`) and engine-level `EventKind` constants. Decide whether all three events should be published or if some are internal-only.

## 2. `ReflectionDir` Not Wired in Engine

**File:** `pkg/engine/engine.go` (~line 502), `pkg/agent/agent.go` (line 74)

`agent.Options.ReflectionDir` enables failure reflection notes (`writeReflection`/`searchReflections`), but the engine never sets it. The feature is dead code when running through the engine. Child agents propagate the value (`tools.go` line 128), but the root agent never receives one.

**Needs:** Add a `reflection_dir` field to `AgentConfig` (or engine-level `Config`), wire it through `registerAgent`. Consider defaulting to `.shelly/reflections/` when the `.shelly/` directory exists.

## 3. Compaction Effect Ordering Dependency

**File:** `pkg/agent/effects/compact.go` (~line 109)

When compaction fires, it replaces the entire chat with system prompt + summary. If other effects (e.g., `LoopDetectEffect`, `ReflectionEffect`) inject messages in the same `PhaseBeforeComplete` iteration *before* compaction runs, those messages are summarized away in the same iteration they were added — the agent never sees the raw intervention.

**Needs:** Either document that compaction-style effects must run last, or snapshot `len(msgs)` before `evalEffects` and preserve messages appended by prior effects in the current iteration.

## 4. Sessions Never Removed (Memory Leak)

**File:** `pkg/engine/engine.go` (~line 266)

`sessions map[string]*Session` grows without bound. `NewSession` adds entries but there is no `RemoveSession` or `CloseSession` method. In long-running applications, sessions (with full chat history) accumulate forever.

**Needs:** Design session lifecycle ownership. Options:
- Add `Session.Close()` that removes from engine map
- Add `Engine.RemoveSession(id)` for the TUI/caller to invoke
- Add TTL-based cleanup for idle sessions

## 5. MCP Subprocess Leak

**File:** `pkg/tools/mcpclient/mcpclient.go` (~line 32)

`newFromTransport` spawns a subprocess via `mcp.CommandTransport`. If the caller abandons the `MCPClient` without calling `Close()`, the subprocess is leaked. Also, `Close()` only calls `session.Close()` without waiting for the subprocess to exit, potentially leaving zombie processes.

**Needs:** Store the `exec.Cmd` reference and call `cmd.Wait()` after closing the session, or use `context.WithCancel` to kill the child process on cleanup.

## 6. Absolute Filesystem Path Leaked to LLM Providers

**File:** `pkg/skill/store.go` (~line 76)

When a skill is loaded, its `Dir` path (absolute path on the developer's machine) is included in the content returned to the agent and sent to third-party LLM providers. This leaks machine-specific paths (usernames, project structure).

**Needs:** Emit a relative path from the working directory, or introduce a path alias that filesystem tools resolve internally.
