package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/germanamz/shelly/pkg/engine"
	"gopkg.in/yaml.v3"
)

// Editor working types.

type editorProvider struct {
	Kind          string
	Name          string
	BaseURL       string
	APIKey        string //nolint:gosec // env var reference, not a secret
	Model         string
	ContextWindow string // empty = use default, "0" = disable compaction, positive = explicit
	InputTPM      string
	OutputTPM     string
	RPM           string
	MaxRetries    string
	BaseDelay     string
}

type editorMCP struct {
	Name    string
	Command string
	Args    string // space-separated arguments
	URL     string // SSE endpoint URL (mutually exclusive with Command)
}

type editorAgent struct {
	Name               string
	Description        string
	Instructions       string
	Provider           string
	MaxIterations      int
	MaxDelegationDepth int
	Toolboxes          []string
}

type editorConfig struct {
	Providers             []editorProvider
	Agents                []editorAgent
	EntryAgent            string
	MCPServers            []editorMCP
	PermissionsFile       string
	GitWorkDir            string
	DefaultContextWindows map[string]int
}

// YAML output types (preserve all fields via omitempty).

type editorConfigYAML struct {
	Providers             []editorProviderYAML `yaml:"providers"`
	MCPServers            []mcpYAML            `yaml:"mcp_servers,omitempty"`
	Agents                []editorAgentYAML    `yaml:"agents"`
	EntryAgent            string               `yaml:"entry_agent"`
	DefaultContextWindows map[string]int       `yaml:"default_context_windows,omitempty"`
	Filesystem            editorFilesystemYAML `yaml:"filesystem,omitempty"`
	Git                   editorGitYAML        `yaml:"git,omitempty"`
}

type editorProviderYAML struct {
	Name          string         `yaml:"name"`
	Kind          string         `yaml:"kind"`
	BaseURL       string         `yaml:"base_url,omitempty"`
	APIKey        string         `yaml:"api_key"` //nolint:gosec // env var reference, not a secret
	Model         string         `yaml:"model"`
	ContextWindow *int           `yaml:"context_window,omitempty"`
	RateLimit     *rateLimitYAML `yaml:"rate_limit,omitempty"`
}

type editorAgentYAML struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	Instructions string        `yaml:"instructions"`
	Provider     string        `yaml:"provider"`
	Toolboxes    []string      `yaml:"toolboxes,omitempty"`
	Options      agentOptsYAML `yaml:"options"`
}

type editorFilesystemYAML struct {
	PermissionsFile string `yaml:"permissions_file,omitempty"`
}

type editorGitYAML struct {
	WorkDir string `yaml:"work_dir,omitempty"`
}

type mcpYAML struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command,omitempty"`
	Args    []string `yaml:"args,omitempty"`
	URL     string   `yaml:"url,omitempty"`
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
		EntryAgent:            cfg.EntryAgent,
		PermissionsFile:       cfg.Filesystem.PermissionsFile,
		GitWorkDir:            cfg.Git.WorkDir,
		DefaultContextWindows: cfg.DefaultContextWindows,
	}

	for _, p := range cfg.Providers {
		ep := editorProvider{
			Kind:    p.Kind,
			Name:    p.Name,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
		}

		if p.ContextWindow != nil {
			ep.ContextWindow = strconv.Itoa(*p.ContextWindow)
		}

		if p.RateLimit.InputTPM > 0 {
			ep.InputTPM = strconv.Itoa(p.RateLimit.InputTPM)
		}

		if p.RateLimit.OutputTPM > 0 {
			ep.OutputTPM = strconv.Itoa(p.RateLimit.OutputTPM)
		}

		if p.RateLimit.RPM > 0 {
			ep.RPM = strconv.Itoa(p.RateLimit.RPM)
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
			Toolboxes:          a.Toolboxes,
		})
	}

	for _, m := range cfg.MCPServers {
		ec.MCPServers = append(ec.MCPServers, editorMCP{
			Name:    m.Name,
			Command: m.Command,
			Args:    strings.Join(m.Args, " "),
			URL:     m.URL,
		})
	}

	return ec
}

