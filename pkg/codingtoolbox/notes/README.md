# notes

Package `notes` provides persistent note-taking tools for agents. Notes are
stored as Markdown files on disk and survive context compaction, allowing agents
to re-read important information after a context reset.

## Architecture

The central type is **`Store`**, which manages a directory of `.md` files. The
directory is created (along with any necessary parents) on the first write
operation. It exposes three tools via `Tools()`:

1. **`write_note`** -- creates or overwrites a note with a given name and Markdown content.
2. **`read_note`** -- reads the content of a previously saved note by name.
3. **`list_notes`** -- lists all available notes with a first-line preview (up to 80 characters). Non-`.md` files and subdirectories are ignored.

### Name Validation

Note names are validated against the regex `^[a-zA-Z0-9_-]+$`, allowing only
alphanumeric characters, hyphens, and underscores. This prevents path traversal,
spaces, dots, and slashes in note names. The `.md` extension is appended
automatically.

## Exported API

### Types

- **`Store`** -- manages persistent notes stored as Markdown files in a directory.

### Functions

- **`New(dir string) *Store`** -- creates a Store that persists notes in the given directory.

### Methods on Store

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox with `write_note`, `read_note`, and `list_notes` tools (3 tools total).

## Tools

| Tool | Description |
|------|-------------|
| `write_note` | Create or overwrite a persistent note. Use descriptive names like `architecture-decisions` or `user_preferences`. |
| `read_note` | Read the content of a previously saved note by name. |
| `list_notes` | List all available notes with a first-line preview of each. Returns "No notes found." if empty. |

## Usage

```go
s := notes.New("/path/to/notes/dir")

// Register the toolbox with an agent.
agent.AddToolBoxes(s.Tools())
```

## Agent Integration

When a notes toolbox is registered with an agent, the agent's system prompt
automatically includes a `<notes_protocol>` section (see `pkg/agent/`). This
lightweight protocol informs the agent that shared notes exist and should be
used for cross-agent communication -- no notes content is preloaded.

## Dependencies

- `pkg/tools/toolbox` -- Tool and ToolBox types
