# codingtoolbox

Built-in coding tools that give agents controlled access to the local environment. Each sub-package implements a specific tool category with appropriate permission gating.

## Architecture

```
codingtoolbox/
├── ask/           ask_user tool — prompts the user and blocks until a response
├── filesystem/    fs_read, fs_write, fs_edit, fs_list, fs_delete, fs_move, fs_copy, fs_stat, fs_diff, fs_patch
├── exec/          exec_run — permission-gated command execution
├── search/        search_content, search_files — permission-gated content/file search
├── git/           git_status, git_diff, git_log, git_commit — permission-gated git ops
├── http/          http_fetch — permission-gated HTTP requests
├── permissions/   Shared permissions store (approved dirs, trusted commands, trusted domains)
└── defaults/      Default toolbox builder — merges built-in toolboxes into one
```

**Dependency graph**: `permissions` is shared by `filesystem`, `exec`, `search`, `git`, and `http`. All tool packages depend on `pkg/tools/toolbox` for the `Tool` and `ToolBox` types. `defaults` merges multiple toolboxes into a single one that every agent receives.

## Sub-packages

### `ask` — User Interaction

Provides the `ask_user` tool and a `Responder` that manages pending questions. The engine hooks up the responder to the event bus so frontends can present questions.

### `filesystem` — File Access

Permission-gated tools for reading, writing, editing, listing, deleting, moving, copying, stat-ing, diffing, and patching files. Users approve directory access on first touch; approvals persist via the shared permissions store.

### `exec` — Command Execution

Permission-gated tool for running CLI commands. Users can approve once or "trust" a command for all future invocations.

### `search` — Content & File Search

Permission-gated tools for searching file contents by regex and finding files by glob pattern (with `**` support). Uses directory approval from the shared permissions store.

### `git` — Git Operations

Permission-gated tools for git status, diff, log, and commit. Uses command trust (trusting "git") from the shared permissions store.

### `http` — HTTP Requests

Permission-gated tool for making HTTP requests. Uses domain trust from the shared permissions store. Response bodies are capped at 1MB.

### `permissions` — Shared Store

Thread-safe JSON-backed store for approved filesystem directories, trusted commands, and trusted domains. Shared by all permission-gated tool packages.

### `defaults` — Default ToolBox

Merges all enabled built-in toolboxes into a single `*toolbox.ToolBox` that every agent receives automatically.
