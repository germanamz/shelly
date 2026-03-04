package toolbox

// ToolBox orchestrates a collection of tools. It allows registering, retrieving,
// listing, and filtering tools. Tools are stored in insertion order.
type ToolBox struct {
	index map[string]int // name → position in items
	items []Tool         // insertion-ordered
}

// New creates a new ToolBox ready for use.
func New() *ToolBox {
	return &ToolBox{
		index: make(map[string]int),
	}
}

// Register adds one or more tools to the ToolBox. If a tool with the same name
// already exists, it is replaced in-place (preserving position).
func (tb *ToolBox) Register(tools ...Tool) {
	for _, t := range tools {
		if idx, ok := tb.index[t.Name]; ok {
			tb.items[idx] = t
			continue
		}
		tb.index[t.Name] = len(tb.items)
		tb.items = append(tb.items, t)
	}
}

// Get returns a tool by name and a boolean indicating whether it was found.
func (tb *ToolBox) Get(name string) (Tool, bool) {
	idx, ok := tb.index[name]
	if !ok {
		return Tool{}, false
	}
	return tb.items[idx], true
}

// Merge registers all tools from another ToolBox into this one, preserving the
// other's insertion order. If a tool with the same name already exists, it is
// replaced in-place.
func (tb *ToolBox) Merge(other *ToolBox) {
	for _, t := range other.items {
		tb.Register(t)
	}
}

// Tools returns all registered tools as a slice in insertion order.
func (tb *ToolBox) Tools() []Tool {
	result := make([]Tool, len(tb.items))
	copy(result, tb.items)
	return result
}

// Len returns the number of registered tools.
func (tb *ToolBox) Len() int { return len(tb.items) }

// Filter returns a new ToolBox containing only the tools whose names appear in
// the provided list, in the order given. Unknown names are silently skipped.
// A nil slice means "no filter" and returns the original ToolBox unchanged.
// An explicit empty slice means "filter to nothing" and returns an empty ToolBox.
func (tb *ToolBox) Filter(names []string) *ToolBox {
	if names == nil {
		return tb
	}
	if len(names) == 0 {
		return New()
	}
	filtered := New()
	for _, name := range names {
		if t, ok := tb.Get(name); ok {
			filtered.Register(t)
		}
	}
	return filtered
}
