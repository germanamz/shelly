# Refactoring Plan

Detailed phased plan for refactoring the Shelly codebase to follow Go best practices and composition patterns.

---

## Phase 1 — Eliminate Code Duplication

Low-risk changes that reduce maintenance burden without altering behavior.

### 1.1 Extract shared OpenAI-compatible provider base ✅

**Problem:** `providers/openai/` and `providers/grok/` share ~200 lines of near-identical code: wire types (`apiMessage`, `apiToolCall`, `apiFunction`, `apiChoice`, `apiUsage`), message conversion logic, response parsing, and the entire batch implementation (`uploadFile`, `downloadResults`, `parseResultsJSONL`, `convertResult`).

**Solution:** Create `providers/internal/openaicompat/` containing:
- Shared wire types (`apiMessage`, `apiToolCall`, `apiFunction`, etc.)
- `ConvertMessages(c *chat.Chat, tools []toolbox.Tool) ([]apiMessage, []apiTool)` — shared request builder
- `ParseChoice(choice apiChoice) message.Message` — shared response parser
- Shared batch infrastructure: `UploadFile`, `DownloadResults`, `ParseResultsJSONL`, `ConvertResult`

Both `openai.Adapter` and `grok.Adapter` then compose with `openaicompat` functions, keeping only provider-specific config (base URL, auth scheme, model defaults).

**Files to create:**
- `pkg/providers/internal/openaicompat/types.go`
- `pkg/providers/internal/openaicompat/convert.go`
- `pkg/providers/internal/openaicompat/batch.go`

**Files to modify:**
- `pkg/providers/openai/openai.go` — delegate to openaicompat
- `pkg/providers/openai/batch.go` — delegate to openaicompat
- `pkg/providers/grok/grok.go` — delegate to openaicompat
- `pkg/providers/grok/batch.go` — delegate to openaicompat

**Risks:** Low. Internal package, no public API change. All provider behavior tested via existing test suites.

### 1.2 Deduplicate `renderConversation` / `renderMessages` in effects

**Problem:** `pkg/agent/effects/compact.go` has `renderConversation()` and `pkg/agent/effects/sliding_window.go` has `renderMessages()` — nearly identical functions that serialize chat messages to a text format for LLM summarization.

**Solution:** Extract a shared `renderMessages(msgs []message.Message) string` in a new file `pkg/agent/effects/render.go`. Both effects call this shared function.

**Files to create:**
- `pkg/agent/effects/render.go`

**Files to modify:**
- `pkg/agent/effects/compact.go` — remove `renderConversation`, call shared
- `pkg/agent/effects/sliding_window.go` — remove `renderMessages`, call shared

**Risks:** Minimal. The functions are nearly identical; any behavioral difference is likely a bug.

### 1.3 Deduplicate `contextSleep`

**Problem:** `contextSleep(ctx, duration)` is defined identically in both `pkg/modeladapter/ratelimit.go` and `pkg/modeladapter/batch/collector.go`.

**Solution:** Move to a single unexported function in `pkg/modeladapter/internal/sleeputil/sleep.go` (or inline it in `modeladapter` and have `batch` import via a package-level export). Since `batch` already imports `modeladapter`, the simplest fix is to export `ContextSleep` from `modeladapter` and use it in `batch`.

**Files to modify:**
- `pkg/modeladapter/ratelimit.go` — rename `contextSleep` → export as `ContextSleep`
- `pkg/modeladapter/batch/collector.go` — remove local `contextSleep`, use `modeladapter.ContextSleep`

**Risks:** None. Pure deduplication.

---

## Phase 2 — Strengthen Type Safety and API Consistency

Changes that fix API inconsistencies and improve type safety.

### 2.1 Fix `Message` receiver consistency

**Problem:** `message.Message` uses value receivers everywhere except `SetMeta()` which uses a pointer receiver. This creates a footgun: calling `SetMeta` on a value copy silently discards the mutation from the caller's perspective. The README explicitly warns about this but the inconsistency is a design flaw.

**Solution:** Two sub-options (pick one):

**Option A (Recommended):** Make `SetMeta` a free function: `func SetMeta(m *Message, key string, value any)`. This forces callers to explicitly pass a pointer, making the mutation visible. This avoids changing the value-type semantics of Message.

**Option B:** Change all Message usage to pointer-based (`*Message` everywhere). This is a larger change that cascades through `chat.Chat`, all effects, all providers, and the agent loop.

**Files to modify (Option A):**
- `pkg/chats/message/message.go` — convert `SetMeta` method to function
- All callers of `.SetMeta()` — update to `message.SetMeta(&msg, ...)`

