package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/germanamz/shelly/pkg/engine"
	"gopkg.in/yaml.v3"
)

// Editor working types.

type editorProvider struct {
	Kind       string
	Name       string
	BaseURL    string
	APIKey     string //nolint:gosec // env var reference, not a secret
	Model      string
	TPM        string
	MaxRetries string
	BaseDelay  string
}

type editorMCP struct {
	Name    string
	Command string
	Args    string // space-separated arguments
}

type editorAgent struct {
	Name               string
	Description        string
	Instructions       string
	Provider           string
	MaxIterations      int
	MaxDelegationDepth int
	ToolBoxNames       []string
}

type editorConfig struct {
	Providers       []editorProvider
	Agents          []editorAgent
	EntryAgent      string
	Tools           []string
	StateEnabled    bool
	TasksEnabled    bool
	MCPServers      []editorMCP
	PermissionsFile string
	GitWorkDir      string
}

// YAML output types (preserve all fields via omitempty).

type editorConfigYAML struct {
	Providers    []editorProviderYAML `yaml:"providers"`
	MCPServers   []mcpYAML            `yaml:"mcp_servers,omitempty"`
	Agents       []editorAgentYAML    `yaml:"agents"`
	EntryAgent   string               `yaml:"entry_agent"`
	Filesystem   editorFilesystemYAML `yaml:"filesystem"`
	Exec         toolYAML             `yaml:"exec"`
	Search       toolYAML             `yaml:"search"`
	Git          editorGitYAML        `yaml:"git"`
	HTTP         toolYAML             `yaml:"http"`
	StateEnabled bool                 `yaml:"state_enabled,omitempty"`
	TasksEnabled bool                 `yaml:"tasks_enabled,omitempty"`
}

type editorProviderYAML struct {
	Name      string         `yaml:"name"`
	Kind      string         `yaml:"kind"`
	BaseURL   string         `yaml:"base_url,omitempty"`
	APIKey    string         `yaml:"api_key"` //nolint:gosec // env var reference, not a secret
	Model     string         `yaml:"model"`
	RateLimit *rateLimitYAML `yaml:"rate_limit,omitempty"`
}

type editorAgentYAML struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	Instructions string        `yaml:"instructions"`
	Provider     string        `yaml:"provider"`
	ToolBoxNames []string      `yaml:"toolbox_names,omitempty"`
	Options      agentOptsYAML `yaml:"options"`
}

type editorFilesystemYAML struct {
	Enabled         bool   `yaml:"enabled"`
	PermissionsFile string `yaml:"permissions_file,omitempty"`
}

type editorGitYAML struct {
	Enabled bool   `yaml:"enabled"`
	WorkDir string `yaml:"work_dir,omitempty"`
}

type mcpYAML struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
}

// runConfigEditor is the entry point: load → menu loop → validate → save.
func runConfigEditor(configPath, shellyDirPath string) error {
	resolved := resolveConfigPath(configPath, shellyDirPath)

	cfg, err := loadRawConfig(resolved)
	if err != nil {
		return err
	}

	ec := configToEditor(cfg)

	for {
		if err := configEditorMenu(&ec); err != nil {
			return err
		}

		finalCfg := editorToEngineConfig(ec)
		if err := finalCfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "Validation error: %v\nReturning to menu.\n", err)

			continue
		}

		break
	}

	data, err := marshalEditorConfig(ec)
	if err != nil {
		return err
	}

	if err := os.WriteFile(resolved, data, 0o600); err != nil {
		return err
	}

	fmt.Printf("Config saved to %s\n", resolved)

	return nil
}

