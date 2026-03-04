package agent

import "github.com/germanamz/shelly/pkg/tools/toolbox"

// deduplicateTools collects tool declarations from all toolboxes,
// deduplicating by name so providers that reject duplicate definitions
// (e.g. Grok) don't fail when parent toolboxes are injected into children.
// It also returns a handler map for O(1) tool dispatch in callTool.
// First-toolbox-wins semantics are preserved: the first handler registered
// for a given name is the one used.
func deduplicateTools(toolboxes []*toolbox.ToolBox) ([]toolbox.Tool, map[string]toolbox.Handler) {
	handlers := make(map[string]toolbox.Handler)
	var tools []toolbox.Tool

	for _, tb := range toolboxes {
		for _, t := range tb.Tools() {
			if _, dup := handlers[t.Name]; dup {
				continue
			}
			handlers[t.Name] = t.Handler
			tools = append(tools, t)
		}
	}

	return tools, handlers
}
