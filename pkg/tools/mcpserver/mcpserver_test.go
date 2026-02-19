package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func echoHandler(_ context.Context, input json.RawMessage) (string, error) {
	return string(input), nil
}

func errorHandler(_ context.Context, _ json.RawMessage) (string, error) {
	return "", errors.New("tool failed")
}

func newTestTool(name string) toolbox.Tool {
	return toolbox.Tool{
		Name:        name,
		Description: "Test tool: " + name,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     echoHandler,
	}
}

// setupTestClient creates an MCPServer, connects an SDK client via in-memory
// transports, and returns the client session. The server runs in a background
// goroutine tied to t.Cleanup.
func setupTestClient(t *testing.T, tools ...toolbox.Tool) *mcp.ClientSession {
	t.Helper()

	s := New("test-server", "1.0.0")
	s.Register(tools...)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- s.run(ctx, serverTransport)
	}()
	t.Cleanup(func() {
		cancel()
		<-serverDone
	})

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	return session
}

func TestNew(t *testing.T) {
	s := New("srv", "1.0.0")
	assert.NotNil(t, s.server)
}

func TestRegister(t *testing.T) {
	s := New("srv", "1.0.0")
	s.Register(newTestTool("a"), newTestTool("b"))
	// Verify tools are registered by running a client and listing them.
	// (No internal map to inspect — the SDK owns tool storage.)
}

func TestListTools(t *testing.T) {
	session := setupTestClient(t,
		newTestTool("echo"),
		toolbox.Tool{
			Name:        "greet",
			Description: "Say hello",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
			Handler:     echoHandler,
		},
	)

	result, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Tools, 2)

	toolsByName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		toolsByName[tool.Name] = tool
	}

	echo, ok := toolsByName["echo"]
	require.True(t, ok)
	assert.Equal(t, "Test tool: echo", echo.Description)

	greet, ok := toolsByName["greet"]
	require.True(t, ok)
	assert.Equal(t, "Say hello", greet.Description)
}

func TestToolCallSuccess(t *testing.T) {
	session := setupTestClient(t, newTestTool("echo"))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"msg": "hello"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.JSONEq(t, `{"msg":"hello"}`, tc.Text)
}

func TestToolCallHandlerError(t *testing.T) {
	session := setupTestClient(t, toolbox.Tool{
		Name:        "fail",
		Description: "Always fails",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     errorHandler,
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "fail",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	require.Len(t, result.Content, 1)

	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, "tool failed", tc.Text)
}

func TestToolCallNotFound(t *testing.T) {
	session := setupTestClient(t)

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "missing",
		Arguments: map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestContextCancellation(t *testing.T) {
	s := New("srv", "1.0.0")
	serverTransport, _ := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.run(ctx, serverTransport)
	assert.ErrorIs(t, err, context.Canceled)
}
