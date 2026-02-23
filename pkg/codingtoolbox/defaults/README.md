# defaults

Package `defaults` provides a plug-and-play default toolbox builder. It merges multiple `*toolbox.ToolBox` instances into a single toolbox that agents receive automatically.

## Purpose

Merges multiple toolboxes into one composite toolbox. This package is available for consumers that need to combine toolboxes outside the engine. The engine itself uses per-agent `toolboxes` lists in configuration — each agent declares exactly which toolboxes it needs, and the `ask` toolbox is always included implicitly.

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
