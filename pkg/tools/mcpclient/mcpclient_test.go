package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServer creates an MCP server with the given tools, connects a
// client via in-memory transports, and returns the client. The server runs
// in a background goroutine tied to t.Cleanup.
func setupTestServer(t *testing.T, tools ...toolbox.Tool) *MCPClient {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)

	for _, tool := range tools {
		handler := tool.Handler
		server.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := handler(ctx, req.Params.Arguments)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
					IsError: true,
				}, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: result}},
			}, nil
		})
	}

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

	return client
}

func echoHandler(_ context.Context, input json.RawMessage) (string, error) {
	return string(input), nil
}

func TestListTools(t *testing.T) {
	client := setupTestServer(t,
		toolbox.Tool{
			Name:        "search",
			Description: "Search the web",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
			Handler:     echoHandler,
		},
		toolbox.Tool{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler:     echoHandler,
		},
	)

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 2)

	toolsByName := make(map[string]toolbox.Tool, len(tools))
	for _, tool := range tools {
		toolsByName[tool.Name] = tool
	}

	search, ok := toolsByName["search"]
	require.True(t, ok)
	assert.Equal(t, "Search the web", search.Description)
	assert.NotNil(t, search.Handler)

	readFile, ok := toolsByName["read_file"]
	require.True(t, ok)
	assert.Equal(t, "Read a file", readFile.Description)
	assert.NotNil(t, readFile.Handler)
}

func TestCallToolSuccess(t *testing.T) {
	client := setupTestServer(t, toolbox.Tool{
		Name:        "echo",
		Description: "Echo input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     echoHandler,
	})

	text, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"msg":"hello"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"msg":"hello"}`, text)
}

func TestCallToolError(t *testing.T) {
	client := setupTestServer(t, toolbox.Tool{
		Name:        "fail",
		Description: "Always fails",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "", errors.New("something went wrong")
		},
	})

	text, err := client.CallTool(context.Background(), "fail", json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "something went wrong")
	assert.Empty(t, text)
}

func TestCallToolMultipleContent(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)

	server.AddTool(&mcp.Tool{
		Name:        "multi",
		Description: "Returns multiple content items",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "line 1"},
				&mcp.TextContent{Text: "line 2"},
			},
		}, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, serverTransport)
	}()
	defer func() {
		cancel()
		<-serverDone
	}()

	client, err := newFromTransport(ctx, clientTransport)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	text, err := client.CallTool(context.Background(), "multi", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "line 1\nline 2", text)
}

func TestListToolsHandlerRoundTrip(t *testing.T) {
	client := setupTestServer(t, toolbox.Tool{
		Name:        "greet",
		Description: "Say hello",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return "hello world", nil
		},
	})

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)

	result, err := tools[0].Handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

func TestNewSSE_InvalidEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := NewSSE(ctx, "http://127.0.0.1:1/invalid")
	assert.Error(t, err, "NewSSE should fail for unreachable endpoint")
}

func TestClose(t *testing.T) {
	client := setupTestServer(t, toolbox.Tool{
		Name:        "noop",
		Description: "Does nothing",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler:     echoHandler,
	})

	err := client.Close()
	assert.NoError(t, err)
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name   string
		result *mcp.CallToolResult
		want   string
	}{
		{
			name: "single text",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "hello"}},
			},
			want: "hello",
		},
		{
			name: "multiple text",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "a"},
					&mcp.TextContent{Text: "b"},
				},
			},
			want: "a\nb",
		},
		{
			name: "empty content",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractText(tt.result))
		})
	}
}

func TestFromSDKTool(t *testing.T) {
	sdkTool := &mcp.Tool{
		Name:        "test",
		Description: "A test tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}

	client := &MCPClient{}
	tool, err := fromSDKTool(sdkTool, client)
	require.NoError(t, err)
	assert.Equal(t, "test", tool.Name)
	assert.Equal(t, "A test tool", tool.Description)
	assert.NotNil(t, tool.InputSchema)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
	assert.Equal(t, "object", schema["type"])
}
