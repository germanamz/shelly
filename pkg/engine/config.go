package engine

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Config is the top-level engine configuration.
type Config struct {
	ShellyDir             string           `yaml:"-"` // Set by CLI, not from YAML.
	Providers             []ProviderConfig `yaml:"providers"`
	MCPServers            []MCPConfig      `yaml:"mcp_servers"`
	Agents                []AgentConfig    `yaml:"agents"`
	EntryAgent            string           `yaml:"entry_agent"`
	Filesystem            FilesystemConfig `yaml:"filesystem"`
	Git                   GitConfig        `yaml:"git"`
	Browser               BrowserConfig    `yaml:"browser"`
	DefaultContextWindows map[string]int   `yaml:"default_context_windows"` // Per-kind context window overrides (e.g. anthropic: 200000).
	StatusFunc            func(string)     `yaml:"-"`                       // Called with progress messages during initialization. Nil means silent.
}

// BrowserConfig holds browser tool settings.
type BrowserConfig struct {
	Headless bool `yaml:"headless"`
}

// FilesystemConfig holds filesystem tool settings.
type FilesystemConfig struct {
	PermissionsFile string `yaml:"permissions_file"`
}

// GitConfig holds git tool settings.
type GitConfig struct {
	WorkDir string `yaml:"work_dir"`
}

// RateLimitConfig controls per-provider rate limiting.
type RateLimitConfig struct {
	InputTPM   int    `yaml:"input_tpm"`   // Input tokens per minute (0 = no limit).
	OutputTPM  int    `yaml:"output_tpm"`  // Output tokens per minute (0 = no limit).
	RPM        int    `yaml:"rpm"`         // Requests per minute (0 = no limit).
	MaxRetries int    `yaml:"max_retries"` // Max retries on 429 (default 3).
	BaseDelay  string `yaml:"base_delay"`  // Initial backoff delay as a duration string (e.g. "1s", "500ms").
}

// ProviderConfig describes an LLM provider instance.
type ProviderConfig struct {
	Name          string          `yaml:"name"`
	Kind          string          `yaml:"kind"`
	BaseURL       string          `yaml:"base_url"`
	APIKey        string          `yaml:"api_key"` //nolint:gosec // configuration field, not a hardcoded secret
	Model         string          `yaml:"model"`
	ContextWindow *int            `yaml:"context_window"` // Max context tokens (nil = use provider default, 0 = no compaction).
	RateLimit     RateLimitConfig `yaml:"rate_limit"`
}

// MCPConfig describes an MCP server to connect to.
type MCPConfig struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	URL     string   `yaml:"url"` // SSE endpoint URL (mutually exclusive with Command).
}

// EffectConfig describes a single effect attached to an agent.
type EffectConfig struct {
	Kind   string         `yaml:"kind"`
	Params map[string]any `yaml:"params"`
}

// ToolboxRef references a toolbox by name with an optional tool whitelist.
// In YAML it supports both a plain string ("filesystem") and an object form
// ({name: git, tools: [git_status, git_diff]}).
type ToolboxRef struct {
	Name  string   `yaml:"name"`
	Tools []string `yaml:"tools"`
}

// UnmarshalYAML supports both scalar strings and mapping nodes.
func (r *ToolboxRef) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		r.Name = value.Value
		return nil
	}
	type alias ToolboxRef
	var a alias
	if err := value.Decode(&a); err != nil {
		return err
	}
	*r = ToolboxRef(a)
	return nil
}

// MarshalYAML emits a plain string when Tools is empty, otherwise a mapping.
func (r ToolboxRef) MarshalYAML() (any, error) {
	if len(r.Tools) == 0 {
		return r.Name, nil
	}
	type alias ToolboxRef
	return alias(r), nil
}

// ToolboxRefNames extracts the Name field from each ToolboxRef.
func ToolboxRefNames(refs []ToolboxRef) []string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	return names
}

// ToolboxRefsFromNames creates plain ToolboxRef values (no tools filter)
// from a list of names.
func ToolboxRefsFromNames(names []string) []ToolboxRef {
	refs := make([]ToolboxRef, len(names))
	for i, n := range names {
		refs[i] = ToolboxRef{Name: n}
	}
	return refs
}

