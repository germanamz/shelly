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

### `ask` — User Interaction

Provides the `ask_user` tool and a `Responder` that manages pending questions. Supports both free-form and multiple-choice questions, with an optional `MultiSelect` mode. The `Ask` method can also be called programmatically by other packages (e.g. `filesystem`, `exec`) to prompt the user without going through the JSON tool handler layer. The engine hooks up the responder to the event bus so frontends can present questions.

### `browser` — Browser Automation

Permission-gated tools for headless Chrome automation via chromedp. Provides web search (DuckDuckGo), page navigation with text extraction, element clicking, text input, content extraction by CSS selector, and screenshots (viewport, full page, or element). Chrome is started lazily on first tool use and runs in incognito mode. Domain trust is shared with the `http` package.

### `filesystem` — File Access

Permission-gated tools for reading, writing, editing, listing, deleting, moving, copying, stat-ing, diffing, patching files, and creating directories. Users approve directory access on first touch; approvals persist via the shared permissions store. File-modifying operations show a unified diff and require user confirmation (or session trust). Concurrent writes to the same file are serialized via a per-path `FileLocker`.

### `exec` — Command Execution

Permission-gated tool for running CLI commands. Users can approve once or "trust" a command for all future invocations. Commands are executed directly via `os/exec` (no shell interpretation). Stdout/stderr are captured with a 1MB cap. Concurrent permission prompts for the same command are coalesced.

### `search` — Content & File Search

Permission-gated tools for searching file contents by regex and finding files by glob pattern (with `**` support). Uses directory approval from the shared permissions store. Content search skips binary files (UTF-8 validity check) and caps total matched content at 1MB.

### `git` — Git Operations

Permission-gated tools for git status, diff, log, and commit. Uses command trust (trusting "git") from the shared permissions store. Log format is restricted to built-in git formats (oneline, short, medium, full, fuller, reference, email, raw) to prevent metadata exfiltration. Commit tool supports staging specific files or all tracked changes, with path traversal protection.

### `http` — HTTP Requests

Permission-gated tool for making HTTP requests. Uses domain trust from the shared permissions store. Response bodies are capped at 1MB. Includes SSRF protection: private/loopback IP ranges are blocked at both DNS resolution time and connection time (via a custom transport), and redirects to untrusted domains or private addresses are rejected.

### `notes` — Persistent Notes

Persistent note-taking tools (`write_note`, `read_note`, `list_notes`) that store Markdown files in `.shelly/notes/`. Notes survive context compaction, allowing agents to save important decisions, progress, and context that can be re-read after a context reset. Note names are sanitized to allow only alphanumeric characters, hyphens, and underscores, preventing path traversal.

### `permissions` — Shared Store

Thread-safe JSON-backed store for approved filesystem directories, trusted commands, and trusted domains. Shared by all permission-gated tool packages. Changes are persisted atomically via temp-file-then-rename. Supports both the current object format and a legacy flat-array format for backward compatibility.

### `defaults` — Default ToolBox

Merges multiple `*toolbox.ToolBox` instances into a single one. Later entries overwrite earlier ones when tool names collide. Used by `pkg/engine` to compose built-in tools for agents.
