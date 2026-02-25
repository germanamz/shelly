package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient communicates with an MCP server using the official MCP Go SDK.
type MCPClient struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

// New spawns an MCP server process and returns a connected client.
// The SDK handles initialization automatically during Connect.
func New(ctx context.Context, command string, args ...string) (*MCPClient, error) {
	transport := &mcp.CommandTransport{
		Command: exec.Command(command, args...), //nolint:gosec // command is caller-provided by design
	}

	return newFromTransport(ctx, transport)
}

// NewSSE connects to an SSE-based MCP server at the given URL.
func NewSSE(ctx context.Context, url string) (*MCPClient, error) {
	transport := &mcp.SSEClientTransport{Endpoint: url}

	return newFromTransport(ctx, transport)
}

// newFromTransport creates an MCPClient using the given transport. Used by New
// and useful for testing with InMemoryTransport.
func newFromTransport(ctx context.Context, transport mcp.Transport) (*MCPClient, error) {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "shelly",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: connect: %w", err)
	}

	return &MCPClient{client: client, session: session}, nil
}

// ListTools fetches available tools from the server and returns them as
// toolbox.Tool instances. Each Tool's Handler closure calls back through
// CallTool.
func (c *MCPClient) ListTools(ctx context.Context) ([]toolbox.Tool, error) {
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: list tools: %w", err)
	}

	tools := make([]toolbox.Tool, 0, len(result.Tools))
	for _, sdkTool := range result.Tools {
		t, err := fromSDKTool(sdkTool, c)
		if err != nil {
			return nil, fmt.Errorf("mcpclient: convert tool %q: %w", sdkTool.Name, err)
		}
		tools = append(tools, t)
	}

	return tools, nil
}

// CallTool calls a named tool on the server with the given arguments.
func (c *MCPClient) CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	var args map[string]any
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("mcpclient: unmarshal arguments: %w", err)
		}
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("mcpclient: call tool: %w", err)
	}

	text := extractText(result)

	if result.IsError {
		return "", fmt.Errorf("mcpclient: tool error: %s", text)
	}

	return text, nil
}

// Close terminates the session and releases resources. The MCP Go SDK handles
// subprocess lifecycle automatically: session.Close() chains through
// jsonrpc2.Connection.Close() → ioConn.Close() → pipeRWC.Close(), which
// closes stdin, waits with timeout, and escalates through SIGTERM/SIGKILL.
func (c *MCPClient) Close() error {
	return c.session.Close()
}

// fromSDKTool converts an SDK *mcp.Tool to a toolbox.Tool. The handler
// closure calls CallTool on the client.
func fromSDKTool(sdkTool *mcp.Tool, c *MCPClient) (toolbox.Tool, error) {
	schemaBytes, err := json.Marshal(sdkTool.InputSchema)
	if err != nil {
		return toolbox.Tool{}, fmt.Errorf("marshal input schema: %w", err)
	}

	name := sdkTool.Name

	return toolbox.Tool{
		Name:        sdkTool.Name,
		Description: sdkTool.Description,
		InputSchema: json.RawMessage(schemaBytes),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			return c.CallTool(ctx, name, input)
		},
	}, nil
}

// extractText joins all TextContent items from a CallToolResult with newlines.
func extractText(result *mcp.CallToolResult) string {
	var texts []string
	for _, item := range result.Content {
		if tc, ok := item.(*mcp.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}

	return strings.Join(texts, "\n")
}