**Risks:** Low for Option A (mechanically find-and-replace all `.SetMeta` calls). High for Option B (cascade).

### 2.2 Make `BySender()` return deep copies (consistent with `Messages()`)

**Problem:** `chat.Chat.Messages()` returns deep-copied messages, but `BySender()` returns shallow copies. Callers who mutate messages from `BySender()` could corrupt internal chat state (shared Part slices, shared Metadata maps).

**Solution:** Apply the same `copyMessage()` function used in `Messages()` to the results of `BySender()`.

**Files to modify:**
- `pkg/chats/chat/chat.go` — apply `copyMessage` in `BySender`

**Risks:** Minimal. Adds allocation cost proportional to message count, but `BySender` is not a hot path.

### 2.3 Rename `GrokAdapter` to `Adapter`

**Problem:** All providers name their exported type `Adapter` (`openai.Adapter`, `anthropic.Adapter`, `gemini.Adapter`) except Grok which uses `grok.GrokAdapter` — redundant stuttering.

**Solution:** Rename `GrokAdapter` → `Adapter` in the `grok` package. Since it's referenced by package-qualified name (`grok.Adapter`), this is a simple rename.

**Files to modify:**
- `pkg/providers/grok/grok.go` — rename type and constructor
- `pkg/providers/grok/batch.go` — update references
- Any registration code in `pkg/engine/` that references `grok.GrokAdapter`

**Risks:** Low. Package-scoped rename with limited external references.

### 2.4 Remove or rethink `Chat.Each()`

**Problem:** `Chat.Each()` holds the read lock during iteration and explicitly warns that `fn` must not call other `Chat` methods (deadlock risk). Meanwhile, `Messages()` already returns a safe deep copy. `Each()` provides marginal value over `for _, m := range c.Messages()` while introducing a footgun.

**Solution:** Deprecate `Each()` or convert it to iterate over a snapshot (like `Messages()` does), releasing the lock before calling `fn`. If no callers depend on the "live iteration under lock" semantics, remove it entirely.

**Analysis step:** Audit all callers of `Each()` first. If all can safely switch to `Messages()`, remove `Each()`. If some need atomic iteration, change to snapshot-then-iterate.

**Files to modify:**
- `pkg/chats/chat/chat.go` — remove or refactor `Each`
- All callers of `Each()` — migrate to `Messages()` or the refactored version

**Risks:** Medium. Need to verify no callers depend on atomically seeing a consistent snapshot during iteration that would be violated by a copy-then-iterate approach.

---

## Phase 3 — Registration and Configuration Hygiene

Make the engine's registration and validation self-maintaining.

### 3.1 Unify effect kind registration

**Problem:** `knownEffectKinds` in `pkg/engine/config.go` must be manually kept in sync with the effect factory map in `pkg/engine/effects.go`. Adding a new effect requires updating both maps independently — a maintenance trap.

**Solution:** Define the effect factory map as the single source of truth. Have `validateAgents()` in `config.go` derive allowed effect kinds from the factory map instead of maintaining a separate `knownEffectKinds` constant.

Concretely:
- Export a function `EffectKinds() []string` from the registration helpers (or make the factory map accessible).
- `validateAgents` calls `EffectKinds()` instead of using the hardcoded `knownEffectKinds` set.

**Files to modify:**
- `pkg/engine/effects.go` — export `EffectKinds()` or a `knownEffects` set derived from the factory
- `pkg/engine/config.go` — remove `knownEffectKinds` map, use `EffectKinds()`

**Risks:** Low. Internal engine change, config validation continues to work identically.

### 3.2 Simplify `registrationContext`

**Problem:** `registrationContext` in `pkg/engine/registration.go` is an internal struct with 18 fields. It acts as a parameter bag for agent factory construction. While the complexity is real (agent registration genuinely requires all these inputs), the large struct makes it easy to miss setting a field.

**Solution:** Split into focused sub-structs grouped by concern:

```go
type completionConfig struct {
    completer  modeladapter.Completer
    maxTokens  int
    modelName  string
}

type toolConfig struct {
    builtinToolboxes []*toolbox.ToolBox
    mcpToolboxes     []*toolbox.ToolBox
    skillStore       *skill.Store
}

type contextConfig struct {
    systemPrompt   string
    projectContext string
    shellyDir      shellydir.Dir
}
```

Then `registrationContext` composes these:
```go
type registrationContext struct {
    completion completionConfig
    tools      toolConfig
    context    contextConfig
    agentCfg   AgentConfig
    // ...remaining fields
}
```

**Files to modify:**
- `pkg/engine/registration.go` — restructure `registrationContext`

