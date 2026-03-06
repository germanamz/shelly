# Shelly Directory Path Management

## Overview

The `pkg/shellydir` package encapsulates all path knowledge for the `.shelly/` project directory. It provides a zero-dependency value object with path accessors for configuration, skills, knowledge, permissions, notes, and local runtime state. This centralized approach ensures consistent directory structure across all Shelly components.

## Core Architecture

### Dir Value Object
```go
type Dir struct {
    root string  // Path to .shelly/ directory
}
```

A lightweight, immutable value object that resolves all paths within a Shelly project directory. The zero value is not useful; use constructors to create valid instances.

### Construction Methods
```go
func New(root string) Dir                    // Create from explicit .shelly/ path
func Find(startPath string) (Dir, error)     // Search upward from startPath
func FindFromCwd() (Dir, error)             // Search upward from current directory
```

## Directory Structure

The `.shelly/` directory follows a standardized layout:

```
.shelly/
├── config.yaml          # Engine configuration
├── context.md           # Project context file  
├── skills/              # Custom agent skills
│   └── <skill-name>/
│       ├── skill.md     # Skill implementation
│       └── examples/    # Usage examples
├── knowledge/           # Curated project knowledge
│   └── *.md            # Knowledge files (this indexing output)
└── local/              # Runtime state (gitignored)
    ├── permissions/     # Tool permission grants
    ├── notes/          # Agent-generated notes
    ├── reflections/    # Learning and insights
    └── state/          # Key-value state store
```

## Path Accessors

### Configuration Paths
```go
func (d Dir) Config() string         // .shelly/config.yaml
func (d Dir) Context() string        // .shelly/context.md
```

**Usage**: Engine configuration and project context loading

### Content Directories
```go
func (d Dir) Skills() string         // .shelly/skills/
func (d Dir) Knowledge() string      // .shelly/knowledge/
```

**Usage**: Skill loading and knowledge graph management

### Runtime State Paths
```go
func (d Dir) Local() string          // .shelly/local/
func (d Dir) Permissions() string    // .shelly/local/permissions/
func (d Dir) Notes() string         // .shelly/local/notes/
func (d Dir) Reflections() string   // .shelly/local/reflections/
func (d Dir) State() string         // .shelly/local/state/
```

**Usage**: Runtime data that should not be version controlled

### Utility Methods
```go
func (d Dir) Root() string           // .shelly/ directory path
func (d Dir) Exists() bool          // Check if .shelly/ directory exists
func (d Dir) IsEmpty() bool         // Check if directory contains no Shelly files
```

## Directory Discovery

### Upward Search Algorithm
The `Find` methods implement Git-like directory traversal:

1. Start from given path (or current directory)
2. Check if `.shelly/` exists in current directory
3. If found, return `Dir` instance
4. If not found, move to parent directory
5. Repeat until filesystem root or `.shelly/` found
6. Return error if no `.shelly/` directory found in tree

```go
// Example: Find .shelly/ from anywhere in project
dir, err := shellydir.FindFromCwd()
if err != nil {
    log.Fatal("No .shelly/ directory found in project tree")
}

configPath := dir.Config()  // Absolute path to config.yaml
```

### Project Root Detection
```go
func (d Dir) ProjectRoot() string
```

Returns the parent directory containing `.shelly/` - the actual project root directory.

## Initialization and Migration

### Directory Structure Creation
```go
func (d Dir) Init() error
```

Creates the complete `.shelly/` directory structure with proper permissions:
- Creates all required subdirectories
- Sets up `.gitignore` for `local/` directory
- Creates default `config.yaml` if missing
- Ensures proper directory permissions

### Migration Support
```go
func (d Dir) Migrate() error
```

Handles upgrades of existing `.shelly/` directories:
- Adds missing directories for new Shelly versions
- Updates `.gitignore` patterns as needed
- Preserves existing configuration and content
- Non-destructive operations only

