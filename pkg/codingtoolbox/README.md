# codingtoolbox

Built-in coding tools that give agents controlled access to the local environment. Each sub-package implements a specific tool category with appropriate permission gating.

## Architecture

```
codingtoolbox/
├── ask/           ask_user tool — prompts the user and blocks until a response
├── filesystem/    fs_read, fs_write, fs_edit, fs_list — permission-gated file access
├── exec/          exec_run — permission-gated command execution
├── permissions/   Shared permissions store (approved dirs + trusted commands)
└── defaults/      Default toolbox builder — merges built-in toolboxes into one
```

**Dependency graph**: `permissions` is shared by `filesystem` and `exec`. All tool packages depend on `pkg/tools/toolbox` for the `Tool` and `ToolBox` types. `defaults` merges multiple toolboxes into a single one that every agent receives.

## Sub-packages

### `ask` — User Interaction

Provides the `ask_user` tool and a `Responder` that manages pending questions. The engine hooks up the responder to the event bus so frontends can present questions.

### `filesystem` — File Access

Permission-gated tools for reading, writing, editing, and listing files. Users approve directory access on first touch; approvals persist via the shared permissions store.

### `exec` — Command Execution

Permission-gated tool for running CLI commands. Users can approve once or "trust" a command for all future invocations.

### `permissions` — Shared Store

Thread-safe JSON-backed store for approved filesystem directories and trusted commands. Shared between `filesystem` and `exec`.

### `defaults` — Default ToolBox

Merges all enabled built-in toolboxes into a single `*toolbox.ToolBox` that every agent receives automatically.
