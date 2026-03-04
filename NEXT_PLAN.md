# Refactoring Plan — Remaining Items

Continuation of the Shelly refactoring plan. All items from Phases 1–5 and 6.1 are complete. This plan covers the remaining work.

---

## Phase 1 — Schema Validation for Tool Definitions ✅ COMPLETE

Catch malformed tool schemas at test time rather than at LLM invocation time.

### 1.1 Add JSON Schema validation tests for all codingtoolbox tools ✅

**Status:** Complete. Implemented with struct-based schema generation (1.2) which supersedes hand-written validation.

**What was done:**
- Created `pkg/tools/schema/` package with `Generate[T any]() json.RawMessage` — derives JSON Schema from Go struct types via reflection using `json` tags (omitempty = optional) and `desc` tags (descriptions)
- Created `pkg/codingtoolbox/internal/schematest/validate.go` — shared `ValidateTools` helper that validates JSON validity, top-level type, required/properties consistency, and property type validity
- Added `schema_test.go` in every tool package (ask, exec, filesystem, git, http, notes, search) — validates all 24 tool schemas

### 1.2 Struct-based schema generation ✅

**Status:** Complete. Implemented as part of 1.1 instead of being deferred.

**What was done:**
- All input structs across all tool packages now use `desc` struct tags for descriptions and `omitempty` for optional fields
- All `InputSchema: json.RawMessage(...)` literals replaced with `schema.Generate[inputType]()` calls
- Schemas are now derived from the same Go structs that handlers unmarshal into — they cannot drift

---

## Phase 2 — Robustness for Long-Running Sessions

Address unbounded growth issues that are safe at current scale but become problems in long-running or batch sessions.

### 2.1 Cap `usage.Tracker` history

**Problem:** `usage.Tracker` in `pkg/modeladapter/usage/usage.go` appends every `TokenCount` to an unbounded `[]TokenCount` slice. `Total()` does an O(n) scan on every call. In long-running batch sessions with thousands of LLM calls, this wastes memory and CPU.

**Current code:**
```go
func (t *Tracker) Add(tc TokenCount) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.entries = append(t.entries, tc)  // grows forever
}
```

**Solution:** Add a configurable rolling window or running total:

**Option A (Running total + recent window):** Maintain a `total TokenCount` that accumulates as entries arrive. Keep only the last N entries (e.g., 100) for `Last()` and per-entry inspection. `Total()` returns the running total in O(1). This preserves the `Last()` API while bounding memory.

```go
type Tracker struct {
    mu      sync.Mutex
    total   TokenCount        // running accumulator
    entries []TokenCount      // ring buffer, last N entries
    head    int               // ring buffer position
    count   int               // total entries ever added
}
```

**Option B (Simple cap):** Add a `Compact()` method that merges all entries into a single total and resets the slice. Callers (e.g., the engine) can call this periodically.

Recommend **Option A** — it's self-maintaining and requires no caller coordination.

**Files to modify:**
- `pkg/modeladapter/usage/usage.go` — implement bounded tracking
- `pkg/modeladapter/usage/usage_test.go` — test ring buffer behavior

**Risks:** Low. Internal change. The `Total()` and `Last()` APIs remain the same. `Count()` may need to return the actual total count vs. the entries-in-buffer count — clarify semantics.

### 2.2 Add size cap to `projectctx.LoadExternal`

**Problem:** `LoadExternal` in `pkg/projectctx/external.go` uses `os.ReadFile` with no size limit. A 100MB `CLAUDE.md` would be read entirely into memory before the downstream rune-based truncation in `Context.String()`.

**Current code:**
```go
func readFileContent(path string) string {
    data, err := os.ReadFile(path)   // no size limit
    if err != nil { return "" }
    return strings.TrimSpace(string(data))
}
```

**Solution:** Replace `os.ReadFile` with a size-capped reader:

