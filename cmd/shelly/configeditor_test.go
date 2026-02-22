package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRawConfig_PreservesEnvVars(t *testing.T) {
	raw := `
providers:
  - name: p1
    kind: anthropic
    api_key: ${MY_API_KEY}
    model: m1
agents:
  - name: a1
    provider: p1
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	cfg, err := loadRawConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "${MY_API_KEY}", cfg.Providers[0].APIKey)
}

func TestLoadRawConfig_FileNotFound(t *testing.T) {
	_, err := loadRawConfig("/no/such/file.yaml")
	assert.Error(t, err)
}

func TestConfigToEditor_RoundTrip(t *testing.T) {
	cfg := engine.Config{
		Providers: []engine.ProviderConfig{
			{Name: "p1", Kind: "anthropic", BaseURL: "https://custom.api", APIKey: "${KEY}", Model: "m1"},
		},
		MCPServers: []engine.MCPConfig{
			{Name: "search", Command: "mcp-search", Args: []string{"--port", "8080"}},
		},
		Agents: []engine.AgentConfig{
			{
				Name: "a1", Description: "desc", Instructions: "inst", Provider: "p1",
				ToolBoxNames: []string{"search"},
				Options:      engine.AgentOptions{MaxIterations: 10, MaxDelegationDepth: 3},
			},
		},
		EntryAgent:   "a1",
		StateEnabled: true,
		TasksEnabled: true,
		Filesystem:   engine.FilesystemConfig{Enabled: true, PermissionsFile: "perms.yaml"},
		Exec:         engine.ExecConfig{Enabled: true},
		Search:       engine.SearchConfig{Enabled: true},
		Git:          engine.GitConfig{Enabled: true, WorkDir: "/repo"},
		HTTP:         engine.HTTPConfig{Enabled: false},
	}

	ec := configToEditor(cfg)

	// Verify editor state.
	assert.Len(t, ec.Providers, 1)
	assert.Equal(t, "https://custom.api", ec.Providers[0].BaseURL)
	assert.Equal(t, "${KEY}", ec.Providers[0].APIKey)
	assert.Len(t, ec.MCPServers, 1)
	assert.Equal(t, "--port 8080", ec.MCPServers[0].Args)
	assert.Len(t, ec.Agents, 1)
	assert.Equal(t, []string{"search"}, ec.Agents[0].ToolBoxNames)
	assert.Equal(t, "a1", ec.EntryAgent)
	assert.Contains(t, ec.Tools, "filesystem")
	assert.Contains(t, ec.Tools, "exec")
	assert.Contains(t, ec.Tools, "search")
	assert.Contains(t, ec.Tools, "git")
	assert.NotContains(t, ec.Tools, "http")
	assert.Equal(t, "perms.yaml", ec.PermissionsFile)
	assert.Equal(t, "/repo", ec.GitWorkDir)

	// Round-trip through marshal and re-parse.
	data, err := marshalEditorConfig(ec)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	got, err := loadRawConfig(path)
	require.NoError(t, err)

	assert.Equal(t, cfg.Providers[0].Name, got.Providers[0].Name)
	assert.Equal(t, cfg.Providers[0].Kind, got.Providers[0].Kind)
	assert.Equal(t, cfg.Providers[0].BaseURL, got.Providers[0].BaseURL)
	assert.Equal(t, cfg.Providers[0].APIKey, got.Providers[0].APIKey)
	assert.Equal(t, cfg.MCPServers[0].Name, got.MCPServers[0].Name)
	assert.Equal(t, cfg.MCPServers[0].Args, got.MCPServers[0].Args)
	assert.Equal(t, cfg.Agents[0].Name, got.Agents[0].Name)
	assert.Equal(t, cfg.Agents[0].ToolBoxNames, got.Agents[0].ToolBoxNames)
	assert.Equal(t, cfg.Agents[0].Options.MaxIterations, got.Agents[0].Options.MaxIterations)
	assert.Equal(t, cfg.Agents[0].Options.MaxDelegationDepth, got.Agents[0].Options.MaxDelegationDepth)
	assert.Equal(t, cfg.EntryAgent, got.EntryAgent)
	assert.Equal(t, cfg.StateEnabled, got.StateEnabled)
	assert.Equal(t, cfg.TasksEnabled, got.TasksEnabled)
	assert.Equal(t, cfg.Filesystem.Enabled, got.Filesystem.Enabled)
	assert.Equal(t, cfg.Filesystem.PermissionsFile, got.Filesystem.PermissionsFile)
	assert.Equal(t, cfg.Git.Enabled, got.Git.Enabled)
	assert.Equal(t, cfg.Git.WorkDir, got.Git.WorkDir)
	assert.False(t, got.HTTP.Enabled)
}

func TestMCPArgsParsing(t *testing.T) {
	// Join preserves order.
	args := []string{"--port", "8080", "--verbose"}
	joined := strings.Join(args, " ")
	assert.Equal(t, "--port 8080 --verbose", joined)

	// Split recovers the original args.
	split := strings.Fields(joined)
	assert.Equal(t, args, split)

	// Empty string produces empty slice.
	assert.Empty(t, strings.Fields(""))
}

func TestEditorToEngineConfig(t *testing.T) {
	ec := editorConfig{
		Providers: []editorProvider{
			{Kind: "anthropic", Name: "p1", APIKey: "key", Model: "m1"},
		},
		Agents: []editorAgent{
			{Name: "a1", Provider: "p1", MaxIterations: 5, MaxDelegationDepth: 1},
		},
		EntryAgent:   "a1",
		Tools:        []string{"filesystem", "git"},
		StateEnabled: true,
		MCPServers: []editorMCP{
			{Name: "s1", Command: "cmd", Args: "--flag value"},
		},
		PermissionsFile: "perms.yaml",
		GitWorkDir:      "/work",
	}

	cfg := editorToEngineConfig(ec)

	assert.True(t, cfg.Filesystem.Enabled)
	assert.Equal(t, "perms.yaml", cfg.Filesystem.PermissionsFile)
	assert.False(t, cfg.Exec.Enabled)
	assert.True(t, cfg.Git.Enabled)
	assert.Equal(t, "/work", cfg.Git.WorkDir)
	assert.False(t, cfg.HTTP.Enabled)
	assert.Len(t, cfg.MCPServers, 1)
	assert.Equal(t, []string{"--flag", "value"}, cfg.MCPServers[0].Args)
	assert.True(t, cfg.StateEnabled)
	assert.Equal(t, "a1", cfg.EntryAgent)
}

func TestMarshalEditorConfig_EmptyMCPServers(t *testing.T) {
	ec := editorConfig{
		Providers: []editorProvider{
			{Kind: "anthropic", Name: "p1", APIKey: "key", Model: "m1"},
		},
		Agents: []editorAgent{
			{Name: "a1", Provider: "p1", MaxIterations: 10, MaxDelegationDepth: 2},
		},
		EntryAgent: "a1",
		Tools:      []string{"filesystem"},
	}

	data, err := marshalEditorConfig(ec)
	require.NoError(t, err)

	// mcp_servers should be omitted when empty.
	assert.NotContains(t, string(data), "mcp_servers")
}