**Risks:** Low. Purely internal refactor within a single file.

---

## Phase 4 — Tool Dispatch and Agent Performance

Optimize hot paths and reduce complexity in agent orchestration.

### 4.1 O(1) tool dispatch in the ReAct loop

**Problem:** `callTool` in `pkg/agent/agent.go` iterates through all toolboxes linearly for each tool call: `for _, tb := range toolboxes { if t := tb.Get(name); t != nil { ... } }`. For typical configurations (5-10 toolboxes, 30-50 tools total) this is fast, but it's an unnecessary O(n) per dispatch.

**Solution:** Before the ReAct loop starts, build a flat `map[string]toolbox.Handler` from all toolboxes (respecting priority/override order). Use this map for O(1) dispatch in `callTool`.

The existing `deduplicateTools()` function already iterates all toolboxes to build the tools slice sent to the LLM — extend it to also return a handler map.

**Files to modify:**
- `pkg/agent/agent.go` — `deduplicateTools()` returns `([]toolbox.Tool, map[string]toolbox.Handler)`, `callTool` uses the map

**Risks:** Low. The map is built once per iteration, tool dispatch semantics unchanged (first toolbox wins on name collisions — preserve this by iterating toolboxes in the same order).

### 4.2 Extract `delegateTool` closure into named functions

**Problem:** `delegateTool` in `pkg/agent/delegation.go` is a 130-line anonymous closure that handles spawning child agents, task board integration, event notification, error handling, and result aggregation. It's the densest function in the codebase.

**Solution:** Extract the inner logic into named unexported functions:

```go
func executeChildAgent(ctx context.Context, child *Agent, task tasks.Task, ...) (string, error)
func buildChildAgent(parent *Agent, name string, cfg delegationConfig) (*Agent, error)
func resolveTargetAgent(registry *Registry, name string) (Factory, error)
```

The closure still exists for closure-captured variables (`a *Agent`, config) but delegates to these named functions for the heavy lifting.

**Files to modify:**
- `pkg/agent/delegation.go` — extract functions

**Risks:** Low. Pure refactor of internal structure, no behavioral change.

---

## Phase 5 — Interface and Composition Improvements

Higher-level design improvements following Go composition best practices.

### 5.1 Introduce `InputEnabler` interface in TUI

**Problem:** The `staleFilter` in `cmd/shelly/internal/tty/` uses a type switch on the concrete `app.AppModel` type to determine if input is enabled. This creates a tight coupling from the I/O layer to the UI model.

**Solution:** Define a small interface:
```go
type InputEnabler interface {
    InputEnabled() bool
}
```

The `staleFilter` accepts this interface instead of switching on concrete types. `AppModel` already has an `InputEnabled()` method, so it satisfies this interface implicitly.

**Files to modify:**
- `cmd/shelly/internal/tty/filter.go` — accept `InputEnabler` interface
- `cmd/shelly/internal/app/app.go` — no changes needed (already has the method)

**Risks:** Minimal. Standard Go interface extraction.

### 5.2 Make `ToolBox.Filter([]string{})` return empty toolbox

**Problem:** `ToolBox.Filter(names)` returns the *original* (unfiltered) toolbox when `names` is empty. This means `Filter(nil)` and `Filter([]string{})` both mean "no filter" — but the latter intuitively means "filter to nothing." In YAML config, writing `tools: []` would unexpectedly give all tools.

**Solution:** Distinguish `nil` (no filter, return all) from `[]string{}` (explicit empty filter, return empty toolbox):

```go
func (tb *ToolBox) Filter(names []string) *ToolBox {
    if names == nil {
        return tb // no filter specified
    }
    if len(names) == 0 {
        return &ToolBox{} // explicitly empty
    }
    // ...existing filter logic
}
```

**Analysis step:** Audit all callers of `Filter()` to ensure none pass `[]string{}` expecting "all tools." The YAML config deserialization may produce empty slices vs nil — verify which.

**Files to modify:**
- `pkg/tools/toolbox/toolbox.go` — update `Filter` logic
- Potentially `pkg/engine/` — ensure config deserialization distinguishes nil vs empty

**Risks:** Medium. Behavioral change — could break callers that rely on `[]string{}` meaning "all tools." Requires careful audit.

### 5.3 Fix permissions store listener cleanup

**Problem:** `permissions.Store.OnDirApproved()` returns an unsubscribe function that sets the listener slot to `nil` rather than removing it from the slice. Over many subscribe/unsubscribe cycles in a long-running session, the listener slice grows unboundedly with nil entries.

**Solution:** Two options:

