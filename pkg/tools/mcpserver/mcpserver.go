package mcpserver

import (
	"context"
	"encoding/json"
	"io"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer serves tools over the MCP protocol using the official MCP Go SDK.
type MCPServer struct {
	server *mcp.Server
}

// New creates a new MCPServer with the given name and version.
func New(name, version string) *MCPServer {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    name,
		Version: version,
	}, nil)

	return &MCPServer{server: server}
}

// Register adds tools to the server.
func (s *MCPServer) Register(tools ...toolbox.Tool) {
	for _, t := range tools {
		s.server.AddTool(toSDKTool(t), toSDKHandler(t.Handler))
	}
}

// Serve starts serving MCP requests. It reads requests from in and writes
// responses to out. It blocks until ctx is cancelled or the transport closes.
func (s *MCPServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	transport := &mcp.IOTransport{
		Reader: io.NopCloser(in),
		Writer: nopWriteCloser{out},
	}

	return s.run(ctx, transport)
}

// run starts the server with the given transport. Exported via Serve for
// production use; called directly by tests with InMemoryTransport.
func (s *MCPServer) run(ctx context.Context, transport mcp.Transport) error {
	return s.server.Run(ctx, transport)
}

// toSDKTool converts a toolbox.Tool to an SDK *mcp.Tool.
func toSDKTool(t toolbox.Tool) *mcp.Tool {
	return &mcp.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

// toSDKHandler wraps a toolbox.Handler as an SDK ToolHandler.
func toSDKHandler(h toolbox.Handler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if args == nil {
			args = json.RawMessage("{}")
		}
		result, err := h(ctx, args)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: result}},
		}, nil
	}
}

// nopWriteCloser wraps an io.Writer as an io.WriteCloser with a no-op Close.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