```go
const maxExternalFileSize = 512 * 1024 // 512 KB

func readFileContent(path string) string {
    f, err := os.Open(path)
    if err != nil { return "" }
    defer f.Close()

    lr := io.LimitReader(f, maxExternalFileSize)
    data, err := io.ReadAll(lr)
    if err != nil { return "" }
    return strings.TrimSpace(string(data))
}
```

Also consider capping the total number of `.mdc` files processed by `globSorted`.

**Files to modify:**
- `pkg/projectctx/external.go` — add size cap to `readFileContent`
- `pkg/projectctx/external_test.go` — test that oversized files are truncated

**Risks:** Low. Files exceeding 512KB are almost certainly not legitimate project context files. The cap prevents accidental memory issues while being generous enough for real use cases.

### 2.3 Make `state.Store` enforce JSON-only values

**Problem:** `state.Store` accepts `any` as values in its Go API, but all tool-layer usage stores `json.RawMessage`. The `copyValue` function only deep-copies `json.RawMessage` and `[]byte` — all other types are shared references, creating potential mutation bugs.

**Solution:** Restrict the `Set` method to accept only JSON-serializable values by enforcing `json.RawMessage`:

**Option A (Strict):** Change `Set(key string, value any)` to `Set(key string, value json.RawMessage)`. This is a breaking API change but enforces correctness.

**Option B (Validate):** Keep the `any` signature but marshal/unmarshal non-`json.RawMessage` values on `Set` to ensure deep-copy safety and JSON compatibility:

```go
func (s *Store) Set(key string, value any) error {
    switch v := value.(type) {
    case json.RawMessage:
        s.data[key] = slices.Clone(v)
    default:
        b, err := json.Marshal(v)
        if err != nil { return err }
        s.data[key] = json.RawMessage(b)
    }
}
```

Recommend **Option A** if audit shows all callers already use `json.RawMessage`. Otherwise, **Option B** for backwards compatibility.

**Analysis step:** Audit all callers of `Store.Set()` to determine which option is viable.

**Files to modify:**
- `pkg/state/store.go` — enforce JSON-only values
- `pkg/state/store_test.go` — update tests
- Callers of `Set()` — adapt if signature changes

**Risks:** Medium for Option A (breaking change). Low for Option B (additive behavior).

---

## Phase 3 — Task Store Resilience

### 3.1 Use UUID-based task IDs

**Problem:** `tasks.Store` uses sequential IDs (`task-1`, `task-2`, ...) from a `nextID` counter that starts at 0 on each `Store` construction. If the store is recreated (process restart, new session), IDs restart and may collide with previously emitted IDs that agents still reference.

**Current code:**
```go
s.nextID++
task.ID = fmt.Sprintf("task-%d", s.nextID)
```

**Solution:** Replace sequential IDs with UUIDs or ULIDs for global uniqueness:

**Option A (UUID):** Use `google/uuid` to generate `task-<uuid>` IDs. The project may already have a UUID dependency.

**Option B (ULID):** Use ULIDs for sortable, human-readable IDs that preserve creation order while being globally unique.

**Option C (Seeded counter):** On `Store` construction, scan existing tasks and set `nextID` to `max(existing IDs) + 1`. This avoids adding a dependency but only works if the store is seeded from a snapshot.

Recommend **Option A** unless task persistence across sessions is not currently planned, in which case this can remain deferred.

**Files to modify:**
- `pkg/tasks/store.go` — change ID generation
- `pkg/tasks/store_test.go` — update assertions (no longer `task-1`, `task-2`)

**Risks:** Low. Tests that assert exact ID strings need updating, but the functional behavior is unchanged.

---

## Execution Order Summary

| Phase | Status | Risk | Effort | Impact |
|-------|--------|------|--------|--------|
| 1 — Schema validation | ✅ Complete | None | Low-Med | Medium — catches schema bugs at test time |
| 2 — Long-running robustness | Pending | Low-Med | Medium | High — prevents OOM and mutation bugs |
| 3 — Task store resilience | Pending | Low | Low | Low — only matters if task persistence is added |

Phase 1 is complete. Phase 2 items can be done independently in any order. Phase 3 can be deferred until task persistence becomes a requirement.