// loadRawConfig reads a YAML config without expanding environment variables,
// preserving ${VAR} references for re-serialization.
func loadRawConfig(path string) (engine.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-provided configuration
	if err != nil {
		return engine.Config{}, fmt.Errorf("load config: %w", err)
	}

	var cfg engine.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return engine.Config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// configToEditor converts an engine.Config to the editor working model.
func configToEditor(cfg engine.Config) editorConfig {
	ec := editorConfig{
		EntryAgent:      cfg.EntryAgent,
		StateEnabled:    cfg.StateEnabled,
		TasksEnabled:    cfg.TasksEnabled,
		PermissionsFile: cfg.Filesystem.PermissionsFile,
		GitWorkDir:      cfg.Git.WorkDir,
	}

	for _, tool := range []struct {
		name    string
		enabled bool
	}{
		{"filesystem", cfg.Filesystem.Enabled},
		{"exec", cfg.Exec.Enabled},
		{"search", cfg.Search.Enabled},
		{"git", cfg.Git.Enabled},
		{"http", cfg.HTTP.Enabled},
	} {
		if tool.enabled {
			ec.Tools = append(ec.Tools, tool.name)
		}
	}

	for _, p := range cfg.Providers {
		ep := editorProvider{
			Kind:    p.Kind,
			Name:    p.Name,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
		}

		if p.RateLimit.TPM > 0 {
			ep.TPM = strconv.Itoa(p.RateLimit.TPM)
		}

		if p.RateLimit.MaxRetries > 0 {
			ep.MaxRetries = strconv.Itoa(p.RateLimit.MaxRetries)
		}

		ep.BaseDelay = p.RateLimit.BaseDelay

		ec.Providers = append(ec.Providers, ep)
	}

	for _, a := range cfg.Agents {
		ec.Agents = append(ec.Agents, editorAgent{
			Name:               a.Name,
			Description:        a.Description,
			Instructions:       a.Instructions,
			Provider:           a.Provider,
			MaxIterations:      a.Options.MaxIterations,
			MaxDelegationDepth: a.Options.MaxDelegationDepth,
			ToolBoxNames:       a.ToolBoxNames,
		})
	}

	for _, m := range cfg.MCPServers {
		ec.MCPServers = append(ec.MCPServers, editorMCP{
			Name:    m.Name,
			Command: m.Command,
			Args:    strings.Join(m.Args, " "),
		})
	}

	return ec
}

// editorToEngineConfig converts the editor working model back to engine.Config
// for validation.
func editorToEngineConfig(ec editorConfig) engine.Config {
	toolSet := make(map[string]bool, len(ec.Tools))
	for _, t := range ec.Tools {
		toolSet[t] = true
	}

	cfg := engine.Config{
		EntryAgent:   ec.EntryAgent,
		StateEnabled: ec.StateEnabled,
		TasksEnabled: ec.TasksEnabled,
		Filesystem:   engine.FilesystemConfig{Enabled: toolSet["filesystem"], PermissionsFile: ec.PermissionsFile},
		Exec:         engine.ExecConfig{Enabled: toolSet["exec"]},
		Search:       engine.SearchConfig{Enabled: toolSet["search"]},
		Git:          engine.GitConfig{Enabled: toolSet["git"], WorkDir: ec.GitWorkDir},
		HTTP:         engine.HTTPConfig{Enabled: toolSet["http"]},
	}

	for _, p := range ec.Providers {
		tpm, _ := strconv.Atoi(p.TPM)
		maxRetries, _ := strconv.Atoi(p.MaxRetries)

		cfg.Providers = append(cfg.Providers, engine.ProviderConfig{
			Name:    p.Name,
			Kind:    p.Kind,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
			RateLimit: engine.RateLimitConfig{
				TPM:        tpm,
				MaxRetries: maxRetries,
				BaseDelay:  p.BaseDelay,
			},
		})
	}

	for _, a := range ec.Agents {
		cfg.Agents = append(cfg.Agents, engine.AgentConfig{
			Name:         a.Name,
			Description:  a.Description,
			Instructions: a.Instructions,
			Provider:     a.Provider,
			ToolBoxNames: a.ToolBoxNames,
			Options: engine.AgentOptions{
				MaxIterations:      a.MaxIterations,
				MaxDelegationDepth: a.MaxDelegationDepth,
			},
		})
	}

	for _, m := range ec.MCPServers {
		cfg.MCPServers = append(cfg.MCPServers, engine.MCPConfig{
			Name:    m.Name,
			Command: m.Command,
			Args:    strings.Fields(m.Args),
		})
	}

	return cfg
}

// marshalEditorConfig serializes the editor config to YAML bytes.
func marshalEditorConfig(ec editorConfig) ([]byte, error) {
	toolSet := make(map[string]bool, len(ec.Tools))
	for _, t := range ec.Tools {
		toolSet[t] = true
	}

	yc := editorConfigYAML{
		EntryAgent: ec.EntryAgent,
		Filesystem: editorFilesystemYAML{
			Enabled:         toolSet["filesystem"],
			PermissionsFile: ec.PermissionsFile,
		},
		Exec:   toolYAML{Enabled: toolSet["exec"]},
		Search: toolYAML{Enabled: toolSet["search"]},
		Git: editorGitYAML{
			Enabled: toolSet["git"],
			WorkDir: ec.GitWorkDir,
		},
		HTTP:         toolYAML{Enabled: toolSet["http"]},
		StateEnabled: ec.StateEnabled,
		TasksEnabled: ec.TasksEnabled,
	}

	for _, p := range ec.Providers {
		py := editorProviderYAML{
			Name:    p.Name,
			Kind:    p.Kind,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
		}

		tpm, _ := strconv.Atoi(p.TPM)
		maxRetries, _ := strconv.Atoi(p.MaxRetries)

		if tpm > 0 || maxRetries > 0 || p.BaseDelay != "" {
			py.RateLimit = &rateLimitYAML{
				TPM:        tpm,
				MaxRetries: maxRetries,
				BaseDelay:  p.BaseDelay,
			}
		}

		yc.Providers = append(yc.Providers, py)
	}

	for _, a := range ec.Agents {
		yc.Agents = append(yc.Agents, editorAgentYAML{
			Name:         a.Name,
			Description:  a.Description,
			Instructions: a.Instructions,
			Provider:     a.Provider,
			ToolBoxNames: a.ToolBoxNames,
			Options: agentOptsYAML{
				MaxIterations:      a.MaxIterations,
				MaxDelegationDepth: a.MaxDelegationDepth,
			},
		})
	}

	for _, m := range ec.MCPServers {
		args := strings.Fields(m.Args)
		yc.MCPServers = append(yc.MCPServers, mcpYAML{
			Name:    m.Name,
			Command: m.Command,
			Args:    args,
		})
	}

	return yaml.Marshal(yc)
}

// configEditorMenu displays the top-level menu and dispatches to section editors.
func configEditorMenu(ec *editorConfig) error {
	for {
		var choice string

		err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Config Editor").
				Options(
					huh.NewOption("Providers", "providers"),
					huh.NewOption("Agents", "agents"),
					huh.NewOption("Entry Agent", "entry_agent"),
					huh.NewOption("Tools", "tools"),
					huh.NewOption("Features", "features"),
					huh.NewOption("MCP Servers", "mcp_servers"),
					huh.NewOption("Save & Exit", "done"),
				).
				Value(&choice),
		)).Run()
		if err != nil {
			return err
		}

		switch choice {
		case "providers":
			if err := editProviders(ec); err != nil {
				return err
			}
		case "agents":
			if err := editAgents(ec); err != nil {
				return err
			}
		case "entry_agent":
			if err := editEntryAgent(ec); err != nil {
				return err
			}
		case "tools":
			if err := editTools(ec); err != nil {
				return err
			}
		case "features":
			if err := editFeatures(ec); err != nil {
				return err
			}
		case "mcp_servers":
			if err := editMCPServers(ec); err != nil {
				return err
			}
		case "done":
			return nil
		}
	}
}

