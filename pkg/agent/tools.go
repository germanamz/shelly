package agent

import "github.com/germanamz/shelly/pkg/tools/toolbox"

// deduplicateTools collects tool declarations from all toolboxes,
// deduplicating by name so providers that reject duplicate definitions
// (e.g. Grok) don't fail when parent toolboxes are injected into children.
func deduplicateTools(toolboxes []*toolbox.ToolBox) []toolbox.Tool {
	seen := make(map[string]struct{})
	var tools []toolbox.Tool

	for _, tb := range toolboxes {
		for _, t := range tb.Tools() {
			if _, dup := seen[t.Name]; dup {
				continue
			}
			seen[t.Name] = struct{}{}
			tools = append(tools, t)
		}
	}

	return tools
}
