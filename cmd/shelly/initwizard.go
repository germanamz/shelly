package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/germanamz/shelly/pkg/engine"
	"gopkg.in/yaml.v3"
)

type wizardProvider struct {
	Kind          string
	Name          string
	APIKey        string //nolint:gosec // env var reference, not a secret
	Model         string
	ContextWindow string // empty = use default, "0" = disable compaction, positive = explicit
	InputTPM      string
	OutputTPM     string
	RPM           string
	MaxRetries    string
	BaseDelay     string
}

type wizardEffect struct {
	Kind   string
	Params map[string]any
}

type wizardAgent struct {
	Name               string
	Description        string
	Instructions       string
	Provider           string
	MaxIterations      int
	MaxDelegationDepth int
	Toolboxes          []string
	Skills             []string
	Effects            []wizardEffect
}

// skillFile describes a skill to create on disk during init.
type skillFile struct {
	Name    string // Folder name (e.g. "orchestrator-workflow").
	Content string // Full SKILL.md content (frontmatter + body).
}

type wizardConfig struct {
	Providers             []wizardProvider
	Agents                []wizardAgent
	EntryAgent            string
	SkillFiles            []skillFile
	DefaultContextWindows map[string]int
}

// wizardResult is the output of a wizard run: config YAML + optional skill files.
type wizardResult struct {
	ConfigYAML []byte
	SkillFiles []skillFile
}

type providerDefault struct {
	APIKey string //nolint:gosec // env var reference template, not a secret
	Model  string
}

//nolint:gosec // env var reference templates, not hardcoded secrets
var providerDefaults = map[string]providerDefault{
	"anthropic": {APIKey: "${ANTHROPIC_API_KEY}", Model: "claude-sonnet-4-20250514"},
	"openai":    {APIKey: "${OPENAI_API_KEY}", Model: "gpt-4o-mini"},
	"grok":      {APIKey: "${GROK_API_KEY}", Model: "grok-3-mini-fast-beta"},
}

func runWizard() (wizardResult, error) {
	var cfg wizardConfig

	if err := wizardProviders(&cfg); err != nil {
		return wizardResult{}, err
	}

	if err := wizardAgents(&cfg); err != nil {
		return wizardResult{}, err
	}

	if err := wizardEntryAgent(&cfg); err != nil {
		return wizardResult{}, err
	}

	data, err := marshalWizardConfig(cfg)
	if err != nil {
		return wizardResult{}, err
	}

	return wizardResult{ConfigYAML: data}, nil
}

func wizardProviders(cfg *wizardConfig) error {
	for {
		p, err := wizardPromptProvider()
		if err != nil {
			return err
		}

		cfg.Providers = append(cfg.Providers, p)

		var more bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Add another provider?").Value(&more),
		)).Run(); err != nil {
			return err
		}

		if !more {
			return nil
		}
	}
}

func wizardPromptProvider() (wizardProvider, error) {
	var p wizardProvider

	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Provider kind").
			Options(
				huh.NewOption("Anthropic", "anthropic"),
				huh.NewOption("OpenAI", "openai"),
				huh.NewOption("Grok", "grok"),
			).
			Value(&p.Kind),
	)).Run(); err != nil {
		return p, err
	}

	defaults := providerDefaults[p.Kind]
	p.Name = p.Kind
	p.APIKey = defaults.APIKey
	p.Model = defaults.Model

	cwTitle := "Context window (empty = default, 0 = no compaction)"
	if builtin, ok := engine.BuiltinContextWindows[p.Kind]; ok {
		cwTitle = fmt.Sprintf("Context window (empty = default: %d, 0 = no compaction)", builtin)
	}

	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Provider name").Value(&p.Name),
		huh.NewInput().Title("API key env var").Value(&p.APIKey),
		huh.NewInput().Title("Model").Value(&p.Model),
		huh.NewInput().Title(cwTitle).Value(&p.ContextWindow).Validate(validateOptionalNonNegativeInt),
	)).Run(); err != nil {
		return p, err
	}

	var configRL bool
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Configure rate limiting?").Value(&configRL),
	)).Run(); err != nil {
		return p, err
	}

	if configRL {
		p.InputTPM = "0"
		p.OutputTPM = "0"
		p.RPM = "0"
		p.MaxRetries = "3"
		p.BaseDelay = "1s"

		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Input tokens per minute (0 = no limit)").Value(&p.InputTPM).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Output tokens per minute (0 = no limit)").Value(&p.OutputTPM).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Requests per minute (0 = no limit)").Value(&p.RPM).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Max retries on 429").Value(&p.MaxRetries).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Base backoff delay (e.g. 1s, 500ms)").Value(&p.BaseDelay).Validate(validateDuration),
		)).Run(); err != nil {
			return p, err
		}
	}

	return p, nil
}

func wizardAgents(cfg *wizardConfig) error {
	providerNames := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		providerNames[i] = p.Name
	}

	for {
		a, err := wizardPromptAgent(providerNames)
		if err != nil {
			return err
		}

		cfg.Agents = append(cfg.Agents, a)

		var more bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Add another agent?").Value(&more),
		)).Run(); err != nil {
			return err
		}

		if !more {
			return nil
		}
	}
}