// AgentConfig describes an agent to register.
type AgentConfig struct {
	Name         string         `yaml:"name"`
	Description  string         `yaml:"description"`
	Instructions string         `yaml:"instructions"`
	Provider     string         `yaml:"provider"`
	Toolboxes    []ToolboxRef   `yaml:"toolboxes"`
	Skills       []string       `yaml:"skills"` // Skill names to assign to this agent. Empty means all engine-level skills.
	Effects      []EffectConfig `yaml:"effects"`
	Options      AgentOptions   `yaml:"options"`
	Prefix       string         `yaml:"prefix"` // Display prefix (e.g. "🤖", "📝"). Default: "🤖".
}

// AgentOptions holds optional agent behaviour settings.
type AgentOptions struct {
	MaxIterations      int     `yaml:"max_iterations"`
	MaxDelegationDepth int     `yaml:"max_delegation_depth"`
	ContextThreshold   float64 `yaml:"context_threshold"` // Fraction triggering compaction (0 = disabled).
}

// LoadConfig reads a YAML file and returns a Config.
// Environment variables referenced as ${VAR} or $VAR in string fields are
// expanded after parsing. This allows API keys and other secrets to be kept
// in environment variables (e.g. loaded from a .env file) rather than
// committed in the config, without risking YAML injection from env values.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-provided configuration, not user input
	if err != nil {
		return Config{}, fmt.Errorf("engine: load config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("engine: parse config: %w", err)
	}

	ExpandConfigStrings(&cfg)

	return cfg, nil
}

// LoadConfigRaw reads a YAML file and returns a Config without expanding
// environment variables. This preserves ${VAR} references in fields like
// api_key, which is useful for config editing round-trips.
func LoadConfigRaw(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-provided configuration, not user input
	if err != nil {
		return Config{}, fmt.Errorf("engine: load config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("engine: parse config: %w", err)
	}

	return cfg, nil
}

// ExpandConfigStrings expands ${VAR} environment variable references in all
// string fields of cfg. This is called automatically by LoadConfig but must be
// called manually when a Config is constructed programmatically (e.g. from an
// embedded template).
func ExpandConfigStrings(cfg *Config) {
	cfg.EntryAgent = os.ExpandEnv(cfg.EntryAgent)
	cfg.Filesystem.PermissionsFile = os.ExpandEnv(cfg.Filesystem.PermissionsFile)
	cfg.Git.WorkDir = os.ExpandEnv(cfg.Git.WorkDir)

	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		p.Name = os.ExpandEnv(p.Name)
		p.Kind = os.ExpandEnv(p.Kind)
		p.BaseURL = os.ExpandEnv(p.BaseURL)
		p.APIKey = os.ExpandEnv(p.APIKey)
		p.Model = os.ExpandEnv(p.Model)
		p.RateLimit.BaseDelay = os.ExpandEnv(p.RateLimit.BaseDelay)
	}

	for i := range cfg.MCPServers {
		m := &cfg.MCPServers[i]
		m.Name = os.ExpandEnv(m.Name)
		m.Command = os.ExpandEnv(m.Command)
		m.URL = os.ExpandEnv(m.URL)
		for j := range m.Args {
			m.Args[j] = os.ExpandEnv(m.Args[j])
		}
	}

	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		a.Name = os.ExpandEnv(a.Name)
		a.Description = os.ExpandEnv(a.Description)
		a.Instructions = os.ExpandEnv(a.Instructions)
		a.Provider = os.ExpandEnv(a.Provider)
		a.Prefix = os.ExpandEnv(a.Prefix)
		for j := range a.Toolboxes {
			a.Toolboxes[j].Name = os.ExpandEnv(a.Toolboxes[j].Name)
			for k := range a.Toolboxes[j].Tools {
				a.Toolboxes[j].Tools[k] = os.ExpandEnv(a.Toolboxes[j].Tools[k])
			}
		}
		for j := range a.Skills {
			a.Skills[j] = os.ExpandEnv(a.Skills[j])
		}
		for j := range a.Effects {
			a.Effects[j].Kind = os.ExpandEnv(a.Effects[j].Kind)
		}
	}
}

// KnownProviderKinds returns the list of registered provider kind strings.
func KnownProviderKinds() []string {
	ensureDefaults()

	factoryMu.RLock()
	defer factoryMu.RUnlock()

	kinds := make([]string, 0, len(factories))
	for k := range factories {
		kinds = append(kinds, k)
	}

	sort.Strings(kinds)
	return kinds
}

// KnownEffectKinds returns the list of recognised effect kind strings.
func KnownEffectKinds() []string {
	kinds := make([]string, 0, len(knownEffectKinds))
	for k := range knownEffectKinds {
		kinds = append(kinds, k)
	}

	sort.Strings(kinds)
	return kinds
}

