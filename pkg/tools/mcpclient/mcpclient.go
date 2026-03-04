package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient communicates with an MCP server using the official MCP Go SDK.
type MCPClient struct {
	client   *mcp.Client
	session  *mcp.ClientSession
	cmd      *exec.Cmd // non-nil for stdio transport; used to force-kill subprocess
	done     chan struct{}
	doneOnce sync.Once
}

// New spawns an MCP server process and returns a connected client.
// The SDK handles initialization automatically during Connect. A background
// goroutine ensures the subprocess is killed if the context is cancelled
// without Close being called (e.g., double signal, unclean exit).
func New(ctx context.Context, command string, args ...string) (*MCPClient, error) {
	cmd := exec.Command(command, args...) //nolint:gosec // command is caller-provided by design
	transport := &mcp.CommandTransport{
		Command: cmd,
	}

	c, err := newFromTransport(ctx, transport)
	if err != nil {
		return nil, err
	}

	c.cmd = cmd

	go c.reap(ctx)

	return c, nil
}

// NewHTTP connects to a Streamable HTTP MCP server at the given URL.
func NewHTTP(ctx context.Context, url string) (*MCPClient, error) {
	transport := &mcp.StreamableClientTransport{Endpoint: url}

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

	return &MCPClient{client: client, session: session, done: make(chan struct{})}, nil
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

// reap waits for the context to be cancelled and ensures the MCP subprocess is
// dead. It gives Close a few seconds to perform graceful shutdown via the SDK
// before resorting to SIGKILL. This is a safety net for cases where Close is
// never called (e.g., the process receives a second signal before the defer
// runs).
func (c *MCPClient) reap(ctx context.Context) {
	<-ctx.Done()

	t := time.NewTimer(3 * time.Second)
	defer t.Stop()

	select {
	case <-c.done:
		// Close completed; nothing to do.
	case <-t.C:
		// Close didn't finish in time — force-kill the subprocess.
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
	}
}

// Close terminates the session and releases resources. The MCP Go SDK handles
// subprocess lifecycle automatically: session.Close() chains through
// jsonrpc2.Connection.Close() → ioConn.Close() → pipeRWC.Close(), which
// closes stdin, waits with timeout, and escalates through SIGTERM/SIGKILL.
// As a fallback the subprocess is also killed explicitly.
func (c *MCPClient) Close() error {
	defer c.doneOnce.Do(func() { close(c.done) })

	err := c.session.Close()

	// Ensure the subprocess is dead even if the SDK's teardown didn't kill it.
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}

	return err
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
