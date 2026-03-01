# NEXT.md — Feature Spec Gap Analysis & Implementation Plan

## Overview

A systematic comparison of the codebase against `FEATURE_SPEC.md` across all 21 sections. The codebase is highly mature — nearly all spec requirements are fully implemented. This document captures the gaps found and proposes fixes.

---

## Summary Table

| # | Area | Severity | Status | Section |
|---|------|----------|--------|---------|
| 6 | Context generation is orphaned — `Generate()` never called | **High** | ~~Dead Code~~ **Done** | §12 |
| 7 | No runtime staleness detection — `IsStale()` never called | **High** | ~~Not Wired~~ **Done** | §12 |
| 8 | Generator produces only shallow package listing | **High** | ~~Incomplete~~ **Done** (removed — replaced by agent-driven indexing) | §12 |
| 9 | No knowledge graph system (`.shelly/knowledge/`) | **High** | ~~Not Implemented~~ **Done** | §12.2 |
| 10 | No `project-indexer` skill in any template | **High** | ~~Not Implemented~~ **Done** | §12.2 |
| 11 | Delegation leaks parent toolboxes to child agents | **High** | ~~Spec Violation~~ **Done** | §6.3 |
| 12 | Task board status update errors silently discarded | Medium | ~~No Error Handling~~ **Done** | §6.5 |
| 13 | No task claim rollback on child failure | Medium | ~~Missing Cleanup~~ **Done** | §6.5 |
| 1 | Gemini rate limit header parser missing | Medium | ~~Not Implemented~~ **Done** (documented — Gemini API does not expose rate limit headers) | §3 |
| 2 | Template naming: `dev-team` → `coding-team` | Low | Naming Mismatch | §16.2 |
| 3 | CLI `--template list` vs `--list` flag | Low | Spec/Code Divergence | §16.2 |
| 4 | Grok constructor doesn't set Name/MaxTokens | Low | API Inconsistency | §2.2 |
| 5 | Spec describes dev-team as 3 agents, code has 5 | Low | Spec Outdated | §16.2 |

---

## 6. Context Generation Is Orphaned — `Generate()` Never Called

**Spec Reference:** §12.1 (Context Sources), §12.2 (Knowledge Graph)

**Problem:** The `Generate()` function in `pkg/projectctx/projectctx.go` produces a structural index and writes it to `.shelly/local/context-cache.json`. However, it is **never called at runtime**. The engine's `Load()` function (`pkg/projectctx/projectctx.go`) reads the 3 context sources but never triggers generation:

```go
// pkg/projectctx/projectctx.go — Load() does:
func Load(d shellydir.Dir, projectRoot string) Context {
    external, _ := LoadExternal(projectRoot)  // ✅ works
    curated, _ := LoadCurated(d)              // ✅ works (but no files exist)
    generated := loadGenerated(d)             // ❌ reads cache file that was never created
    return Context{External: external, Curated: curated, Generated: generated}
}
```

The engine calls `Load()` during initialization (`pkg/engine/engine.go:343-347`) but never calls `Generate()` or `IsStale()`.

**Impact:** The third context source (auto-generated structural index) is completely dead. Agents never receive module info, entry points, or package listings.

**Plan:**

1. Wire `Generate()` into the engine initialization, gated by `IsStale()`:
   ```go
   // In engine.go parallelInit, after loading project context:
   if projectctx.IsStale(dir, projectRoot) {
       _, _ = projectctx.Generate(projectRoot, dir)
   }
   loadedCtx = projectctx.Load(dir, projectRoot)
   ```
2. Consider whether to call `Generate()` only on first run or on every stale detection.
3. Add a `--regenerate-context` CLI flag or `shelly context regenerate` subcommand for manual refresh.

**Files to modify:**
- `pkg/engine/engine.go` — Wire `IsStale()` + `Generate()` into initialization
- `cmd/shelly/main.go` — Optionally add manual regeneration command

---

## 7. No Runtime Staleness Detection

**Spec Reference:** §12.2

