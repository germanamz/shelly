package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/mcpclient"
	"github.com/germanamz/shelly/pkg/tools/toolbox"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpResult holds the outcome of a single parallel MCP connection attempt.
type mcpResult struct {
	name   string
	client *mcpclient.MCPClient
	tb     *toolbox.ToolBox
	err    error
}

// connectMCPClients connects to all configured MCP servers in parallel and
// populates toolboxes. On any error, successfully-connected clients are closed.
func (e *Engine) connectMCPClients(ctx context.Context, servers []MCPConfig, status func(string)) error {
	if len(servers) == 0 {
		return nil
	}

	results := make([]mcpResult, len(servers))

	var wg sync.WaitGroup
	for i, mc := range servers {
		wg.Go(func() {
			status(fmt.Sprintf("Connecting MCP server %q...", mc.Name))
			start := time.Now()

			var client *mcpclient.MCPClient
			var err error
			if mc.URL != "" {
				client, err = mcpclient.NewHTTP(ctx, mc.URL)
			} else {
				client, err = mcpclient.New(ctx, mc.Command, mc.Args...)
			}
			if err != nil {
				results[i] = mcpResult{name: mc.Name, err: fmt.Errorf("engine: mcp %q: %w", mc.Name, err)}
				return
			}

			tools, err := client.ListTools(ctx)
			if err != nil {
				_ = client.Close()
				results[i] = mcpResult{name: mc.Name, err: fmt.Errorf("engine: mcp %q: list tools: %w", mc.Name, err)}
				return
			}

			tb := toolbox.New()
			tb.Register(tools...)

			status(fmt.Sprintf("MCP server %q ready — %d tools (%s)", mc.Name, len(tools), time.Since(start).Round(time.Millisecond)))
			results[i] = mcpResult{name: mc.Name, client: client, tb: tb}
		})
	}
	wg.Wait()

	// Check results: on first error, close any successfully-connected clients.
	for _, r := range results {
		if r.err != nil {
			for _, r2 := range results {
				if r2.client != nil {
					_ = r2.client.Close()
				}
			}
			return r.err
		}
	}

	// All succeeded — populate engine state.
	for _, r := range results {
		e.mcpClients = append(e.mcpClients, r.client)
		e.toolboxes[r.name] = r.tb
	}

	return nil
}

// wireRoots seeds MCP clients with currently-approved directories as roots and
// registers an observer that dynamically propagates new approvals.
func (e *Engine) wireRoots(permStore *permissions.Store) {
	if len(e.mcpClients) == 0 {
		return
	}

	dirs := permStore.ApprovedDirs()
	if len(dirs) > 0 {
		roots := make([]*mcp.Root, len(dirs))
		for i, d := range dirs {
			roots[i] = mcpclient.DirToRoot(d)
		}
		for _, c := range e.mcpClients {
			c.AddRoots(roots...)
		}
	}

	permStore.OnDirApproved(func(dir string) {
		root := mcpclient.DirToRoot(dir)
		for _, c := range e.mcpClients {
			c.AddRoots(root)
		}
	})
}
