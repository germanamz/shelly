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

## Phase 2 — Robustness for Long-Running Sessions ✅ COMPLETE

Address unbounded growth issues that are safe at current scale but become problems in long-running or batch sessions.

### 2.1 Cap `usage.Tracker` history ✅

**Status:** Complete. Implemented Option A (running total + ring buffer).

**What was done:**
- Replaced unbounded `[]TokenCount` slice with a fixed-size ring buffer (default 128 entries)
- Added running `total TokenCount` accumulator — `Total()` now returns in O(1)
- Made buffer size configurable via `WithCapacity` option
- `Last()` returns the most recent entry from the ring buffer
- `Count()` returns the total number of entries ever added

### 2.2 Add size cap to `projectctx.LoadExternal` ✅

**Status:** Complete.

**What was done:**
- Replaced `os.ReadFile` with `io.LimitReader` capped at 512 KB (`DefaultMaxExternalFileSize`) in `readFileContent`
- Made the cap configurable via `ContextConfig.MaxExternalFileSize` in engine config (`context.max_external_file_size` YAML key); 0 = default 512 KB
- Threaded the setting through `Load` → `LoadExternal` → `readFileContent`
- Added `TestReadFileContent_OversizedFile` and `TestLoadExternal_OversizedFileIsTruncated` tests

### 2.3 Make `state.Store` enforce JSON-only values ✅

**Status:** Complete. Implemented Option A (strict typing).

**What was done:**
- Changed `Set(key string, value any)` to `Set(key string, value json.RawMessage)` — enforces JSON-only values at compile time
- Changed `Get` return type from `(any, bool)` to `(json.RawMessage, bool)`
- Changed `Watch` return type from `(any, error)` to `(json.RawMessage, error)`
- Changed `Snapshot` return type from `map[string]any` to `map[string]json.RawMessage`
- Changed internal `data` map from `map[string]any` to `map[string]json.RawMessage`
- `Set` now deep-copies input via `slices.Clone` to prevent caller mutations
- Removed `copyValue` helper — `slices.Clone` on `json.RawMessage` handles all deep-copy needs
- Added `TestSetDeepCopiesInput` to verify input isolation
- Updated all tests to use `json.RawMessage` values
- Audit confirmed the only non-test caller (`handleSet`) already used `json.RawMessage`

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
| 2 — Long-running robustness | ✅ Complete | Low-Med | Medium | High — prevents OOM and mutation bugs |
| 3 — Task store resilience | Pending | Low | Low | Low — only matters if task persistence is added |

Phases 1 and 2 are complete. Phase 3 can be deferred until task persistence becomes a requirement.
