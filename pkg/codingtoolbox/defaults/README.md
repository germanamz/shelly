# defaults

Package `defaults` provides a plug-and-play default toolbox builder. It merges multiple `*toolbox.ToolBox` instances into a single toolbox that agents receive automatically.

## Purpose

Instead of requiring every agent to list built-in toolboxes (ask, filesystem, state) individually, the engine builds a single defaults toolbox from all enabled built-in tools and injects it into every agent. Additional toolboxes (e.g. MCP servers) can still be added per-agent via `toolbox_names` in config.

## Usage

```go
import "github.com/germanamz/shelly/pkg/codingtoolbox/defaults"

// Merge any number of toolboxes into one.
tb := defaults.New(askToolbox, filesystemToolbox, stateToolbox)
```

Later entries overwrite earlier ones when tool names collide.

## Architecture

- Depends only on `pkg/tools/toolbox`
- Used by `pkg/engine` to compose built-in tools
- Stateless — the returned toolbox owns the merged tools