**Problem:** `IsStale()` in `pkg/projectctx/projectctx.go:118-132` compares the cache file mtime against `go.mod` mtime. It's properly implemented and tested, but never called outside of tests.

**Impact:** Even if `Generate()` were called once, the cache would go stale as the project evolves and never be refreshed.

**Plan:** Part of #6 — wire `IsStale()` into the engine init path so the cache is regenerated when `go.mod` changes.

**Files to modify:**
- Same as #6

---

## 8. Generator Produces Only Shallow Package Listing

**Spec Reference:** §12.2

**Problem:** The `generateIndex()` function in `pkg/projectctx/generator.go` produces a minimal 3-section output:

```
Go module: github.com/germanamz/shelly

Entry points:
- cmd/shelly/main.go

Packages:
- pkg/agent
- pkg/agent/effects
- pkg/chats
...
```

What it **does NOT** produce:
- Package purpose/role descriptions (from READMEs or doc comments)
- Dependency relationships between packages
- Public API surface (exported types, interfaces)
- File roles or counts
- Architecture overview

The spec envisions something much richer: *"Agents build and maintain this graph using their existing tools"* — implying the generated context should be good enough to orient an agent in the codebase.

**Impact:** Even once wired up, the generated context provides very little value — an agent would learn almost nothing about the project's architecture from a flat package list.

**Plan:**

Enhance `generateIndex()` incrementally:

### Phase 1 — Package descriptions from READMEs
1. For each discovered package in `pkg/`, read the first paragraph of its `README.md` (if present).
2. Include a one-line description next to each package entry:
   ```
   Packages:
   - pkg/agent — ReAct loop, registry delegation, middleware, effects system
   - pkg/chats — Provider-agnostic chat data model (foundation, no pkg deps)
   - pkg/engine — Composition root, wires everything from YAML config
   ```

### Phase 2 — Package doc comments
1. If no README, extract the package doc comment from the first `.go` file.
2. Use this as the description line.

### Phase 3 — Dependency graph (optional)
1. Parse `import` statements to build a basic dependency graph.
2. Include a summary section showing which packages depend on which.

**Recommendation:** Phase 1 alone provides significant value and is low-effort since all packages already have READMEs per project conventions.

**Files to modify:**
- `pkg/projectctx/generator.go` — Enhance `generateIndex()` and `findPackages()`
- `pkg/projectctx/generator_test.go` — Update tests

---

## 9. No Knowledge Graph System

**Spec Reference:** §12.2

**Problem:** The spec describes a filesystem-based knowledge graph:

> *"Entry points: Top-level `*.md` files in `.shelly/` are loaded into agent context automatically. These act as indexes that reference deeper files."*
> *"Deep nodes: Detailed topic files stored in organized subdirectories (e.g., `.shelly/knowledge/architecture.md`, `.shelly/knowledge/api-contracts.md`). Agents navigate to these on-demand based on task relevance."*

