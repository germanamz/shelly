# codingtoolbox

Built-in coding tools that give agents controlled access to the local environment. Each sub-package implements a specific tool category with appropriate permission gating.

## Architecture

```
codingtoolbox/
├── ask/           ask_user tool — prompts the user and blocks until a response
├── browser/       browser_search, browser_navigate, browser_click, browser_type, browser_extract, browser_screenshot — headless Chrome automation
├── filesystem/    fs_read, fs_write, fs_edit, fs_list, fs_delete, fs_move, fs_copy, fs_stat, fs_diff, fs_patch, fs_mkdir
├── exec/          exec_run — permission-gated command execution
├── search/        search_content, search_files — permission-gated content/file search
├── git/           git_status, git_diff, git_log, git_commit — permission-gated git ops
├── http/          http_fetch — permission-gated HTTP requests
├── notes/         write_note, read_note, list_notes — persistent notes surviving context compaction
├── permissions/   Shared permissions store (approved dirs, trusted commands, trusted domains)
└── defaults/      Default toolbox builder — merges built-in toolboxes into one
```

**Dependency graph**: `permissions` is shared by `filesystem`, `exec`, `search`, `git`, `http`, and `browser`. All tool packages depend on `pkg/tools/toolbox` for the `Tool` and `ToolBox` types. `defaults` merges multiple toolboxes into a single one that every agent receives.

## Sub-packages

### `ask` -- User Interaction

Provides the `ask_user` tool and a `Responder` that manages pending questions. The `Responder` holds a map of pending question channels keyed by auto-incremented IDs (`q-1`, `q-2`, ...). Questions can be free-form or include multiple-choice options via the `Options` field on the `Question` struct. The `Question` struct also carries optional `Header` (short tab label) and `MultiSelect` (checkboxes vs single choice) fields for frontend rendering.

The `Ask` method can be called programmatically by other packages (e.g. `filesystem`, `exec`) to prompt the user without going through the JSON tool handler layer. The engine hooks up the responder to the event bus so frontends can present questions.

**Exported types**: `Question`, `OnAskFunc`, `Responder`.
**Constructor**: `NewResponder(onAsk OnAskFunc) *Responder`.
**Methods**: `Respond(questionID, response string) error`, `Ask(ctx, text string, options []string) (string, error)`, `Tools() *toolbox.ToolBox`.

### `browser` -- Browser Automation

Permission-gated tools for headless Chrome automation via chromedp. Provides web search (DuckDuckGo -- no domain trust required), page navigation with text extraction, element clicking, text input, content extraction by CSS selector, and screenshots (viewport, full page, or element). Chrome is started lazily on first tool use via `ensureBrowser` and runs in incognito mode with GPU disabled. Domain trust is shared with the `http` package via the permissions store.

Extracted text has scripts, styles, noscript, and SVG elements stripped, with whitespace collapsed. Text output is capped at 100KB. Each browser operation uses a 30-second timeout.

**Exported types**: `AskFunc`, `Option`, `Browser`.
**Constructor**: `New(store *permissions.Store, askFn AskFunc, opts ...Option) *Browser`.
**Options**: `WithHeadless()` -- enables headless Chrome mode.
**Methods**: `Tools() *toolbox.ToolBox`, `Close()` -- shuts down the Chrome process.

### `filesystem` -- File Access

Permission-gated tools for reading, writing, editing, listing, deleting, moving, copying, stat-ing, diffing, patching files, and creating directories. Users approve directory access on first touch; approvals persist via the shared permissions store. Approving a directory implicitly approves all its subdirectories. Symlinks are resolved and the real target directory requires independent approval if it differs from the apparent path.

File-modifying operations (`fs_write`, `fs_edit`, `fs_patch`, `fs_delete`, `fs_move`, `fs_copy`, `fs_mkdir`) show a unified diff (via go-difflib) and require user confirmation. Users can choose "trust this session" to skip confirmation for subsequent changes within the session. `SessionTrust` tracks this per-session state and is propagated via context using `WithSessionTrust`. When the session is trusted, a `NotifyFunc` callback displays changes without blocking.

The `fs_read` tool caps file reads at 10MB. The `fs_edit` tool requires the `old_text` to appear exactly once. The `fs_patch` tool applies multiple find-and-replace hunks sequentially in one atomic operation. Concurrent writes to the same file are serialized via a per-path `FileLocker`. Two-path operations (`fs_move`, `fs_copy`) use `LockPair`/`UnlockPair` with consistent ordering to avoid deadlocks.

**Exported types**: `AskFunc`, `NotifyFunc`, `FS`, `FileLocker`, `SessionTrust`.
**Constructor**: `New(store *permissions.Store, askFn AskFunc, notifyFn NotifyFunc) *FS`.
**Helpers**: `NewFileLocker() *FileLocker`, `WithSessionTrust(ctx, st *SessionTrust) context.Context`.
**Methods**: `Tools() *toolbox.ToolBox`.

### `exec` -- Command Execution

Permission-gated tool for running CLI commands. Users can approve once ("yes") or "trust" a command (program name) for all future invocations without being prompted again. Trusted commands are persisted to the shared permissions store. Commands are executed directly via `os/exec` (no shell interpretation). Stdout/stderr are captured with a 1MB cap via `limitedBuffer`.

