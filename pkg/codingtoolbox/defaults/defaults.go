// Package defaults provides a plug-and-play default toolbox builder. It
// composes multiple toolboxes into a single one that agents receive
// automatically.
package defaults

import "github.com/germanamz/shelly/pkg/tools/toolbox"

// New builds a default toolbox by merging the given toolboxes together. Each
// toolbox is merged in order so later entries overwrite earlier ones when tool
// names collide.
func New(toolboxes ...*toolbox.ToolBox) *toolbox.ToolBox {
	tb := toolbox.New()
	for _, other := range toolboxes {
		tb.Merge(other)
	}

	return tb
}
