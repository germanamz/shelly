package mcpclient

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddRoots_ListRoots(t *testing.T) {
	// Create a server that can list roots from client.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		RootsListChangedHandler: func(context.Context, *mcp.RootsListChangedRequest) {},
	})

	// Register a tool that lists roots via the server session.
	server.AddTool(&mcp.Tool{
		Name:        "list_roots",
		Description: "List client roots",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := req.Session.ListRoots(ctx, nil)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil
		}
		data, _ := json.Marshal(res.Roots)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, serverTransport)
	}()
	t.Cleanup(func() {
		cancel()
		<-serverDone
	})

	client, err := newFromTransport(ctx, clientTransport)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	// Add roots.
	client.AddRoots(
		DirToRoot("/home/user/projects"),
		DirToRoot("/tmp/scratch"),
	)

	// Call the tool to verify roots are visible from server side.
	text, err := client.CallTool(ctx, "list_roots", json.RawMessage(`{}`))
	require.NoError(t, err)

	var roots []*mcp.Root
	require.NoError(t, json.Unmarshal([]byte(text), &roots))
	assert.Len(t, roots, 2)

	uris := make([]string, len(roots))
	for i, r := range roots {
		uris[i] = r.URI
	}
	assert.Contains(t, uris, "file:///home/user/projects")
	assert.Contains(t, uris, "file:///tmp/scratch")
}

func TestRemoveRoots(t *testing.T) {
	client := setupTestServer(t, toolbox.Tool{
		Name:        "noop",
		Description: "noop",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     echoHandler,
	})

	// AddRoots then RemoveRoots should not panic.
	client.AddRoots(DirToRoot("/tmp"))
	client.RemoveRoots("file:///tmp")
}

func TestDirToRoot(t *testing.T) {
	root := DirToRoot("/home/user/projects")
	assert.Equal(t, "file:///home/user/projects", root.URI)
	assert.Equal(t, "/home/user/projects", root.Name)
}
