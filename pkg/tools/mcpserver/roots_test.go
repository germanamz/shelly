package mcpserver

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRootsChangedHandler(t *testing.T) {
	var mu sync.Mutex
	var receivedRoots []*mcp.Root

	s := New("test-server", "1.0.0", WithRootsChangedHandler(func(roots []*mcp.Root) {
		mu.Lock()
		receivedRoots = roots
		mu.Unlock()
	}))
	s.Register(newTestTool("echo"))

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

	// Add roots from client side.
	client.AddRoots(&mcp.Root{
		Name: "project",
		URI:  "file:///home/user/project",
	})

	// Wait a bit for the notification to propagate.
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedRoots) > 0
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	assert.Len(t, receivedRoots, 1)
	assert.Equal(t, "file:///home/user/project", receivedRoots[0].URI)
	mu.Unlock()
}

func TestNew_WithoutOptions(t *testing.T) {
	// Ensure backward compatibility: New without options still works.
	s := New("srv", "1.0.0")
	assert.NotNil(t, s.server)
}

func TestNew_WithOptions_ListTools(t *testing.T) {
	// Verify that options don't break tool registration and listing.
	s := New("srv", "1.0.0", WithRootsChangedHandler(func([]*mcp.Root) {}))
	s.Register(newTestTool("echo"))

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

	result, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, result.Tools, 1)
}

func TestRootPaths(t *testing.T) {
	roots := []*mcp.Root{
		{URI: "file:///home/user/projects", Name: "projects"},
		{URI: "file:///tmp/scratch", Name: "scratch"},
		{URI: "https://example.com", Name: "web"},
		{URI: "invalid://foo", Name: "invalid"},
	}

	paths := RootPaths(roots)
	assert.Equal(t, []string{"/home/user/projects", "/tmp/scratch"}, paths)
}

func TestRootPaths_Empty(t *testing.T) {
	assert.Nil(t, RootPaths(nil))
	assert.Nil(t, RootPaths([]*mcp.Root{}))
}

// Verify the setup helper still works (backward compatibility of New).
func TestSetupTestClient_BackwardCompat(t *testing.T) {
	tool := toolbox.Tool{
		Name:        "ping",
		Description: "Ping",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     echoHandler,
	}
	session := setupTestClient(t, tool)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "ping",
		Arguments: map[string]any{"msg": "pong"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
}