// knownEffectKinds lists all recognised effect kind strings.
var knownEffectKinds = map[string]struct{}{
	"compact":           {},
	"trim_tool_results": {},
	"loop_detect":       {},
	"sliding_window":    {},
	"observation_mask":  {},
	"reflection":        {},
	"progress":          {},
}

// Validate checks that the configuration is internally consistent.
func (c Config) Validate() error {
	if len(c.Providers) == 0 {
		return fmt.Errorf("engine: config: at least one provider is required")
	}

	providerNames, err := validateProviders(c.Providers)
	if err != nil {
		return err
	}

	mcpNames, err := validateMCPServers(c.MCPServers)
	if err != nil {
		return err
	}

	if len(c.Agents) == 0 {
		return fmt.Errorf("engine: config: at least one agent is required")
	}

	agentNames, err := validateAgents(c.Agents, providerNames, mcpNames)
	if err != nil {
		return err
	}

	if c.EntryAgent != "" {
		if _, ok := agentNames[c.EntryAgent]; !ok {
			return fmt.Errorf("engine: config: entry_agent %q not found in agents", c.EntryAgent)
		}
	}

	return nil
}

func validateProviders(providers []ProviderConfig) (map[string]struct{}, error) {
	names := make(map[string]struct{}, len(providers))
	for _, p := range providers {
		if p.Name == "" {
			return nil, fmt.Errorf("engine: config: provider name is required")
		}
		if p.Kind == "" {
			return nil, fmt.Errorf("engine: config: provider %q: kind is required", p.Name)
		}
		if p.ContextWindow != nil && *p.ContextWindow < 0 {
			return nil, fmt.Errorf("engine: config: provider %q: context_window must be >= 0", p.Name)
		}
		if _, dup := names[p.Name]; dup {
			return nil, fmt.Errorf("engine: config: duplicate provider name %q", p.Name)
		}
		names[p.Name] = struct{}{}
	}
	return names, nil
}

func validateMCPServers(servers []MCPConfig) (map[string]struct{}, error) {
	names := make(map[string]struct{}, len(servers))
	for _, m := range servers {
		if m.Name == "" {
			return nil, fmt.Errorf("engine: config: mcp server name is required")
		}
		if m.Command == "" && m.URL == "" {
			return nil, fmt.Errorf("engine: config: mcp server %q: command or url is required", m.Name)
		}
		if m.Command != "" && m.URL != "" {
			return nil, fmt.Errorf("engine: config: mcp server %q: command and url are mutually exclusive", m.Name)
		}
		if _, dup := names[m.Name]; dup {
			return nil, fmt.Errorf("engine: config: duplicate mcp server name %q", m.Name)
		}
		names[m.Name] = struct{}{}
	}
	return names, nil
}

func validateAgents(agents []AgentConfig, providerNames, mcpNames map[string]struct{}) (map[string]struct{}, error) {
	names := make(map[string]struct{}, len(agents))
	for _, a := range agents {
		if a.Name == "" {
			return nil, fmt.Errorf("engine: config: agent name is required")
		}
		if _, dup := names[a.Name]; dup {
			return nil, fmt.Errorf("engine: config: duplicate agent name %q", a.Name)
		}
		names[a.Name] = struct{}{}

		if a.Options.ContextThreshold != 0 && (a.Options.ContextThreshold < 0 || a.Options.ContextThreshold >= 1) {
			return nil, fmt.Errorf("engine: config: agent %q: context_threshold must be in (0, 1) or 0 to disable", a.Name)
		}

		for i, ef := range a.Effects {
			if ef.Kind == "" {
				return nil, fmt.Errorf("engine: config: agent %q: effect[%d]: kind is required", a.Name, i)
			}
			if _, ok := knownEffectKinds[ef.Kind]; !ok {
				return nil, fmt.Errorf("engine: config: agent %q: effect[%d]: unknown kind %q", a.Name, i, ef.Kind)
			}
		}

		if _, ok := providerNames[a.Provider]; a.Provider != "" && !ok {
			return nil, fmt.Errorf("engine: config: agent %q: unknown provider %q", a.Name, a.Provider)
		}

		for _, ref := range a.Toolboxes {
			if _, builtin := builtinToolboxNames[ref.Name]; builtin {
				continue
			}
			if _, ok := mcpNames[ref.Name]; !ok {
				return nil, fmt.Errorf("engine: config: agent %q: unknown toolbox %q", a.Name, ref.Name)
			}
		}
	}
	return names, nil
}