### Validation
```go
func (d Dir) Validate() error
```

Checks directory structure integrity:
- Verifies all required directories exist
- Validates file permissions
- Checks for configuration file presence
- Reports specific issues for repair

## Usage Patterns

### Engine Initialization
```go
func NewEngine(configPath string) (*Engine, error) {
    var dir shellydir.Dir
    var err error
    
    if configPath != "" {
        dir = shellydir.New(filepath.Dir(configPath))
    } else {
        dir, err = shellydir.FindFromCwd()
        if err != nil {
            return nil, fmt.Errorf("no .shelly directory found: %w", err)
        }
    }
    
    config, err := loadConfig(dir.Config())
    // ... engine setup
}
```

### Skill Loading
```go
func LoadSkills(dir shellydir.Dir) (map[string]*Skill, error) {
    skillsDir := dir.Skills()
    entries, err := os.ReadDir(skillsDir)
    
    skills := make(map[string]*Skill)
    for _, entry := range entries {
        if entry.IsDir() {
            skillPath := filepath.Join(skillsDir, entry.Name())
            skill, err := LoadSkill(skillPath)
            if err == nil {
                skills[entry.Name()] = skill
            }
        }
    }
    return skills, nil
}
```

### State Management
```go
func NewStateStore(dir shellydir.Dir) *StateStore {
    return &StateStore{
        dataDir: dir.State(),
        // ... initialization
    }
}
```

### Permission System
```go
func (p *PermissionManager) LoadGrants(dir shellydir.Dir) error {
    permDir := dir.Permissions()
    files, err := filepath.Glob(filepath.Join(permDir, "*.json"))
    
    for _, file := range files {
        grant, err := loadPermissionGrant(file)
        if err == nil {
            p.grants[grant.Tool] = grant
        }
    }
    return nil
}
```

## Design Principles

### Zero Dependencies
- No imports from other `pkg/` packages
- Only standard library dependencies (`os`, `path/filepath`)
- Foundation layer that others build upon

### Immutable Value Object
- `Dir` instances are immutable after creation
- Path accessors return strings, not mutable references
- Safe to pass by value and share across goroutines

### Centralized Path Knowledge
- Single source of truth for all `.shelly/` paths
- Consistent directory structure across all components
- Easy to update paths when structure evolves

### Filesystem Abstraction
- Provides logical path structure independent of OS
- Handles cross-platform path separators correctly
- Enables testing with custom directory structures

## Error Handling

### Discovery Errors
```go
var ErrNotFound = errors.New("no .shelly directory found")
```

Returned when upward search fails to find a `.shelly/` directory in the filesystem tree.

### Initialization Errors
- Permission denied creating directories
- Disk space exhaustion during setup
- Configuration file syntax errors

### Migration Errors  
- Incompatible directory structure versions
- File permission issues during updates
- Corrupted configuration or state files

## Integration Points

The shellydir package provides path services to:

- **pkg/engine**: Configuration file loading and directory setup
- **pkg/skill**: Skill discovery and loading from `skills/` directory  
- **pkg/projectctx**: Knowledge file management in `knowledge/` directory
- **pkg/state**: State persistence in `local/state/` directory
- **pkg/codingtoolbox**: Permission grants in `local/permissions/` directory
- **CLI**: Project discovery and initialization commands

## Thread Safety

All operations in shellydir are thread-safe:
- `Dir` is an immutable value object
- Path methods perform no I/O, only string manipulation
- Filesystem operations (`Init`, `Migrate`) use standard library calls
- No shared mutable state between instances

## Performance Considerations

- Path resolution is O(1) string concatenation
- Directory discovery is O(tree depth) but cached after first use
- No unnecessary filesystem access in path accessor methods
- Minimal memory footprint per `Dir` instance

This package forms the foundation for all filesystem interactions in Shelly, ensuring consistent project structure and enabling reliable path resolution across all system components.