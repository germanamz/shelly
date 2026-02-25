package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleYAML = `
providers:
  - name: default
    kind: anthropic
    api_key: sk-test
    model: claude-sonnet-4-20250514

mcp_servers:
  - name: search
    command: mcp-search
    args: ["--port", "8080"]

agents:
  - name: assistant
    description: A helpful assistant
    instructions: Be concise.
    provider: default
    toolboxes: [search, state]
    options:
      max_iterations: 10
      max_delegation_depth: 3

entry_agent: assistant
`

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(sampleYAML), 0o600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Len(t, cfg.Providers, 1)
	assert.Equal(t, "default", cfg.Providers[0].Name)
	assert.Equal(t, "anthropic", cfg.Providers[0].Kind)
	assert.Equal(t, "sk-test", cfg.Providers[0].APIKey)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Providers[0].Model)

	assert.Len(t, cfg.MCPServers, 1)
	assert.Equal(t, "search", cfg.MCPServers[0].Name)
	assert.Equal(t, []string{"--port", "8080"}, cfg.MCPServers[0].Args)

	assert.Len(t, cfg.Agents, 1)
	assert.Equal(t, "assistant", cfg.Agents[0].Name)
	assert.Equal(t, "default", cfg.Agents[0].Provider)
	assert.Equal(t, 10, cfg.Agents[0].Options.MaxIterations)
	assert.Equal(t, 3, cfg.Agents[0].Options.MaxDelegationDepth)

	assert.Equal(t, []string{"search", "state"}, cfg.Agents[0].Toolboxes)
	assert.Equal(t, "assistant", cfg.EntryAgent)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/no/such/file.yaml")
	assert.Error(t, err)
}

func TestLoadConfig_ExpandsEnvVars(t *testing.T) {
	t.Setenv("SHELLY_TEST_API_KEY", "sk-from-env")

	yaml := `
providers:
  - name: p1
    kind: anthropic
    api_key: ${SHELLY_TEST_API_KEY}
    model: claude-sonnet-4-20250514
agents:
  - name: a1
    provider: p1
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "sk-from-env", cfg.Providers[0].APIKey)
}

func TestLoadConfig_UnsetEnvVarExpandsToEmpty(t *testing.T) {
	yaml := `
providers:
  - name: p1
    kind: anthropic
    api_key: ${SHELLY_TEST_UNSET_VAR_12345}
    model: m1
agents:
  - name: a1
    provider: p1
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Empty(t, cfg.Providers[0].APIKey)
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_NoProviders(t *testing.T) {
	cfg := Config{Agents: []AgentConfig{{Name: "a1"}}}
	assert.ErrorContains(t, cfg.Validate(), "at least one provider")
}

func TestConfig_Validate_NoAgents(t *testing.T) {
	cfg := Config{Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}}}
	assert.ErrorContains(t, cfg.Validate(), "at least one agent")
}

func TestConfig_Validate_DuplicateProvider(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{
			{Name: "p1", Kind: "anthropic"},
			{Name: "p1", Kind: "openai"},
		},
		Agents: []AgentConfig{{Name: "a1"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "duplicate provider name")
}

func TestConfig_Validate_DuplicateAgent(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents: []AgentConfig{
			{Name: "a1"},
			{Name: "a1"},
		},
	}
	assert.ErrorContains(t, cfg.Validate(), "duplicate agent name")
}

func TestConfig_Validate_UnknownProvider(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "nope"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "unknown provider")
}

func TestConfig_Validate_UnknownToolbox(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Toolboxes: []string{"nope"}}},
	}
	assert.ErrorContains(t, cfg.Validate(), "unknown toolbox")
}

func TestConfig_Validate_StateToolbox(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Toolboxes: []string{"state"}}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_UnknownEntryAgent(t *testing.T) {
	cfg := Config{
		Providers:  []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:     []AgentConfig{{Name: "a1"}},
		EntryAgent: "nope",
	}
	assert.ErrorContains(t, cfg.Validate(), "entry_agent")
}

func TestConfig_Validate_ProviderNameRequired(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "provider name is required")
}

func TestConfig_Validate_ProviderKindRequired(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1"}},
		Agents:    []AgentConfig{{Name: "a1"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "kind is required")
}

func TestConfig_Validate_MCPNameRequired(t *testing.T) {
	cfg := Config{
		Providers:  []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		MCPServers: []MCPConfig{{Command: "cmd"}},
		Agents:     []AgentConfig{{Name: "a1"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "mcp server name is required")
}

func TestConfig_Validate_MCPCommandRequired(t *testing.T) {
	cfg := Config{
		Providers:  []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		MCPServers: []MCPConfig{{Name: "m1"}},
		Agents:     []AgentConfig{{Name: "a1"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "command is required")
}

func TestConfig_Validate_FilesystemToolbox(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Toolboxes: []string{"filesystem"}}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_DuplicateMCP(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		MCPServers: []MCPConfig{
			{Name: "m1", Command: "cmd1"},
			{Name: "m1", Command: "cmd2"},
		},
		Agents: []AgentConfig{{Name: "a1"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "duplicate mcp server name")
}

func intPtr(v int) *int { return &v }

func TestConfig_Validate_NegativeContextWindow(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic", ContextWindow: intPtr(-1)}},
		Agents:    []AgentConfig{{Name: "a1"}},
	}
	assert.ErrorContains(t, cfg.Validate(), "context_window must be >= 0")
}

func TestConfig_Validate_ValidContextWindow(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic", ContextWindow: intPtr(200000)}},
		Agents:    []AgentConfig{{Name: "a1"}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_NilContextWindow(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1"}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_ZeroContextWindow(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic", ContextWindow: intPtr(0)}},
		Agents:    []AgentConfig{{Name: "a1"}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_InvalidContextThreshold(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Options: AgentOptions{ContextThreshold: 1.5}}},
	}
	assert.ErrorContains(t, cfg.Validate(), "context_threshold must be in (0, 1)")
}

func TestConfig_Validate_NegativeContextThreshold(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Options: AgentOptions{ContextThreshold: -0.5}}},
	}
	assert.ErrorContains(t, cfg.Validate(), "context_threshold must be in (0, 1)")
}

func TestConfig_Validate_ValidContextThreshold(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Options: AgentOptions{ContextThreshold: 0.8}}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_ZeroContextThreshold(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Options: AgentOptions{ContextThreshold: 0}}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_SearchToolbox(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Toolboxes: []string{"search"}}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_GitToolbox(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Toolboxes: []string{"git"}}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_HTTPToolbox(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents:    []AgentConfig{{Name: "a1", Toolboxes: []string{"http"}}},
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_AllEffectKinds(t *testing.T) {
	allKinds := []string{
		"compact", "trim_tool_results", "loop_detect",
		"sliding_window", "observation_mask", "reflection", "progress",
	}

	for _, kind := range allKinds {
		t.Run(kind, func(t *testing.T) {
			cfg := Config{
				Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
				Agents: []AgentConfig{{
					Name:    "a1",
					Effects: []EffectConfig{{Kind: kind}},
				}},
			}
			assert.NoError(t, cfg.Validate())
		})
	}
}

func TestConfig_Validate_UnknownEffectKind(t *testing.T) {
	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "anthropic"}},
		Agents: []AgentConfig{{
			Name:    "a1",
			Effects: []EffectConfig{{Kind: "nonexistent"}},
		}},
	}
	assert.ErrorContains(t, cfg.Validate(), "unknown kind")
}
