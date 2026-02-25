# Why Coder Sub-Agents Produce Unfocused or Incomplete Work

Analysis of the multi-agent orchestration in Shelly, focused on the dev-team pattern where an orchestrator delegates to planner/coder agents.

## The Chain Reaction

The issues compound: minimal orchestrator instructions lead to underspecified task strings, which means the coder starts with almost no context, explores randomly to figure out what to do, burns iterations, and either hits `max_iterations` or produces a vague "done" response that the orchestrator can't verify.

---

## ~~Issue 1: Sub-Agents Receive Only a Bare Text String~~ (FIXED)

Fixed: Both `delegate_to_agent` and `spawn_agents` now have a required `context` field. The parent LLM provides background information (file contents, decisions, constraints) which is prepended as a `<delegation_context>` user message before the task message. See `pkg/agent/tools.go`.

## ~~Issue 2: Orchestrator and Coder Instructions Are Too Minimal~~ (FIXED)

Fixed: Per-agent skill assignment via `skills` field in `AgentConfig`. Each agent gets only the workflow skills relevant to its role (orchestrator-workflow, planner-workflow, coder-workflow). Skills use YAML frontmatter so only the description goes into the system prompt — full content is loaded on-demand via `load_skill`. The skills cover handoff protocols, verification, note/task-board usage, result formats, and failure handling. See `.shelly/skills/` and `pkg/engine/engine.go`.

## ~~Issue 3: No Structured Completion Signal~~ (FIXED)

Fixed: Sub-agents (depth > 0) now receive a `task_complete` tool that signals completion with structured metadata (`status`, `summary`, `files_modified`, `tests_run`, `caveats`). The system prompt includes a `<completion_protocol>` section. When called, the ReAct loop stops immediately and `delegate_to_agent` returns the `CompletionResult` as JSON. `spawn_agents` includes a `completion` field in each result. Backward compatible: agents that stop without calling `task_complete` still return `TextContent()`. See `pkg/agent/tools.go` and `pkg/agent/agent.go`.

## ~~Issue 4: Compact Effect Is Silently Inert~~ (FIXED)

Fixed: `ProviderConfig.ContextWindow` is now `*int` to distinguish "not set" (nil) from "explicitly disabled" (0). Known provider kinds have built-in default context windows (anthropic: 200k, openai: 128k, grok: 131k). When `context_window` is omitted in YAML, `resolveContextWindow()` in `pkg/engine/provider.go` returns the per-kind default, so compaction works out of the box. Setting `context_window: 0` explicitly disables compaction.

## ~~Issue 5: Task Board Exists But Isn't Wired Into the Workflow~~ (FIXED)

FIXED — delegation tools (`delegate_to_agent`, `spawn_agents`) accept an optional `task_id` parameter for automatic claim/status-update. When provided, the task is auto-claimed for the child agent before it runs, and its status is auto-updated based on the child's `task_complete` result. Skills updated to document the `task_id` workflow. See `pkg/agent/tools.go` and `pkg/agent/agent.go`.

## ~~Issue 6: Notes Are the Only Durable Cross-Agent State, But Their Use Isn't Enforced~~ (FIXED)

Fixed: When an agent has notes tools (detected by `list_notes` in its toolboxes), the system prompt automatically includes a `<notes_protocol>` section that informs the agent about the shared notes system, its durability across compaction, and when to check/write notes. The init wizard now includes "notes" in default toolboxes. See `pkg/agent/agent.go`.

## ~~Issue 7: No Recovery Path for Iteration Exhaustion~~ (FIXED)

Fixed: Delegation tools (`delegate_to_agent`, `spawn_agents`) now synthesize a structured `CompletionResult{Status: "failed"}` on iteration exhaustion instead of propagating an opaque error. The task board is auto-updated to "failed" when `task_id` is provided. The completion protocol in the system prompt instructs sub-agents to proactively call `task_complete` with `status: "failed"` when running low on iterations. Orchestrator and coder workflow skills updated with recovery guidance. See `pkg/agent/tools.go` and `pkg/agent/agent.go`.

