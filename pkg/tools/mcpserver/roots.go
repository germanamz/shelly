package mcpserver

import (
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerOption configures optional behavior of MCPServer.
type MCPServerOption func(*mcpServerConfig)

type mcpServerConfig struct {
	rootsChangedHandler func([]*mcp.Root)
}

// WithRootsChangedHandler registers a callback that fires when a connected
// client's root list changes. The handler receives the full updated root list.
func WithRootsChangedHandler(fn func(roots []*mcp.Root)) MCPServerOption {
	return func(cfg *mcpServerConfig) {
		cfg.rootsChangedHandler = fn
	}
}

// RootPaths extracts absolute filesystem paths from a slice of MCP roots,
// skipping any roots that don't use the file:// scheme.
func RootPaths(roots []*mcp.Root) []string {
	var paths []string

	for _, r := range roots {
		u, err := url.Parse(r.URI)
		if err != nil || u.Scheme != "file" {
			continue
		}

		paths = append(paths, u.Path)
	}

	return paths
}