// editProviders manages the providers list (Add/Edit/Remove/Back).
func editProviders(ec *editorConfig) error {
	for {
		opts := []huh.Option[string]{
			huh.NewOption("Add provider", "add"),
		}

		for _, p := range ec.Providers {
			opts = append(opts, huh.NewOption(fmt.Sprintf("Edit: %s (%s)", p.Name, p.Kind), "edit:"+p.Name))
		}

		for _, p := range ec.Providers {
			opts = append(opts, huh.NewOption(fmt.Sprintf("Remove: %s", p.Name), "remove:"+p.Name))
		}

		opts = append(opts, huh.NewOption("Back", "back"))

		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Providers").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return err
		}

		switch {
		case choice == "add":
			p := editorProvider{
				Kind:   "anthropic",
				Name:   "anthropic",
				APIKey: providerDefaults["anthropic"].APIKey,
				Model:  providerDefaults["anthropic"].Model,
			}
			if err := editProviderForm(&p); err != nil {
				return err
			}

			ec.Providers = append(ec.Providers, p)
		case strings.HasPrefix(choice, "edit:"):
			name := strings.TrimPrefix(choice, "edit:")

			for i := range ec.Providers {
				if ec.Providers[i].Name == name {
					if err := editProviderForm(&ec.Providers[i]); err != nil {
						return err
					}

					break
				}
			}
		case strings.HasPrefix(choice, "remove:"):
			name := strings.TrimPrefix(choice, "remove:")

			for i, p := range ec.Providers {
				if p.Name == name {
					ec.Providers = append(ec.Providers[:i], ec.Providers[i+1:]...)

					break
				}
			}
		case choice == "back":
			return nil
		}
	}
}