## ~~Issue 8: Concurrent Spawns Can Clobber Each Other~~ (FIXED)

Fixed: Added `FileLocker` (per-path `sync.Mutex`) to the `FS` struct. All file-modifying handlers (`fs_write`, `fs_edit`, `fs_patch`, `fs_delete`, `fs_move`, `fs_copy`, `fs_mkdir`) acquire the lock before their read-modify-write cycle. Two-path operations lock in sorted order to avoid deadlocks. Read-only tools don't lock. Additionally, `delegate_to_agent` and `spawn_agents` were unified into a single `delegate` tool that accepts an array of tasks and runs them concurrently. See `pkg/codingtoolbox/filesystem/filelocker.go` and `pkg/agent/tools.go`.

## ~~Issue 9: `agentctx` Name Mismatch With Task Board~~ (FIXED)

Fixed: Added `Store.Reassign()` method that overrides any existing assignee (unlike `Claim()` which rejects re-assignment). The engine's `taskBoardAdapter.ClaimTask()` now calls `Reassign()` instead of `Claim()`, so delegation auto-claim correctly transfers ownership from the orchestrator to the child agent. The manual `shared_tasks_claim` tool still uses `Store.Claim()` via `handleClaim`, preserving "no stealing" semantics for direct agent-to-agent tool calls. See `pkg/tasks/store.go` and `pkg/engine/engine.go`.

---

## Summary Table

| # | Problem | Impact | Key Location |
|---|---------|--------|-------------|
| ~~1~~ | ~~Bare text handoff~~ | ~~FIXED — `context` field added~~ | `tools.go` |
| ~~2~~ | ~~Minimal agent instructions~~ | ~~FIXED — per-agent skills with workflow protocols~~ | `config.yaml`, `.shelly/skills/` |
| ~~3~~ | ~~No structured completion~~ | ~~FIXED — `task_complete` tool with structured metadata~~ | `tools.go`, `agent.go` |
| ~~4~~ | ~~Compact effect inert~~ | ~~FIXED — per-kind default context windows~~ | `provider.go`, `config.go` |
| ~~5~~ | ~~Task board unused~~ | ~~FIXED — `task_id` param on delegation tools for auto lifecycle~~ | `tools.go`, `agent.go` |
| ~~6~~ | ~~Notes not enforced~~ | ~~FIXED — `<notes_protocol>` in system prompt when notes tools present~~ | `agent.go` |
| ~~7~~ | ~~No iteration exhaustion recovery~~ | ~~FIXED — structured `CompletionResult` on exhaustion~~ | `tools.go`, `agent.go` |
| ~~8~~ | ~~Concurrent file clobbering~~ | ~~FIXED — FileLocker + unified `delegate` tool~~ | `filelocker.go`, `tools.go` |
| ~~9~~ | ~~Agent name mismatch in tasks~~ | ~~FIXED — `Reassign()` method + `taskBoardAdapter` uses it~~ | `store.go`, `engine.go` |

---

## Fix Categories

### Configuration / Instructions (no code changes)
- Richer orchestrator instructions: require structured task specs, mandate note usage, define verification steps
- Richer coder instructions: read notes first, report files changed, use task board lifecycle
- ~~Set `context_window` on the provider so compact/trim effects actually fire~~ (DONE — per-kind defaults)

### Code Changes
- ~~**Structured handoff**: Pass a structured task spec (not just free text) to child agents, optionally including relevant parent context snippets~~ (DONE)
- ~~**Structured results**: Return structured data from sub-agents (files modified, tests run, status) instead of just `TextContent()`~~ (DONE)
- ~~**File-level locking**: Coordinate concurrent spawned agents to prevent clobbering~~ (DONE)
- ~~**Completion protocol**: Add an explicit "task complete" tool that sub-agents must call, distinguishing intentional completion from the loop simply ending~~ (DONE)
- ~~**Context propagation**: Optionally forward a summary of the parent's accumulated context to the child at spawn time~~ (DONE)
