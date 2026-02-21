package modeladapter

import "github.com/germanamz/shelly/pkg/tools/toolbox"

// ToolAware is an optional interface that Completers can implement to receive
// tool declarations. The engine calls SetTools before creating agents so the
// provider knows which tools to declare in API requests.
type ToolAware interface {
	SetTools(tools []toolbox.Tool)
}