// editorToEngineConfig converts the editor working model back to engine.Config
// for validation.
func editorToEngineConfig(ec editorConfig) engine.Config {
	cfg := engine.Config{
		EntryAgent:            ec.EntryAgent,
		Filesystem:            engine.FilesystemConfig{PermissionsFile: ec.PermissionsFile},
		Git:                   engine.GitConfig{WorkDir: ec.GitWorkDir},
		DefaultContextWindows: ec.DefaultContextWindows,
	}

	for _, p := range ec.Providers {
		inputTPM, _ := strconv.Atoi(p.InputTPM)
		outputTPM, _ := strconv.Atoi(p.OutputTPM)
		rpm, _ := strconv.Atoi(p.RPM)
		maxRetries, _ := strconv.Atoi(p.MaxRetries)

		var contextWindow *int
		if p.ContextWindow != "" {
			v, _ := strconv.Atoi(p.ContextWindow)
			contextWindow = &v
		}

		cfg.Providers = append(cfg.Providers, engine.ProviderConfig{
			Name:          p.Name,
			Kind:          p.Kind,
			BaseURL:       p.BaseURL,
			APIKey:        p.APIKey,
			Model:         p.Model,
			ContextWindow: contextWindow,
			RateLimit: engine.RateLimitConfig{
				InputTPM:   inputTPM,
				OutputTPM:  outputTPM,
				RPM:        rpm,
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
			Toolboxes:    a.Toolboxes,
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
			URL:     m.URL,
		})
	}

	return cfg
}

// marshalEditorConfig serializes the editor config to YAML bytes.
func marshalEditorConfig(ec editorConfig) ([]byte, error) {
	yc := editorConfigYAML{
		EntryAgent:            ec.EntryAgent,
		DefaultContextWindows: ec.DefaultContextWindows,
		Filesystem: editorFilesystemYAML{
			PermissionsFile: ec.PermissionsFile,
		},
		Git: editorGitYAML{
			WorkDir: ec.GitWorkDir,
		},
	}

	for _, p := range ec.Providers {
		py := editorProviderYAML{
			Name:    p.Name,
			Kind:    p.Kind,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
		}

		if p.ContextWindow != "" {
			v, _ := strconv.Atoi(p.ContextWindow)
			py.ContextWindow = &v
		}

		inputTPM, _ := strconv.Atoi(p.InputTPM)
		outputTPM, _ := strconv.Atoi(p.OutputTPM)
		rpm, _ := strconv.Atoi(p.RPM)
		maxRetries, _ := strconv.Atoi(p.MaxRetries)

		if inputTPM > 0 || outputTPM > 0 || rpm > 0 || maxRetries > 0 || p.BaseDelay != "" {
			py.RateLimit = &rateLimitYAML{
				InputTPM:   inputTPM,
				OutputTPM:  outputTPM,
				RPM:        rpm,
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
			Toolboxes:    a.Toolboxes,
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
			URL:     m.URL,
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
					huh.NewOption("MCP Servers", "mcp_servers"),
					huh.NewOption("Default Context Windows", "context_windows"),
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
		case "mcp_servers":
			if err := editMCPServers(ec); err != nil {
				return err
			}
		case "context_windows":
			if err := editDefaultContextWindows(ec); err != nil {
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
	cwTitle := "Context window (empty = default, 0 = no compaction)"
	if builtin, ok := engine.BuiltinContextWindows[p.Kind]; ok {
		cwTitle = fmt.Sprintf("Context window (empty = default: %d, 0 = no compaction)", builtin)
	}

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
			huh.NewInput().Title(cwTitle).Value(&p.ContextWindow).Validate(validateOptionalNonNegativeInt),
		),
		huh.NewGroup(
			huh.NewInput().Title("Input tokens per minute (0 = no limit)").Value(&p.InputTPM).Validate(validateOptionalNonNegativeInt),
			huh.NewInput().Title("Output tokens per minute (0 = no limit)").Value(&p.OutputTPM).Validate(validateOptionalNonNegativeInt),
			huh.NewInput().Title("Requests per minute (0 = no limit)").Value(&p.RPM).Validate(validateOptionalNonNegativeInt),
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

			if err := editAgentForm(&a, providerNames); err != nil {
				return err
			}

			if err := editAgentToolboxes(&a, mcpNames); err != nil {
				return err
			}

			ec.Agents = append(ec.Agents, a)
		case strings.HasPrefix(choice, "edit:"):
			name := strings.TrimPrefix(choice, "edit:")

			for i := range ec.Agents {
				if ec.Agents[i].Name == name {
					if err := editAgentMenu(&ec.Agents[i], providerNames, mcpNames); err != nil {
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

// editAgentMenu shows a sub-menu for editing different aspects of an agent.
func editAgentMenu(a *editorAgent, providerNames, mcpNames []string) error {
	for {
		var choice string

		err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Edit Agent: %s", a.Name)).
				Options(
					huh.NewOption("Details", "details"),
					huh.NewOption("Toolboxes", "toolboxes"),
					huh.NewOption("Back", "back"),
				).
				Value(&choice),
		)).Run()
		if err != nil {
			return err
		}

		switch choice {
		case "details":
			if err := editAgentForm(a, providerNames); err != nil {
				return err
			}
		case "toolboxes":
			if err := editAgentToolboxes(a, mcpNames); err != nil {
				return err
			}
		case "back":
			return nil
		}
	}
}

// editAgentForm shows a pre-filled form for editing agent details.
func editAgentForm(a *editorAgent, providerNames []string) error {
	provOpts := make([]huh.Option[string], len(providerNames))
	for i, n := range providerNames {
		provOpts[i] = huh.NewOption(n, n)
	}

	maxIter := strconv.Itoa(a.MaxIterations)
	maxDepth := strconv.Itoa(a.MaxDelegationDepth)

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

	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}

	a.MaxIterations, _ = strconv.Atoi(maxIter)
	a.MaxDelegationDepth, _ = strconv.Atoi(maxDepth)

	return nil
}

// editAgentToolboxes shows a multi-select form for editing an agent's toolboxes.
func editAgentToolboxes(a *editorAgent, mcpNames []string) error {
	builtinToolboxes := []string{"filesystem", "exec", "search", "git", "http", "browser", "notes", "state", "tasks"}

	selectedSet := make(map[string]bool, len(a.Toolboxes))
	for _, tb := range a.Toolboxes {
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

	if len(toolboxOpts) == 0 {
		fmt.Println("No toolboxes available.")

		return nil
	}

	return huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Toolboxes").
			Options(toolboxOpts...).
			Value(&a.Toolboxes),
	)).Run()
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
		huh.NewInput().Title("Command (leave empty for SSE)").Value(&m.Command),
		huh.NewInput().Title("Arguments (space-separated)").Value(&m.Args),
		huh.NewInput().Title("SSE URL (leave empty for command)").Value(&m.URL),
	)).Run()
}

// editDefaultContextWindows lets the user override built-in context windows per kind.
func editDefaultContextWindows(ec *editorConfig) error {
	if ec.DefaultContextWindows == nil {
		ec.DefaultContextWindows = make(map[string]int)
	}

	for {
		opts := []huh.Option[string]{}

		// Known kinds in stable order.
		knownKinds := []string{"anthropic", "openai", "grok"}
		for _, kind := range knownKinds {
			builtin := engine.BuiltinContextWindows[kind]
			var label string
			if v, ok := ec.DefaultContextWindows[kind]; ok {
				label = fmt.Sprintf("%s (built-in: %d, override: %d)", kind, builtin, v)
			} else {
				label = fmt.Sprintf("%s (built-in: %d)", kind, builtin)
			}
			opts = append(opts, huh.NewOption(label, kind))
		}

		// Custom kinds already in the map.
		var customKinds []string
		for kind := range ec.DefaultContextWindows {
			if kind == "anthropic" || kind == "openai" || kind == "grok" {
				continue
			}
			customKinds = append(customKinds, kind)
		}
		sort.Strings(customKinds)
		for _, kind := range customKinds {
			label := fmt.Sprintf("%s (override: %d)", kind, ec.DefaultContextWindows[kind])
			opts = append(opts, huh.NewOption(label, kind))
		}

		opts = append(opts,
			huh.NewOption("Add custom kind", "add"),
			huh.NewOption("Back", "back"),
		)

		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Default Context Windows").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return err
		}

		switch choice {
		case "back":
			// Clean up empty map so it's omitted from YAML.
			if len(ec.DefaultContextWindows) == 0 {
				ec.DefaultContextWindows = nil
			}
			return nil
		case "add":
			var kind, value string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewInput().Title("Provider kind").Value(&kind),
				huh.NewInput().Title("Default context window").Value(&value).Validate(validatePositiveInt),
			)).Run(); err != nil {
				return err
			}
			v, _ := strconv.Atoi(value)
			ec.DefaultContextWindows[kind] = v
		default:
			kind := choice
			actionOpts := []huh.Option[string]{
				huh.NewOption("Set override", "set"),
			}
			if _, ok := ec.DefaultContextWindows[kind]; ok {
				actionOpts = append(actionOpts, huh.NewOption("Remove override", "remove"))
			}
			actionOpts = append(actionOpts, huh.NewOption("Back", "back"))

			var action string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().Title(fmt.Sprintf("Edit: %s", kind)).Options(actionOpts...).Value(&action),
			)).Run(); err != nil {
				return err
			}

			switch action {
			case "set":
				current := ""
				if v, ok := ec.DefaultContextWindows[kind]; ok {
					current = strconv.Itoa(v)
				}
				if err := huh.NewForm(huh.NewGroup(
					huh.NewInput().Title("Context window").Value(&current).Validate(validatePositiveInt),
				)).Run(); err != nil {
					return err
				}
				v, _ := strconv.Atoi(current)
				ec.DefaultContextWindows[kind] = v
			case "remove":
				delete(ec.DefaultContextWindows, kind)
			}
		}
	}
}
