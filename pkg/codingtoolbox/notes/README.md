# notes

Package `notes` provides persistent note-taking tools for agents. Notes are
stored as Markdown files on disk and survive context compaction, allowing agents
to re-read important information after a context reset.

## Architecture

The central type is **Store**, which manages a directory of `.md` files. It
exposes three tools via `Tools()`:

1. **write_note** -- creates or overwrites a note with a given name and content.
2. **read_note** -- reads the content of a previously saved note by name.
3. **list_notes** -- lists all available notes with a first-line preview.

Note names are sanitized to allow only alphanumeric characters, hyphens, and
underscores, preventing path traversal or other filesystem issues.

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
used for cross-agent communication — no notes content is preloaded.

## Dependencies

- `pkg/tools/toolbox` -- Tool and ToolBox types