**Current state of `.shelly/`:**
- `config.yaml` ✅
- `skills/` directory with workflow skills ✅
- `local/` directory with runtime state ✅
- `context.md` ❌ (no curated entry point)
- `knowledge/` ❌ (directory doesn't exist)

The curated context loading code (`LoadCurated()`) works — it reads `*.md` files from `.shelly/` root. But there are no such files to load.

**Impact:** The knowledge graph is the spec's core vision for project understanding. Without it, agents have no persistent, structured understanding of the codebase beyond what's in CLAUDE.md.

**Plan:**

### Phase 1 — Bootstrap creates a skeleton `context.md`
1. Modify `shellydir.Bootstrap()` to create a starter `.shelly/context.md` with a template:
   ```markdown
   # Project Context

   <!-- This file is loaded into agent context automatically. -->
   <!-- Add project overview, conventions, and links to deeper docs. -->
   ```
2. Create `.shelly/knowledge/` directory in bootstrap.

### Phase 2 — Template-provided entry point
1. Each template (simple-assistant, dev-team) ships a `context.md` tailored to its use case.
2. The `coding-team` template's `context.md` could reference `.shelly/knowledge/architecture.md`, etc.

### Phase 3 — Agent-driven graph building (the spec's full vision)
1. Create a `project-indexer` skill (see #10) that instructs agents to explore the project and write knowledge graph files.
2. On first run, the entry agent detects empty `.shelly/knowledge/` and runs the indexer.
3. On subsequent runs, agents read and update the graph as needed.

**Files to modify:**
- `pkg/shellydir/init.go` — Add `context.md` and `knowledge/` to `Bootstrap()`
- `cmd/shelly/internal/templates/` — Add template-specific `context.md` files

---

## 10. No `project-indexer` Skill

**Spec Reference:** §12.2

**Problem:** The spec states:

> *"Templates ship a skill (e.g., `project-indexer`) that instructs the agent how to explore and index the project."*

No such skill exists in any template. The embedded skills are all workflow-oriented:
- `lead-workflow`
- `explorer-workflow`
- `planner-workflow`
- `coder-workflow`
- `reviewer-workflow`

**Impact:** Without a project-indexer skill, agents have no guidance on how to build or maintain the knowledge graph. The spec's vision of agents autonomously maintaining project understanding is completely absent.

**Plan:**

1. Create `.shelly/skills/project-indexer/SKILL.md` as an on-demand skill (has description so it's loaded via `load_skill` tool rather than embedded in every prompt).
2. The skill should instruct the agent to:
   - Explore the project structure using filesystem and search tools
   - Read key files (README, go.mod, entry points, package docs)
   - Write an index to `.shelly/context.md` (loaded into prompt automatically)
   - Write detailed topic files to `.shelly/knowledge/` (read on-demand by agents)
3. Include the skill in the `simple-assistant` and `dev-team` templates.
4. The `explorer` agent in the `dev-team` template is a natural fit — add `project-indexer` to its skills list.

**Files to create:**
- `cmd/shelly/internal/templates/skills/project-indexer.md` — The skill content
- Update template YAML files to reference the skill

---

## 11. Delegation Leaks Parent Toolboxes to Child Agents

**Spec Reference:** §6.3 point 4

**Problem:** The spec states:

> "Child uses only its own configured toolboxes (no inheritance from parent)"

But the delegation tool at `pkg/agent/tools.go:176` explicitly adds the parent's toolboxes to the child:

```go
// tools.go:147-150 — snapshot parent's toolboxes
toolboxSnapshot := make([]*toolbox.ToolBox, len(a.toolboxes))
copy(toolboxSnapshot, a.toolboxes)

// tools.go:176 — add parent's toolboxes to child
child.AddToolBoxes(toolboxSnapshot...)
```

The child agent is created by `registry.Spawn()` which calls the factory. The factory (`pkg/engine/engine.go:726-727`) already gives the child its own configured toolboxes:

```go
a := agent.New(name, desc, instr, completer, opts)
a.AddToolBoxes(tbs...)  // child's OWN toolboxes from its config
```

Then `tools.go:176` appends the parent's toolboxes ON TOP. `AddToolBoxes` deduplicates by pointer equality, but since the engine creates separate `ToolBox` instances per toolbox name, most parent toolboxes are distinct pointers and get added.

**Impact:** A child agent configured with only `[filesystem, search]` will also get the parent's `[exec, git, http, browser, ...]` — violating the principle of least privilege. An "explorer" agent that should only read files could also run commands if its parent has `exec`.

**Plan:**

1. Remove `child.AddToolBoxes(toolboxSnapshot...)` from `pkg/agent/tools.go:176`.
2. Remove the toolbox snapshot code at lines 149-150 (no longer needed).
3. Update or remove tests that verify toolbox inheritance:
   - `TestSpawnAgentsToolboxInheritance` (agent_test.go)
   - `TestDelegateToolboxInheritance` (agent_test.go)
4. Add a new test verifying child uses ONLY its own toolboxes after delegation.

**Files to modify:**
- `pkg/agent/tools.go` — Remove lines 149-150 and 176
- `pkg/agent/agent_test.go` — Fix/replace inheritance tests

---

## 12. Task Board Status Update Errors Silently Discarded

**Spec Reference:** §6.5

**Problem:** At two locations in `pkg/agent/tools.go`, task board update errors are silently discarded:

```go
// tools.go:215 — after ErrMaxIterations
_ = taskBoard.UpdateTaskStatus(t.TaskID, cr.Status)

// tools.go:236 — after successful completion
_ = taskBoard.UpdateTaskStatus(t.TaskID, cr.Status)
```

**Impact:** If the task board update fails (e.g., task already in terminal state, or task ID invalid), the parent agent receives a successful delegation result but the task board shows stale state. Other agents watching the task via `tasks_watch` would hang indefinitely.

**Plan:**

1. Log the error rather than discarding it (at minimum).
2. Consider including the error in the `delegateResult` as a warning field so the parent can be aware.

**Files to modify:**
- `pkg/agent/tools.go` — Handle errors from `UpdateTaskStatus` at lines 215 and 236

---

## 13. No Task Claim Rollback on Child Failure

**Spec Reference:** §6.5

**Problem:** In `pkg/agent/tools.go:186-194`, tasks are claimed BEFORE the child runs:

```go
if t.TaskID != "" && taskBoard != nil {
    if claimErr := taskBoard.ClaimTask(t.TaskID, child.name); claimErr != nil {
        // ... return error
    }
}
// ... later:
reply, err := child.Run(ctx)  // if this panics or context cancels...
```

If `child.Run()` fails with an error other than `ErrMaxIterations` (e.g., context cancellation, panic in tool), the task remains in `in_progress` state claimed by a dead child. No cleanup logic releases it.

**Impact:** Stale tasks stuck in `in_progress` with no agent working on them. Other agents cannot claim or reassign these tasks.

**Plan:**

1. Add a deferred cleanup that transitions the task to `failed` if the child errors out non-gracefully:
   ```go
   defer func() {
       if t.TaskID != "" && taskBoard != nil && child.CompletionResult() == nil {
           _ = taskBoard.UpdateTaskStatus(t.TaskID, "failed")
       }
   }()
   ```
2. This is only needed for the case where `Run()` returns an error that is NOT `ErrMaxIterations` (which already handles task updates at line 215).

**Files to modify:**
- `pkg/agent/tools.go` — Add deferred task cleanup after claim

---

## 1. Gemini Rate Limit Header Parser Missing

**Spec Reference:** §3.2 (Reactive Retry), §3.3 (Adaptive Throttling)

**Problem:** The Gemini provider (`pkg/providers/gemini/gemini.go`) does not set `HeaderParser` on its `ModelAdapter`. All other providers do:

```
pkg/providers/anthropic/anthropic.go:41  → a.HeaderParser = modeladapter.ParseAnthropicRateLimitHeaders
pkg/providers/openai/openai.go:35       → a.HeaderParser = modeladapter.ParseOpenAIRateLimitHeaders
pkg/providers/grok/grok.go:35           → a.HeaderParser = modeladapter.ParseOpenAIRateLimitHeaders
pkg/providers/gemini/gemini.go          → (nothing)
```

**Impact:** Adaptive throttling (`RateLimitedCompleter.adaptFromServerInfo()`) will never activate for Gemini because `LastRateLimitInfo()` always returns nil. The completer will fall back to proactive throttling only, which works but isn't optimal — the completer can't back off when Gemini signals capacity is nearly exhausted.

**Plan:**

1. Research Gemini API rate limit response headers. Google's Generative AI API may use headers like `x-ratelimit-remaining-requests`, `x-ratelimit-remaining-tokens`, etc. or its own format.
2. Implement `ParseGeminiRateLimitHeaders()` in `pkg/modeladapter/ratelimitinfo.go` following the same pattern as existing parsers.
3. Set `a.HeaderParser = modeladapter.ParseGeminiRateLimitHeaders` in `gemini.New()`.
4. Add tests in `pkg/providers/gemini/gemini_test.go` covering:
   - Header parsing with valid headers
   - Missing headers returning nil
   - Partial headers
5. If Gemini doesn't expose rate limit headers at all, document that explicitly in a code comment and in `pkg/providers/gemini/README.md`.

**Files to modify:**
- `pkg/modeladapter/ratelimitinfo.go` — Add `ParseGeminiRateLimitHeaders()`
- `pkg/providers/gemini/gemini.go` — Set `HeaderParser` in `New()`
- `pkg/providers/gemini/gemini_test.go` — Add rate limit header tests

---

## 2. Template Naming: `dev-team` vs `coding-team`

**Spec Reference:** §16.2

**Problem:** The spec and README both reference a template called `dev-team`:

```
FEATURE_SPEC.md:1056  →  ./bin/shelly init --template dev-team
README.md:194          →  ./bin/shelly init --template dev-team
pkg/engine/README.md:318 → The `dev-team` config template pre-assigns workflow skills...
```

But the actual template file is named `coding-team`:

```
cmd/shelly/internal/templates/settings/coding-team.yaml → template.name: coding-team
```

**Impact:** Users following the README or spec will get a "template not found" error when running `--template dev-team`.

**Plan:** Decide on one canonical name and align everything. Two options:

**Option A — Rename template to `dev-team` (match the spec):**
- Rename `cmd/shelly/internal/templates/settings/coding-team.yaml` → `dev-team.yaml`
- Change `template.name` inside the YAML from `coding-team` to `dev-team`
- Update test references in `cmd/shelly/internal/templates/templates_test.go`

**Option B — Update spec/README to `coding-team` (match the code):**
- Update `FEATURE_SPEC.md` line 1056
- Update `README.md` line 194
- Update `pkg/engine/README.md` line 318

**Recommendation:** Option A — the spec is the source of truth, rename the file.

**Files to modify:**
- `cmd/shelly/internal/templates/settings/coding-team.yaml` → rename to `dev-team.yaml`
- `cmd/shelly/internal/templates/templates_test.go` — update references
- Or alternatively update `FEATURE_SPEC.md`, `README.md`, `pkg/engine/README.md`

---

## 3. CLI `--template list` vs `--list` Flag

**Spec Reference:** §16.2

**Problem:** The spec shows:

```bash
./bin/shelly init --template list              # List available templates
```

But the implementation uses a separate boolean flag:

```go
// cmd/shelly/init.go
templateName := fs.String("template", "", "template name (non-interactive)")
list := fs.Bool("list", false, "list available templates")
```

So the actual CLI is:

```bash
./bin/shelly init --list                       # List templates
./bin/shelly init --template simple-assistant  # Apply template
```

**Impact:** Minor UX mismatch. The `--list` flag works fine but doesn't match documentation.

**Plan:** Two options:

**Option A — Support both (backward compatible):**
- Keep `--list` flag as-is
- Add special-case handling: if `--template` value is `"list"`, treat it as listing
- This matches the spec while keeping the existing `--list` flag working

**Option B — Update spec to match code:**
- Change `FEATURE_SPEC.md` and `README.md` to show `--list`

**Recommendation:** Option B — the separate `--list` flag is cleaner CLI design. Update the spec.

**Files to modify:**
- `FEATURE_SPEC.md` — Update the template CLI examples
- `README.md` — Update matching section

---

## 4. Grok Constructor Doesn't Set Name/MaxTokens

**Spec Reference:** §2.2

**Problem:** The Grok provider constructor `New()` doesn't set `Name` (model) or `MaxTokens`, unlike the other three providers:

```go
// Anthropic — sets both
func New(apiKey string, client *http.Client) *AnthropicAdapter {
    a.Name = ""  // set by engine later, but MaxTokens = 4096 set here
    a.MaxTokens = 4096

// OpenAI — sets both
func New(apiKey string, client *http.Client) *OpenAIAdapter {
    a.MaxTokens = 4096

// Grok — sets neither
func New(apiKey string, client *http.Client) *GrokAdapter {
    // only HeaderParser is set
```

The engine sets `Name` and `MaxTokens` post-construction (`pkg/engine/provider.go`), so this works at runtime. But it's an API inconsistency — direct users of the Grok package get zero defaults for `MaxTokens`.

**Impact:** Low — engine wiring handles it. Only matters for library consumers using Grok directly.

**Plan:**

1. Add `a.MaxTokens = 4096` to `grok.New()` for consistency
2. The engine can still override it, this just provides a sane default

**Files to modify:**
- `pkg/providers/grok/grok.go` — Add `a.MaxTokens = 4096` in `New()`

---

## 5. Spec Template Description Outdated

**Spec Reference:** §16.2

**Problem:** The spec describes the multi-agent template as:

```
./bin/shelly init --template dev-team           # Orchestrator + planner + coder
```

But the actual `coding-team.yaml` template has 5 agents: **lead, explorer, planner, coder, reviewer** — which is more than the 3 described.

**Impact:** Misleading documentation. Users expect 3 agents but get 5.

**Plan:** Update the spec description to accurately reflect the template contents:

```
./bin/shelly init --template dev-team           # Lead + explorer + planner + coder + reviewer
```

**Files to modify:**
- `FEATURE_SPEC.md` — Update template description
- `README.md` — Update matching description if present

---

## Sections with Full Compliance (No Action Needed)

For completeness, all other spec sections are fully implemented:

| Section | Feature | Status |
|---------|---------|--------|
| §1 | Overview | Complete |
| §2.1 | Provider-agnostic architecture (Completer interface) | Complete |
| §2.2 | Anthropic, OpenAI, Grok, Gemini providers | Complete |
| §2.3 | Provider YAML configuration with env expansion | Complete |
| §2.4 | Custom provider registration (RegisterProvider) | Complete |
| §2.5 | Default context window overrides | Complete |
| §3.1 | Proactive throttling (TPM/RPM sliding window) | Complete |
| §3.2 | Reactive retry (429, exponential backoff, Retry-After) | Complete |
| §3.3 | Adaptive throttling (server reset time) | Complete (except Gemini, see #1) |
| §4 | Chat data model (roles, parts, messages, container) | Complete |
| §5.1 | ReAct loop (8-step cycle) | Complete |
| §5.2 | Agent configuration (all fields) | Complete |
| §5.3 | System prompt structure (9 ordered sections) | Complete |
| §5.4 | Middleware (Timeout, Recovery, Logger, OutputGuardrail) | Complete |
| §5.5 | Entry agent designation | Complete |
| §6.1 | Registry (Register, Spawn, NextID) | Complete |
| §6.2 | Delegation depth (tool injection rules) | Complete |
| §6.3 | Delegation tool (all features) | **Partial — see #11 (toolbox leakage)** |
| §6.4 | Completion protocol (task_complete, CompletionResult) | Complete |
| §6.5 | Task board integration (auto-claim, auto-update) | **Partial — see #12, #13 (error handling, rollback)** |
| §7.1-7.4 | Effect interface, phases, lifecycle, priority sorting | Complete |
| §7.5 | All 7 effects implemented | Complete |
| §7.6 | CompactEffect | Complete |
| §7.7 | TrimToolResultsEffect | Complete |
| §7.8 | SlidingWindowEffect | Complete |
| §7.9 | ObservationMaskEffect | Complete |
| §7.10 | LoopDetectEffect | Complete |
| §7.11 | ReflectionEffect | Complete |
| §7.12 | ProgressEffect | Complete |
| §7.13 | Auto-generated effects | Complete |
| §8 | Token estimation (heuristic, EstimateChat/Tools/Total) | Complete |
| §9.1 | Tool architecture (ToolBox, Handler, InputSchema) | Complete |
| §9.2 | Permission model (directory/command/domain-level) | Complete |
| §9.3 | Filesystem tools (11 + bonus fs_read_lines) | Complete |
| §9.4 | Exec tool (direct subprocess, 1MB cap, coalescing) | Complete |
| §9.5 | Search tools (regex + glob) | Complete |
| §9.6 | Git tools (4, format restrictions, path protection) | Complete |
| §9.7 | HTTP tool (SSRF protection, redirect validation) | Complete |
| §9.8 | Browser tools (6, chromedp, lazy startup) | Complete |
| §9.9 | Notes tools (3, name validation, persistence) | Complete |
| §9.10 | Ask tool (auto-increment IDs, multi-select) | Complete |
| §9.11 | State tools (KV store with Watch) | Complete |
| §9.12 | Task tools (6, status flow, blocking deps) | Complete |
| §9.13 | Toolbox assignment (per-agent, whitelist, ask implicit) | Complete |
| §10 | Skills system (folder-based, inline/on-demand, filtering) | Complete |
| §11 | MCP integration (stdio + HTTP, client + server) | Complete |
| §12 | Project context (external + curated + generated) | **Partial — see #6-#10** |
| §13 | Shelly directory (layout, bootstrap, migrate) | Complete |
| §14.1 | Engine (composition root, parallel MCP init) | Complete |
| §14.2 | Session (Send, SendParts, Chat, Respond, mutex) | Complete |
| §14.3 | EventBus (8 event kinds, non-blocking publish) | Complete |
| §15.1 | TUI architecture (Bubbletea v2, state machine) | Complete |
| §15.2 | Layout (messages + input regions, 80-col min) | Complete |
| §15.3 | Theme (GitHub light, dark detection, 7 colors, sub-agent colors) | Complete |
| §15.4 | User input (6 key bindings, token counter) | Complete |
| §15.5 | File picker (WalkDir, filtering, 4-item window) | Complete |
| §15.6 | Command picker (/help, /clear, /exit, substring filter) | Complete |
| §15.7 | Chat view (5 display items, sub-agent nesting, tool groups) | Complete |
| §15.8 | Task panel (status counts, sorting, icons, max 6) | Complete |
| §15.9 | Ask User UI (200ms debounce, tabbed, 4 question types) | Complete |
| §15.10 | Bridge (3 goroutines: event, chat, task watchers) | Complete |
| §15.11 | Markdown rendering (Glamour, cached, theme-aware) | Complete |
| §15.12 | Stale send detection (sendGeneration counter) | Complete |
| §16.1 | Config wizard (5 screens, stack navigation) | Complete |
| §17 | Concurrency & thread safety (all guarantees) | Complete |
| §18 | Security (permissions, SSRF, output caps, path traversal) | Complete |
| §19 | Reflections (failure notes, retrieval on delegation) | Complete |
| §20 | Agent context propagation (agentctx package) | Complete |
| §21 | Development tooling (gofumpt, golangci-lint, testify, gotestsum, go-task) | Complete |

---

## Recommended Implementation Order

### High Priority — Delegation Bugs (§6)

1. ~~**#11 Remove parent toolbox leakage** — Spec violation, security concern (least privilege). Small, targeted fix.~~ **Done**

### High Priority — Context Generation System (§12)

These form a connected feature area and should be implemented together:

2. ~~**#6 Wire `Generate()` + `IsStale()` into engine** — Minimal effort, unlocks the generated context source~~ **Done**
3. ~~**#8 Enhance generator output** — Phase 1 (README descriptions) provides high value for low effort~~ **Done** (removed — replaced by agent-driven indexing)
4. ~~**#9 Bootstrap knowledge graph skeleton** — Add `context.md` and `knowledge/` to bootstrap + templates~~ **Done**
5. ~~**#10 Create `project-indexer` skill** — Enables the agent-driven knowledge graph vision~~ **Done**

### Medium Priority

6. ~~**#12 Task board silent error handling** — Could cause watch hangs in multi-agent workflows~~ **Done**
7. ~~**#13 Task claim rollback** — Stale tasks on child failure~~ **Done**
8. ~~**#1 Gemini rate limit headers** — Functional gap in adaptive throttling~~ **Done** (documented — no headers available)

### Low Priority — Documentation & Polish

9. **#2 Template rename** (`coding-team` → `dev-team`) — User-facing naming mismatch
10. **#5 Spec description update** — Docs accuracy
11. **#3 CLI flag docs** — Docs accuracy
12. **#4 Grok constructor default** — API consistency polish
