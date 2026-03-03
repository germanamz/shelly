package effects

import (
	"context"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// ToolScopeConfig holds parameters for the ToolScopeEffect.
type ToolScopeConfig struct {
	Exclude []string // Tool names to exclude (blacklist).
}

// ToolScopeEffect filters which tools the LLM sees on each iteration by
// excluding named tools. It implements both agent.Effect (no-op Eval) and
// agent.ToolFilter.
type ToolScopeEffect struct {
	exclude map[string]struct{}
}

// NewToolScopeEffect creates a ToolScopeEffect from the given configuration.
func NewToolScopeEffect(cfg ToolScopeConfig) *ToolScopeEffect {
	exclude := make(map[string]struct{}, len(cfg.Exclude))
	for _, name := range cfg.Exclude {
		exclude[name] = struct{}{}
	}
	return &ToolScopeEffect{exclude: exclude}
}

// Eval implements agent.Effect. ToolScopeEffect is a no-op during evaluation;
// all work happens in FilterTools.
func (e *ToolScopeEffect) Eval(_ context.Context, _ agent.IterationContext) error {
	return nil
}

// FilterTools implements agent.ToolFilter, removing tools whose names are in
// the exclude set.
func (e *ToolScopeEffect) FilterTools(_ context.Context, _ agent.IterationContext, tools []toolbox.Tool) []toolbox.Tool {
	if len(e.exclude) == 0 {
		return tools
	}

	filtered := make([]toolbox.Tool, 0, len(tools))
	for _, t := range tools {
		if _, excluded := e.exclude[t.Name]; !excluded {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
