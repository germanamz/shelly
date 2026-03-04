package mcpclient

import (
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddRoots adds roots to the client's root list and notifies connected servers.
func (c *Client) AddRoots(roots ...*mcp.Root) {
	c.client.AddRoots(roots...)
}

// RemoveRoots removes roots by URI and notifies connected servers.
func (c *Client) RemoveRoots(uris ...string) {
	c.client.RemoveRoots(uris...)
}

// DirToRoot converts an absolute directory path to an MCP Root with a file:// URI.
func DirToRoot(dir string) *mcp.Root {
	u := &url.URL{Scheme: "file", Path: dir}
	return &mcp.Root{
		Name: dir,
		URI:  u.String(),
	}
}