// editProviderForm shows a pre-filled form for editing a single provider.
func editProviderForm(p *editorProvider) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provider kind").
				Options(
					huh.NewOption("Anthropic", "anthropic"),
					huh.NewOption("OpenAI", "openai"),
					huh.NewOption("Grok", "grok"),
				).
				Value(&p.Kind),
			huh.NewInput().Title("Provider name").Value(&p.Name),
			huh.NewInput().Title("Base URL (optional)").Value(&p.BaseURL),
			huh.NewInput().Title("API key env var").Value(&p.APIKey),
			huh.NewInput().Title("Model").Value(&p.Model),
		),
		huh.NewGroup(
			huh.NewInput().Title("Tokens per minute (0 = no limit)").Value(&p.TPM).Validate(validateOptionalNonNegativeInt),
			huh.NewInput().Title("Max retries on 429").Value(&p.MaxRetries).Validate(validateOptionalNonNegativeInt),
			huh.NewInput().Title("Base backoff delay (e.g. 1s, 500ms)").Value(&p.BaseDelay).Validate(validateDuration),
		).Title("Rate Limiting"),
	).Run()
}

func validateOptionalNonNegativeInt(s string) error {
	if s == "" {
		return nil
	}

	return validateNonNegativeInt(s)
}

// editAgents manages the agents list (Add/Edit/Remove/Back).
func editAgents(ec *editorConfig) error {
	for {
		opts := []huh.Option[string]{
			huh.NewOption("Add agent", "add"),
		}

		for _, a := range ec.Agents {
			opts = append(opts, huh.NewOption(fmt.Sprintf("Edit: %s", a.Name), "edit:"+a.Name))
		}

		for _, a := range ec.Agents {
			opts = append(opts, huh.NewOption(fmt.Sprintf("Remove: %s", a.Name), "remove:"+a.Name))
		}

		opts = append(opts, huh.NewOption("Back", "back"))

		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Agents").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return err
		}

		providerNames := make([]string, len(ec.Providers))
		for i, p := range ec.Providers {
			providerNames[i] = p.Name
		}

		mcpNames := make([]string, len(ec.MCPServers))
		for i, m := range ec.MCPServers {
			mcpNames[i] = m.Name
		}

		switch {
		case choice == "add":
			a := editorAgent{
				Name:               "assistant",
				Description:        "A helpful assistant",
				Instructions:       "You are a helpful assistant. Be concise and accurate.",
				MaxIterations:      10,
				MaxDelegationDepth: 2,
			}

			if len(providerNames) > 0 {
				a.Provider = providerNames[0]
			}

			if err := editAgentForm(&a, providerNames, mcpNames); err != nil {
				return err
			}

			ec.Agents = append(ec.Agents, a)
		case strings.HasPrefix(choice, "edit:"):
			name := strings.TrimPrefix(choice, "edit:")

			for i := range ec.Agents {
				if ec.Agents[i].Name == name {
					if err := editAgentForm(&ec.Agents[i], providerNames, mcpNames); err != nil {
						return err
					}

					break
				}
			}
		case strings.HasPrefix(choice, "remove:"):
			name := strings.TrimPrefix(choice, "remove:")

			for i, a := range ec.Agents {
				if a.Name == name {
					ec.Agents = append(ec.Agents[:i], ec.Agents[i+1:]...)

					break
				}
			}
		case choice == "back":
			return nil
		}
	}
}

// editAgentForm shows a pre-filled form for editing a single agent.
func editAgentForm(a *editorAgent, providerNames, mcpNames []string) error {
	provOpts := make([]huh.Option[string], len(providerNames))
	for i, n := range providerNames {
		provOpts[i] = huh.NewOption(n, n)
	}

	maxIter := strconv.Itoa(a.MaxIterations)
	maxDepth := strconv.Itoa(a.MaxDelegationDepth)

	builtinToolboxes := []string{"filesystem", "exec", "search", "git", "http", "state", "tasks", "ask", "defaults"}

	selectedSet := make(map[string]bool, len(a.ToolBoxNames))
	for _, tb := range a.ToolBoxNames {
		selectedSet[tb] = true
	}

	var toolboxOpts []huh.Option[string]

	for _, name := range builtinToolboxes {
		opt := huh.NewOption(name, name)
		if selectedSet[name] {
			opt = opt.Selected(true)
		}

		toolboxOpts = append(toolboxOpts, opt)
	}

	for _, name := range mcpNames {
		opt := huh.NewOption(fmt.Sprintf("mcp: %s", name), name)
		if selectedSet[name] {
			opt = opt.Selected(true)
		}

		toolboxOpts = append(toolboxOpts, opt)
	}

	fields := []huh.Field{
		huh.NewInput().Title("Agent name").Value(&a.Name),
		huh.NewInput().Title("Description").Value(&a.Description),
		huh.NewText().Title("Instructions").Value(&a.Instructions),
	}

	if len(provOpts) > 0 {
		fields = append(fields, huh.NewSelect[string]().Title("Provider").Options(provOpts...).Value(&a.Provider))
	}

	fields = append(fields,
		huh.NewInput().Title("Max iterations").Value(&maxIter).Validate(validatePositiveInt),
		huh.NewInput().Title("Max delegation depth").Value(&maxDepth).Validate(validateNonNegativeInt),
	)

	if len(toolboxOpts) > 0 {
		fields = append(fields,
			huh.NewMultiSelect[string]().
				Title("Toolbox names (agent-specific)").
				Options(toolboxOpts...).
				Value(&a.ToolBoxNames),
		)
	}

	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}

	a.MaxIterations, _ = strconv.Atoi(maxIter)
	a.MaxDelegationDepth, _ = strconv.Atoi(maxDepth)

	return nil
}