**Option A (Simple):** On unsubscribe, compact the slice by swapping with the last element and truncating. Requires index tracking:
```go
func (s *Store) OnDirApproved(fn func(string)) func() {
    s.listenerMu.Lock()
    idx := len(s.dirListeners)
    s.dirListeners = append(s.dirListeners, fn)
    s.listenerMu.Unlock()
    return func() {
        s.listenerMu.Lock()
        // swap with last, truncate
        last := len(s.dirListeners) - 1
        s.dirListeners[idx] = s.dirListeners[last]
        s.dirListeners[last] = nil
        s.dirListeners = s.dirListeners[:last]
        s.listenerMu.Unlock()
    }
}
```

**Option B (Cleaner):** Use an ID-based map (`map[uint64]func(string)`) instead of a slice.

**Files to modify:**
- `pkg/codingtoolbox/permissions/store.go` — refactor listener tracking

**Risks:** Low. The current nil-slot approach works correctly (nil entries are skipped during dispatch), this just prevents unbounded growth.

---

## Phase 6 — Documentation and Schema Validation

Improvements to developer experience and configuration safety.

### 6.1 Update provider README to match post-refactor architecture

**Problem:** `pkg/providers/README.md` still references `modeladapter.ModelAdapter` which was decomposed into `Client` + `ModelConfig` in commit `88696ec`.

**Solution:** Update the README to reflect the current `Client`/`ModelConfig` composition pattern.

**Files to modify:**
- `pkg/providers/README.md`

**Risks:** None. Documentation only.

### 6.2 Consider structured JSON schemas for tool definitions

**Problem:** Every tool definition in `codingtoolbox/` embeds its JSON schema as a raw string literal. These schemas cannot be validated at compile time — a typo in a schema string is only caught when an LLM tries to call the tool.

**Solution:** This is a larger effort that can be done incrementally. Two approaches:

**Option A (Incremental):** Add a test in each tool package that unmarshals each tool's `InputSchema` into a schema validator and checks it's valid JSON Schema. This catches malformed schemas at test time without changing the definition pattern.

**Option B (Full):** Define Go structs for each tool's input and use a library like `invopop/jsonschema` to generate schemas from struct tags. This provides compile-time type safety and keeps schemas in sync with handler input parsing.

Recommend **Option A** first as a low-risk improvement, then evaluate Option B per-package.

**Files to create (Option A):**
- `pkg/codingtoolbox/*/schema_test.go` — per-package schema validation tests

**Risks:** None for Option A. Medium for Option B (changes tool definition pattern project-wide).

---

## Phase 7 — Future Considerations (Non-urgent)

Items that are correctly designed for current scale but may need attention as the project grows.

### 7.1 `usage.Tracker` unbounded history

The `[]TokenCount` slice in `usage.Tracker` is never pruned. For typical session lifetimes this is fine, but long-running batch sessions could accumulate thousands of entries. Consider adding a `Compact()` method or a rolling window if batch sessions become common.

### 7.2 `state.Store` value type safety

`state.Store` accepts `any` as values. Values stored through tool handlers are always `json.RawMessage`, but the Go API allows any type. If state sharing between agents becomes more complex, consider a typed wrapper or JSON-only enforcement at the API level.

### 7.3 `projectctx.LoadExternal` unbounded reads

`LoadExternal` reads files with `os.ReadFile` without size limits before the rune-based truncation in `Context.String()`. A 100MB `CLAUDE.md` would be read entirely into memory. Consider adding a size cap at read time.

### 7.4 Sequential task IDs

`tasks.Store` uses sequential IDs (`task-1`, `task-2`) which reset if the store is recreated. If task persistence across sessions becomes a requirement, consider UUIDs or a durable ID counter.

---

## Execution Order Summary

| Phase | Risk | Effort | Impact |
|-------|------|--------|--------|
| 1 — Eliminate duplication | Low | Medium | High — reduces ~250 lines of duplicated code |
| 2 — Type safety & API consistency | Low-Med | Medium | High — prevents mutation bugs, cleaner API |
| 3 — Registration hygiene | Low | Low | Medium — prevents sync bugs in config validation |
| 4 — Tool dispatch & delegation | Low | Low-Med | Medium — performance + readability |
| 5 — Interface improvements | Low-Med | Medium | Medium — better composition, decoupling |
| 6 — Docs & schema validation | None | Low-Med | Medium — developer experience |
| 7 — Future considerations | — | — | Deferred — track and revisit |

Each phase is independently shippable. Phases 1-3 are recommended as the first priority — they are low-risk, high-confidence improvements. Phases 4-6 can be done in any order based on immediate needs.
