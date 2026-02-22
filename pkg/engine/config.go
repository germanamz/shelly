package engine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level engine configuration.
type Config struct {
	ShellyDir    string           `yaml:"-"` // Set by CLI, not from YAML.
	Providers    []ProviderConfig `yaml:"providers"`
	MCPServers   []MCPConfig      `yaml:"mcp_servers"`
	Agents       []AgentConfig    `yaml:"agents"`
	EntryAgent   string           `yaml:"entry_agent"`
	StateEnabled bool             `yaml:"state_enabled"`
	TasksEnabled bool             `yaml:"tasks_enabled"`
	Filesystem   FilesystemConfig `yaml:"filesystem"`
	Exec         ExecConfig       `yaml:"exec"`
	Search       SearchConfig     `yaml:"search"`
	Git          GitConfig        `yaml:"git"`
	HTTP         HTTPConfig       `yaml:"http"`
}

// FilesystemConfig controls the filesystem tools.
type FilesystemConfig struct {
	Enabled         bool   `yaml:"enabled"`
	PermissionsFile string `yaml:"permissions_file"`
}

// ExecConfig controls the exec tool.
type ExecConfig struct {
	Enabled bool `yaml:"enabled"`
}

// SearchConfig controls the search tools.
type SearchConfig struct {
	Enabled bool `yaml:"enabled"`
}

// GitConfig controls the git tools.
type GitConfig struct {
	Enabled bool   `yaml:"enabled"`
	WorkDir string `yaml:"work_dir"`
}

// HTTPConfig controls the http tool.
type HTTPConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ProviderConfig describes an LLM provider instance.
type ProviderConfig struct {
	Name    string `yaml:"name"`
	Kind    string `yaml:"kind"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"` //nolint:gosec // configuration field, not a hardcoded secret
	Model   string `yaml:"model"`
}

// MCPConfig describes an MCP server to connect to.
type MCPConfig struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

// AgentConfig describes an agent to register.
type AgentConfig struct {
	Name         string       `yaml:"name"`
	Description  string       `yaml:"description"`
	Instructions string       `yaml:"instructions"`
	Provider     string       `yaml:"provider"`
	ToolBoxNames []string     `yaml:"toolbox_names"`
	Options      AgentOptions `yaml:"options"`
}

// AgentOptions holds optional agent behaviour settings.
type AgentOptions struct {
	MaxIterations      int `yaml:"max_iterations"`
	MaxDelegationDepth int `yaml:"max_delegation_depth"`
}

// LoadConfig reads a YAML file and returns a Config.
// Environment variables referenced as ${VAR} or $VAR in the YAML are expanded
// before parsing. This allows API keys and other secrets to be kept in
// environment variables (e.g. loaded from a .env file) rather than committed
// in the config.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-provided configuration, not user input
	if err != nil {
		return Config{}, fmt.Errorf("engine: load config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return Config{}, fmt.Errorf("engine: parse config: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration is internally consistent.
func (c Config) Validate() error {
	if len(c.Providers) == 0 {
		return fmt.Errorf("engine: config: at least one provider is required")
	}

	providerNames := make(map[string]struct{}, len(c.Providers))
	for _, p := range c.Providers {
		if p.Name == "" {
			return fmt.Errorf("engine: config: provider name is required")
		}
		if p.Kind == "" {
			return fmt.Errorf("engine: config: provider %q: kind is required", p.Name)
		}
		if _, dup := providerNames[p.Name]; dup {
			return fmt.Errorf("engine: config: duplicate provider name %q", p.Name)
		}
		providerNames[p.Name] = struct{}{}
	}

	mcpNames := make(map[string]struct{}, len(c.MCPServers))
	for _, m := range c.MCPServers {
		if m.Name == "" {
			return fmt.Errorf("engine: config: mcp server name is required")
		}
		if m.Command == "" {
			return fmt.Errorf("engine: config: mcp server %q: command is required", m.Name)
		}
		if _, dup := mcpNames[m.Name]; dup {
			return fmt.Errorf("engine: config: duplicate mcp server name %q", m.Name)
		}
		mcpNames[m.Name] = struct{}{}
	}

	if len(c.Agents) == 0 {
		return fmt.Errorf("engine: config: at least one agent is required")
	}

	agentNames := make(map[string]struct{}, len(c.Agents))
	for _, a := range c.Agents {
		if a.Name == "" {
			return fmt.Errorf("engine: config: agent name is required")
		}
		if _, dup := agentNames[a.Name]; dup {
			return fmt.Errorf("engine: config: duplicate agent name %q", a.Name)
		}
		agentNames[a.Name] = struct{}{}

		if _, ok := providerNames[a.Provider]; a.Provider != "" && !ok {
			return fmt.Errorf("engine: config: agent %q: unknown provider %q", a.Name, a.Provider)
		}

		for _, tb := range a.ToolBoxNames {
			if _, builtin := builtinToolboxNames[tb]; builtin {
				continue
			}
			if _, ok := mcpNames[tb]; !ok {
				return fmt.Errorf("engine: config: agent %q: unknown toolbox %q", a.Name, tb)
			}
		}
	}

	if c.EntryAgent != "" {
		if _, ok := agentNames[c.EntryAgent]; !ok {
			return fmt.Errorf("engine: config: entry_agent %q not found in agents", c.EntryAgent)
		}
	}

	return nil
}
