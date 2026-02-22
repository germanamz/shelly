# shellydir

Package `shellydir` encapsulates all path knowledge for the `.shelly/` project directory.

## Purpose

The `.shelly/` directory is the single source of truth for a Shelly instance running in a project. This package provides:

- **`Dir`** — a zero-dependency value object with path accessors for config, context, skills, permissions, and local runtime state.
- **`EnsureStructure`** — creates the `local/` directory and `.gitignore` if missing (idempotent).
- **`MigratePermissions`** — moves the legacy `permissions.json` from `.shelly/` to `.shelly/local/` (idempotent).

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
```

## Usage

```go
d := shellydir.New(".shelly")

if d.Exists() {
    shellydir.EnsureStructure(d)
    shellydir.MigratePermissions(d)
}

cfg := d.ConfigPath()       // ".shelly/config.yaml"
perms := d.PermissionsPath() // ".shelly/local/permissions.json"
ctxFiles := d.ContextFiles() // all *.md files in .shelly/
```

## Dependencies

Zero dependencies on other `pkg/` packages.
