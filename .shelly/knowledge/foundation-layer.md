# Shelly Foundation Layer Overview

## Layer Architecture

The Shelly foundation layer consists of three core packages that provide zero-dependency abstractions upon which all other system components are built:

```
Foundation Layer (Layer 1)
├── pkg/chats/        # Provider-agnostic chat data model
├── pkg/agentctx/     # Agent identity context helpers  
└── pkg/shellydir/    # .shelly/ directory management
```

All three packages have **zero dependencies on other `pkg/` packages**, making them safe to import from anywhere in the system.

---

## pkg/chats — Chat Data Model

**Purpose:** Defines the complete data model for LLM conversations — roles, content parts, messages, and a thread-safe conversation container.

**Sub-packages:**

| Sub-package | Key Types | Purpose |
|-------------|-----------|---------|
| `role` | `Role` (string type) | Four roles: `System`, `User`, `Assistant`, `Tool`; `Valid()` check |
| `content` | `Part` interface; `Text`, `Image`, `Document`, `ToolCall`, `ToolResult` | Multi-modal content parts |
| `message` | `Message` struct | Value type with `Sender`, `Role`, `Parts []content.Part`, `Metadata`; helpers like `TextContent()`, `ToolCalls()`, `ToolResults()` |
| `chat` | `Chat` struct | Thread-safe mutable conversation with `Append`, `Messages`, `BySender`, `Since`, `At`, `Last`, `Replace`, `Wait(ctx, n)`, `SystemPrompt` |

**Key design decisions:**
- `Message` is a value type (copy-safe), `Chat` is a pointer type (mutable, locked)
- `content.Part` is an interface — extensible for new content types
- `ToolCall.Arguments` is a raw JSON **string** (not `json.RawMessage`)
- `Image` and `Document` carry `Data []byte` + `MediaType string` (binary, not URLs)
- `message.SetMeta` is a **free function** returning a new message (value semantics)
- `Chat.Wait(ctx, n)` blocks until more than `n` messages exist; `Append`/`Replace` signal implicitly

**Detailed docs:** [chats-foundation.md](chats-foundation.md)

---

## pkg/agentctx — Agent Identity

**Purpose:** Two functions and a filename sanitizer. Propagates agent names via `context.Context` without creating import cycles.

**Complete API:**

| Function | Signature | Purpose |
|----------|-----------|---------|
| `WithAgentName` | `(ctx, name) → context.Context` | Stores agent name in context |
| `AgentNameFromContext` | `(ctx) → string` | Reads agent name from context (empty if absent) |
| `SanitizeFilename` | `(s) → string` | Replaces non-`[a-zA-Z0-9_-]` chars with hyphens |

**That's the entire package.** Two source files (`context.go`, `sanitize.go`), no types, no interfaces, no structs (other than the unexported context key).

**Detailed docs:** [agentctx-identity.md](agentctx-identity.md)

---

## pkg/shellydir — Directory Management

**Purpose:** Encapsulates all path knowledge for `.shelly/`, plus bootstrap and migration logic.

**Key types and functions:**

| Item | Purpose |
|------|---------|
| `Dir` (value object) | Created via `New(root)`. 12+ path accessor methods (`Root`, `ConfigPath`, `ContextPath`, `SkillsDir`, `KnowledgeDir`, `LocalDir`, `PermissionsPath`, `NotesPath`, `ReflectionsDir`, `SessionsDir`, `GitignorePath`, `DefaultsPath`) plus listing helpers (`KnowledgeFiles`, `SkillDirs`) |
| `Bootstrap(d)` | Creates full `.shelly/` structure from scratch with skeleton config |
| `BootstrapWithConfig(d, config)` | Creates structure with provided config bytes |
| `EnsureStructure(d)` | Idempotent — creates only `local/`, `sessions/`, `.gitignore` if missing |
| `MigratePermissions(d)` | Moves legacy permissions file to `local/` |

**Key design decisions:**
- `Dir` is a pure value object — path accessors do no I/O
- Bootstrap never overwrites existing files (`O_CREATE|O_EXCL`)
- `local/` is gitignored (runtime state only)

**Detailed docs:** [shellydir-paths.md](shellydir-paths.md)

---

## Cross-Cutting Patterns

1. **Zero coupling:** All three packages can be imported by any other package safely.
2. **Value semantics:** `Dir`, `Message`, and `Role` are all value types. Only `Chat` uses pointer semantics (it holds a mutex).
3. **Thread safety:** `Chat` is fully thread-safe internally. The other packages are stateless.
4. **No global state:** No `init()`, no package-level vars that need setup.
5. **Test-friendly:** Value types and lack of I/O in accessors make all three packages easy to unit test.

## Dependency Flow

```
pkg/chats/     ← pkg/modeladapter, pkg/providers/*, pkg/agent, pkg/engine
pkg/agentctx/  ← pkg/agent, pkg/engine, pkg/codingtoolbox
pkg/shellydir/ ← pkg/projectctx, pkg/engine, cmd/shelly
```

No foundation package imports any other `pkg/` package.