func wizardPromptAgent(providerNames []string) (wizardAgent, error) {
	a := wizardAgent{
		Name:               "assistant",
		Description:        "A helpful assistant",
		Instructions:       "You are a helpful assistant. Be concise and accurate.",
		MaxIterations:      10,
		MaxDelegationDepth: 2,
		Toolboxes:          []string{"filesystem", "exec", "search", "git", "http", "state", "tasks", "notes"},
	}

	if len(providerNames) > 0 {
		a.Provider = providerNames[0]
	}

	opts := make([]huh.Option[string], len(providerNames))
	for i, n := range providerNames {
		opts[i] = huh.NewOption(n, n)
	}

	maxIter := strconv.Itoa(a.MaxIterations)
	maxDepth := strconv.Itoa(a.MaxDelegationDepth)

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Agent name").Value(&a.Name),
		huh.NewInput().Title("Description").Value(&a.Description),
		huh.NewText().Title("Instructions").Value(&a.Instructions),
		huh.NewSelect[string]().Title("Provider").Options(opts...).Value(&a.Provider),
		huh.NewInput().Title("Max iterations").Value(&maxIter).Validate(validatePositiveInt),
		huh.NewInput().Title("Max delegation depth").Value(&maxDepth).Validate(validateNonNegativeInt),
		huh.NewMultiSelect[string]().
			Title("Toolboxes").
			Options(
				huh.NewOption("Filesystem", "filesystem").Selected(true),
				huh.NewOption("Exec", "exec").Selected(true),
				huh.NewOption("Search", "search").Selected(true),
				huh.NewOption("Git", "git").Selected(true),
				huh.NewOption("HTTP", "http").Selected(true),
				huh.NewOption("State", "state").Selected(true),
				huh.NewOption("Tasks", "tasks").Selected(true),
				huh.NewOption("Notes", "notes").Selected(true),
			).
			Value(&a.Toolboxes),
	)).Run()
	if err != nil {
		return a, err
	}

	a.MaxIterations, _ = strconv.Atoi(maxIter)
	a.MaxDelegationDepth, _ = strconv.Atoi(maxDepth)

	return a, nil
}

func wizardEntryAgent(cfg *wizardConfig) error {
	if len(cfg.Agents) == 1 {
		cfg.EntryAgent = cfg.Agents[0].Name
		return nil
	}

	opts := make([]huh.Option[string], len(cfg.Agents))
	for i, a := range cfg.Agents {
		opts[i] = huh.NewOption(a.Name, a.Name)
	}

	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Which agent should be the entry point?").
			Options(opts...).
			Value(&cfg.EntryAgent),
	)).Run()
}

func validatePositiveInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fmt.Errorf("must be a positive integer")
	}

	return nil
}

func validateNonNegativeInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return fmt.Errorf("must be a non-negative integer")
	}

	return nil
}

func validateDuration(s string) error {
	if s == "" {
		return nil
	}

	if _, err := time.ParseDuration(s); err != nil {
		return fmt.Errorf("must be a valid duration (e.g. 1s, 500ms)")
	}

	return nil
}

// YAML output types.

type configYAML struct {
	Providers             []providerYAML `yaml:"providers"`
	Agents                []agentYAML    `yaml:"agents"`
	EntryAgent            string         `yaml:"entry_agent"`
	DefaultContextWindows map[string]int `yaml:"default_context_windows,omitempty"`
}

type providerYAML struct {
	Name          string         `yaml:"name"`
	Kind          string         `yaml:"kind"`
	APIKey        string         `yaml:"api_key"` //nolint:gosec // env var reference, not a secret
	Model         string         `yaml:"model"`
	ContextWindow *int           `yaml:"context_window,omitempty"`
	RateLimit     *rateLimitYAML `yaml:"rate_limit,omitempty"`
}

type rateLimitYAML struct {
	InputTPM   int    `yaml:"input_tpm,omitempty"`
	OutputTPM  int    `yaml:"output_tpm,omitempty"`
	RPM        int    `yaml:"rpm,omitempty"`
	MaxRetries int    `yaml:"max_retries,omitempty"`
	BaseDelay  string `yaml:"base_delay,omitempty"`
}

type effectYAML struct {
	Kind   string         `yaml:"kind"`
	Params map[string]any `yaml:"params,omitempty"`
}

type agentYAML struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	Instructions string        `yaml:"instructions"`
	Provider     string        `yaml:"provider"`
	Toolboxes    []string      `yaml:"toolboxes,omitempty"`
	Skills       []string      `yaml:"skills,omitempty"`
	Effects      []effectYAML  `yaml:"effects,omitempty"`
	Options      agentOptsYAML `yaml:"options"`
}

type agentOptsYAML struct {
	MaxIterations      int `yaml:"max_iterations"`
	MaxDelegationDepth int `yaml:"max_delegation_depth"`
}

func marshalWizardConfig(cfg wizardConfig) ([]byte, error) {
	yc := configYAML{
		EntryAgent:            cfg.EntryAgent,
		DefaultContextWindows: cfg.DefaultContextWindows,
	}

	for _, p := range cfg.Providers {
		py := providerYAML{
			Name:   p.Name,
			Kind:   p.Kind,
			APIKey: p.APIKey,
			Model:  p.Model,
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

	for _, a := range cfg.Agents {
		ay := agentYAML{
			Name:         a.Name,
			Description:  a.Description,
			Instructions: a.Instructions,
			Provider:     a.Provider,
			Toolboxes:    a.Toolboxes,
			Skills:       a.Skills,
			Options: agentOptsYAML{
				MaxIterations:      a.MaxIterations,
				MaxDelegationDepth: a.MaxDelegationDepth,
			},
		}

		for _, e := range a.Effects {
			ay.Effects = append(ay.Effects, effectYAML(e))
		}

		yc.Agents = append(yc.Agents, ay)
	}

	return yaml.Marshal(yc)
}