// editEntryAgent lets the user pick which agent is the entry point.
func editEntryAgent(ec *editorConfig) error {
	if len(ec.Agents) == 0 {
		fmt.Println("No agents defined. Add an agent first.")

		return nil
	}

	opts := make([]huh.Option[string], len(ec.Agents))
	for i, a := range ec.Agents {
		opts[i] = huh.NewOption(a.Name, a.Name)
	}

	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Entry agent").
			Options(opts...).
			Value(&ec.EntryAgent),
	)).Run()
}

// editTools manages the global tool toggles via multi-select.
func editTools(ec *editorConfig) error {
	toolSet := make(map[string]bool, len(ec.Tools))
	for _, t := range ec.Tools {
		toolSet[t] = true
	}

	allTools := []string{"filesystem", "exec", "search", "git", "http"}

	var opts []huh.Option[string]
	for _, name := range allTools {
		label := strings.ToUpper(name[:1]) + name[1:]
		opt := huh.NewOption(label, name)
		if toolSet[name] {
			opt = opt.Selected(true)
		}

		opts = append(opts, opt)
	}

	return huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Which built-in tools should be enabled?").
			Options(opts...).
			Value(&ec.Tools),
	)).Run()
}

// editFeatures manages boolean feature flags.
func editFeatures(ec *editorConfig) error {
	return huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Enable shared state store?").Value(&ec.StateEnabled),
		huh.NewConfirm().Title("Enable shared task board?").Value(&ec.TasksEnabled),
	)).Run()
}

// editMCPServers manages the MCP servers list (Add/Edit/Remove/Back).
func editMCPServers(ec *editorConfig) error {
	for {
		opts := []huh.Option[string]{
			huh.NewOption("Add MCP server", "add"),
		}

		for _, m := range ec.MCPServers {
			opts = append(opts, huh.NewOption(fmt.Sprintf("Edit: %s", m.Name), "edit:"+m.Name))
		}

		for _, m := range ec.MCPServers {
			opts = append(opts, huh.NewOption(fmt.Sprintf("Remove: %s", m.Name), "remove:"+m.Name))
		}

		opts = append(opts, huh.NewOption("Back", "back"))

		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("MCP Servers").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return err
		}

		switch {
		case choice == "add":
			m := editorMCP{}
			if err := editMCPServerForm(&m); err != nil {
				return err
			}

			ec.MCPServers = append(ec.MCPServers, m)
		case strings.HasPrefix(choice, "edit:"):
			name := strings.TrimPrefix(choice, "edit:")

			for i := range ec.MCPServers {
				if ec.MCPServers[i].Name == name {
					if err := editMCPServerForm(&ec.MCPServers[i]); err != nil {
						return err
					}

					break
				}
			}
		case strings.HasPrefix(choice, "remove:"):
			name := strings.TrimPrefix(choice, "remove:")

			for i, m := range ec.MCPServers {
				if m.Name == name {
					ec.MCPServers = append(ec.MCPServers[:i], ec.MCPServers[i+1:]...)

					break
				}
			}
		case choice == "back":
			return nil
		}
	}
}

// editMCPServerForm shows a pre-filled form for editing a single MCP server.
func editMCPServerForm(m *editorMCP) error {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Server name").Value(&m.Name),
		huh.NewInput().Title("Command").Value(&m.Command),
		huh.NewInput().Title("Arguments (space-separated)").Value(&m.Args),
	)).Run()
}
