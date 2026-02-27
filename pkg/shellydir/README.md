# shellydir

Package `shellydir` encapsulates all path knowledge for the `.shelly/` project directory.

## Purpose

The `.shelly/` directory is the single source of truth for a Shelly instance running in a project. This package provides:

- **`Dir`** -- a zero-dependency value object with path accessors for config, context, skills, permissions, notes, reflections, and local runtime state.
- **`Bootstrap`** / **`BootstrapWithConfig`** -- creates a `.shelly/` directory from scratch with the full initial structure and a skeleton (or custom) config.
- **`EnsureStructure`** -- creates the `local/` directory and `.gitignore` if missing (idempotent).
- **`MigratePermissions`** -- moves the legacy `permissions.json` from `.shelly/` to `.shelly/local/` (idempotent).

## Directory Layout

```
.shelly/
  .gitignore            # contains "local/"
  config.yaml           # main config (committed)
  context.md            # curated project instructions (committed)
  skills/               # skill folders (committed)
    code-review/
      SKILL.md
  local/                # gitignored runtime state
    permissions.json    # permission grants
    context-cache.json  # auto-generated project index
    notes/              # agent notes
    reflections/        # agent reflections
```

## Exported Types

### Dir

```go
type Dir struct { /* unexported fields */ }
```

A value object that resolves paths within a `.shelly/` directory. Created with `New()`. No I/O is performed at construction time; the path is converted to absolute form.

#### Path Accessors

| Method | Returns |
|--------|---------|
| `Root()` | `.shelly/` |
| `ConfigPath()` | `.shelly/config.yaml` |
| `ContextPath()` | `.shelly/context.md` |
| `SkillsDir()` | `.shelly/skills` |
| `LocalDir()` | `.shelly/local` |
| `PermissionsPath()` | `.shelly/local/permissions.json` |
| `ContextCachePath()` | `.shelly/local/context-cache.json` |
| `NotesDir()` | `.shelly/local/notes` |
| `ReflectionsDir()` | `.shelly/local/reflections` |
| `GitignorePath()` | `.shelly/.gitignore` |

#### Other Methods

- **`ContextFiles()`** -- returns sorted paths of all `*.md` files in the `.shelly/` root directory (non-recursive). Returns `nil` if the directory does not exist or contains no `.md` files.
- **`Exists()`** -- reports whether the `.shelly/` root directory exists on disk.

## Exported Functions

### New

```go
func New(root string) Dir
```

Creates a `Dir` rooted at the given path. The path is converted to an absolute path. No I/O is performed.

### Bootstrap

```go
func Bootstrap(d Dir) error
```

Creates the `.shelly/` directory from scratch with a full initial structure: root, `skills/`, `local/`, `.gitignore`, and a skeleton `config.yaml`. Existing files are never overwritten, making it safe to run on an already-initialized directory.

### BootstrapWithConfig

```go
func BootstrapWithConfig(d Dir, config []byte) error
```

Same as `Bootstrap` but uses the provided config content instead of the skeleton default.

### EnsureStructure

```go
func EnsureStructure(d Dir) error
```

Creates the `local/` directory and `.gitignore` file if they are missing. Idempotent -- safe to call multiple times. Does NOT create the `.shelly/` root itself.

### MigratePermissions

```go
func MigratePermissions(d Dir) error
```

Moves the legacy `permissions.json` from `.shelly/permissions.json` to `.shelly/local/permissions.json`. The operation is idempotent: it is a no-op if the old file does not exist or the new file already exists. If both files exist, the new file is preserved and the old file is left in place.

## Usage

```go
d := shellydir.New(".shelly")

// Bootstrap a new project.
shellydir.Bootstrap(d)

// Or set up an existing directory.
if d.Exists() {
    shellydir.EnsureStructure(d)
    shellydir.MigratePermissions(d)
}

cfg := d.ConfigPath()        // ".shelly/config.yaml"
perms := d.PermissionsPath() // ".shelly/local/permissions.json"
ctxFiles := d.ContextFiles() // all *.md files in .shelly/
```

## Dependencies

Zero dependencies on other `pkg/` packages.