Concurrent permission prompts for the same command are coalesced: when a prompt is in-flight, subsequent callers wait for its result. One-time approvals ("yes") are not coalesced since they apply to specific arguments. An optional `OnExecFunc` callback notifies the frontend when a trusted command is about to execute.

**Exported types**: `AskFunc`, `OnExecFunc`, `Option`, `Exec`.
**Constructor**: `New(store *permissions.Store, askFn AskFunc, opts ...Option) *Exec`.
**Options**: `WithOnExec(fn OnExecFunc)` -- callback for trusted command display.
**Methods**: `Tools() *toolbox.ToolBox`.

### `search` -- Content & File Search

Permission-gated tools for searching file contents by regex (`search_content`) and finding files by glob pattern (`search_files`) with `**` support for recursive matching. Uses directory approval from the shared permissions store. Symlinks are resolved and checked to prevent escaping approved directories.

Content search skips binary files (UTF-8 validity check on the first 512 bytes) and caps total matched content at 1MB. Both tools default to 100 max results. File search supports `**` patterns via a custom `matchDoublestar` implementation.

**Exported types**: `AskFunc`, `Search`.
**Constructor**: `New(store *permissions.Store, askFn AskFunc) *Search`.
**Methods**: `Tools() *toolbox.ToolBox`.

### `git` -- Git Operations

Permission-gated tools for git status, diff, log, and commit. Uses command trust (trusting "git") from the shared permissions store. All git commands execute in a configurable working directory. Stdout/stderr are captured with a 1MB cap.

Log format is restricted to built-in git format names (oneline, short, medium, full, fuller, reference, email, raw) to prevent metadata exfiltration via custom format strings. Default is 10 commits in oneline format. The diff tool rejects absolute paths and path traversal (`..`). The commit tool supports staging specific files or all tracked changes (`-a`), with path traversal protection on staged file paths. Commit messages must not start with `-`.

**Exported types**: `AskFunc`, `Git`.
**Constructor**: `New(store *permissions.Store, askFn AskFunc, workDir string) *Git`.
**Methods**: `Tools() *toolbox.ToolBox`.

### `http` -- HTTP Requests

Permission-gated tool for making HTTP requests. Uses domain trust from the shared permissions store. Users can approve a domain once ("yes") for a single request or "trust" it for all future requests. Response bodies are capped at 1MB. Allowed methods: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS. Only http and https schemes are accepted.

Includes SSRF protection: private/loopback IP ranges (127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, ::1/128, fc00::/7, fe80::/10) are blocked at both DNS resolution time and connection time via a custom `safeTransport` with a validating `DialContext`. Redirects to untrusted domains are rejected via `CheckRedirect`. The client has a 60-second overall timeout.

**Exported types**: `AskFunc`, `HTTP`.
**Constructor**: `New(store *permissions.Store, askFn AskFunc) *HTTP`.
**Methods**: `Tools() *toolbox.ToolBox`.

### `notes` -- Persistent Notes

Persistent note-taking tools (`write_note`, `read_note`, `list_notes`) that store Markdown files in a configurable directory (typically `.shelly/notes/`). Notes survive context compaction, allowing agents to save important decisions, progress, and context that can be re-read after a context reset.

Note names are validated against `^[a-zA-Z0-9_-]+$`, preventing path traversal. The `list_notes` tool shows each note with a first-line preview (truncated at 80 characters). The directory is created on first write.

**Exported types**: `Store`.
**Constructor**: `New(dir string) *Store`.
**Methods**: `Tools() *toolbox.ToolBox`.

### `permissions` -- Shared Store

Thread-safe JSON-backed store for approved filesystem directories, trusted commands, and trusted domains. Shared by all permission-gated tool packages. Directory approval checks walk the path hierarchy so approving a parent implicitly approves all children.

Changes are persisted atomically via temp-file-then-rename under a dedicated write mutex. The read path uses `sync.RWMutex` for concurrent reads. Supports both the current object format (`{"fs_directories":[], "trusted_commands":[], "trusted_domains":[]}`) and a legacy flat-array format (JSON array of directory strings) for backward compatibility.

**Exported types**: `Store`.
**Constructor**: `New(filePath string) (*Store, error)`.
**Methods**: `IsDirApproved(dir string) bool`, `ApproveDir(dir string) error`, `IsCommandTrusted(cmd string) bool`, `TrustCommand(cmd string) error`, `IsDomainTrusted(domain string) bool`, `TrustDomain(domain string) error`.

### `defaults` -- Default ToolBox

Merges multiple `*toolbox.ToolBox` instances into a single one. Later entries overwrite earlier ones when tool names collide. Used by `pkg/engine` to compose built-in tools for agents.

**Constructor**: `New(toolboxes ...*toolbox.ToolBox) *toolbox.ToolBox`.

## Use Cases

- **Agent composition**: `pkg/engine` creates instances of each tool sub-package, then uses `defaults.New` to merge them into a single toolbox wired into every agent.
- **Permission gating**: All environment-interacting tools (filesystem, exec, search, git, http, browser) share a single `permissions.Store` so that approvals are consistent across tool categories.
- **User interaction**: The `ask` package provides a blocking question/response mechanism used both as a standalone tool and internally by other packages for permission prompts.
- **Session trust**: The `filesystem` package supports per-session trust so users can approve all file changes in a session without repeated prompts.
- **Context persistence**: The `notes` package gives agents a way to persist information that survives context compaction.
