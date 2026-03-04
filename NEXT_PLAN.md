# Refactoring Plan — Remaining Items

Continuation of the Shelly refactoring plan. All items from Phases 1–5 and 6.1 are complete. This plan covers the remaining work.

---

## Phase 1 — Schema Validation for Tool Definitions

Catch malformed tool schemas at test time rather than at LLM invocation time.

### 1.1 Add JSON Schema validation tests for all codingtoolbox tools

**Problem:** Every tool in `codingtoolbox/` defines its `InputSchema` as a hardcoded `json.RawMessage` string literal. A typo or structural error in any schema is only caught when an LLM tries to call the tool at runtime. No existing test validates that these schemas are well-formed JSON or valid JSON Schema.

**Affected packages:**
- `pkg/codingtoolbox/ask/`
- `pkg/codingtoolbox/exec/`
- `pkg/codingtoolbox/filesystem/`
- `pkg/codingtoolbox/git/`
- `pkg/codingtoolbox/http/`
- `pkg/codingtoolbox/notes/`
- `pkg/codingtoolbox/search/`
- `pkg/codingtoolbox/defaults/`

**Solution:** Create a single shared test helper and per-package `schema_test.go` files:

1. Add a test helper (e.g., in `pkg/codingtoolbox/internal/schematest/`) that:
   - Unmarshals `InputSchema` to verify it's valid JSON
   - Validates it conforms to JSON Schema Draft 2020-12 structure (has `"type"`, valid `"properties"`, etc.)
   - Checks `"required"` fields reference properties that actually exist in the schema

2. Each tool package gets a `schema_test.go` that calls the helper for all tools it exports.

**Files to create:**
- `pkg/codingtoolbox/internal/schematest/validate.go` — shared validation helper
- `pkg/codingtoolbox/ask/schema_test.go`
- `pkg/codingtoolbox/exec/schema_test.go`
- `pkg/codingtoolbox/filesystem/schema_test.go`
- `pkg/codingtoolbox/git/schema_test.go`
- `pkg/codingtoolbox/http/schema_test.go`
- `pkg/codingtoolbox/notes/schema_test.go`
- `pkg/codingtoolbox/search/schema_test.go`
- `pkg/codingtoolbox/defaults/schema_test.go`

**Risks:** None. Test-only additions — no production code changes. Can use `encoding/json` for basic validation or bring in a lightweight JSON Schema validator if the project already has one as a dependency.

### 1.2 (Future) Evaluate struct-based schema generation

**Problem:** Even with validation tests, the schemas are still hand-written strings that can drift from handler input parsing logic.

**Solution:** Evaluate using `invopop/jsonschema` or similar to generate `InputSchema` from Go structs with JSON tags. This would provide compile-time type safety and keep schemas in sync with handler input parsing. Do this per-package as an incremental migration.

**Status:** Deferred — evaluate after 1.1 is in place.

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

| Phase | Risk | Effort | Impact |
|-------|------|--------|--------|
| 1 — Schema validation | None | Low-Med | Medium — catches schema bugs at test time |
| 2 — Long-running robustness | Low-Med | Medium | High — prevents OOM and mutation bugs |
| 3 — Task store resilience | Low | Low | Low — only matters if task persistence is added |

Phase 1 is the highest-confidence, lowest-risk starting point. Phase 2 items can be done independently in any order. Phase 3 can be deferred until task persistence becomes a requirement.
