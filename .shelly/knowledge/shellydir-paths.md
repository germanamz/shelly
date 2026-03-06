# Shelly Directory Path Management

## Overview

The `pkg/shellydir` package encapsulates all path knowledge for the `.shelly/` project directory. It provides a `Dir` value object with path accessors, plus bootstrap and migration functions. Zero dependencies on other `pkg/` packages.

## Files

```
pkg/shellydir/
├── shellydir.go      # Dir type and all path accessors
├── init.go           # Bootstrap, BootstrapWithConfig, EnsureStructure
├── migrate.go        # MigratePermissions (legacy → local/)
├── shellydir_test.go # Tests
└── README.md
```

---

## Dir Type

```go
type Dir struct {
    root string  // unexported
}

func New(root string) Dir
```

Pure value object. `New` takes the absolute path to the `.shelly/` directory.

### Path Accessors

Every method returns a resolved filepath string. No I/O is performed.

| Method | Returns |
|--------|---------|
| `Root()` | `.shelly/` root path |
| `ConfigPath()` | `.shelly/config.yaml` |
| `ContextPath()` | `.shelly/context.md` |
| `SkillsDir()` | `.shelly/skills/` |
| `KnowledgeDir()` | `.shelly/knowledge/` |
| `LocalDir()` | `.shelly/local/` |
| `PermissionsPath()` | `.shelly/local/permissions.json` |
| `NotesPath()` | `.shelly/local/notes.json` |
| `ReflectionsDir()` | `.shelly/local/reflections/` |
| `SessionsDir()` | `.shelly/local/sessions/` |
| `GitignorePath()` | `.shelly/.gitignore` |
| `DefaultsPath()` | `.shelly/local/defaults.json` |

### Directory Listing Helpers

```go
func (d Dir) KnowledgeFiles() ([]string, error)
```

Returns sorted list of filenames (not full paths) in the knowledge directory. Returns `(nil, nil)` if the directory doesn't exist.

```go
func (d Dir) SkillDirs() ([]string, error)
```

Returns sorted list of directory names in the skills directory. Only includes entries that are directories. Returns `(nil, nil)` if the skills directory doesn't exist.

---

## Bootstrap Functions (init.go)

### Bootstrap

```go
func Bootstrap(d Dir) error
```

Creates the full `.shelly/` structure from scratch with a skeleton `config.yaml`. Calls `BootstrapWithConfig` with a built-in default config (Anthropic provider, single "assistant" agent).

### BootstrapWithConfig

```go
func BootstrapWithConfig(d Dir, config []byte) error
```

Creates the full directory structure:
1. `Root()` directory
2. `SkillsDir()`
3. `KnowledgeDir()`
4. `LocalDir()` and `SessionsDir()` (via `EnsureStructure`)
5. `.gitignore` (ignores `local/`)
6. `config.yaml` (from provided bytes)
7. `context.md` (starter template)

**Existing files are never overwritten** — uses `O_CREATE|O_EXCL` for atomic create-or-skip.

### EnsureStructure

```go
func EnsureStructure(d Dir) error
```

Creates only `local/`, `sessions/`, and `.gitignore` if missing. Does NOT create the root. Idempotent — safe to call on every startup.

---

## Migration (migrate.go)

### MigratePermissions

```go
func MigratePermissions(d Dir) error
```

Moves legacy `permissions.json` from `.shelly/permissions.json` to `.shelly/local/permissions.json`. Idempotent: no-op if old file doesn't exist or new file already exists.

---

## Directory Layout

```
.shelly/
├── config.yaml                    # Engine YAML config
├── context.md                     # Auto-loaded project context
├── .gitignore                     # Ignores local/
├── skills/                        # Custom skill folders
├── knowledge/                     # Knowledge graph markdown files
└── local/                         # Runtime state (gitignored)
    ├── permissions.json           # Tool permission store
    ├── notes.json                 # Agent notes
    ├── defaults.json              # Tool defaults
    ├── reflections/               # Agent reflections
    └── sessions/                  # Session persistence
```

## Key Patterns

- **Value object**: `Dir` is a plain struct with no I/O in accessors — safe to pass by value, use in tests with temp dirs.
- **Idempotent init**: Bootstrap and EnsureStructure are safe to call repeatedly.
- **Gitignored local/**: Everything under `local/` is runtime state not committed to version control.
- **No dependencies**: Zero imports from other `pkg/` packages.
