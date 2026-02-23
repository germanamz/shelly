package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/huh"
	"gopkg.in/yaml.v3"
)

type wizardProvider struct {
	Kind       string
	Name       string
	APIKey     string //nolint:gosec // env var reference, not a secret
	Model      string
	TPM        string
	MaxRetries string
	BaseDelay  string
}

type wizardAgent struct {
	Name               string
	Description        string
	Instructions       string
	Provider           string
	MaxIterations      int
	MaxDelegationDepth int
}

type wizardConfig struct {
	Providers    []wizardProvider
	Agents       []wizardAgent
	EntryAgent   string
	Tools        []string
	StateEnabled bool
	TasksEnabled bool
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

func runWizard() ([]byte, error) {
	var cfg wizardConfig

	if err := wizardProviders(&cfg); err != nil {
		return nil, err
	}

	if err := wizardAgents(&cfg); err != nil {
		return nil, err
	}

	if err := wizardEntryAgent(&cfg); err != nil {
		return nil, err
	}

	if err := wizardTools(&cfg); err != nil {
		return nil, err
	}

	if err := wizardFeatures(&cfg); err != nil {
		return nil, err
	}

	return marshalWizardConfig(cfg)
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

	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Provider name").Value(&p.Name),
		huh.NewInput().Title("API key env var").Value(&p.APIKey),
		huh.NewInput().Title("Model").Value(&p.Model),
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
		p.TPM = "0"
		p.MaxRetries = "3"
		p.BaseDelay = "1s"

		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Tokens per minute (0 = no limit)").Value(&p.TPM).Validate(validateNonNegativeInt),
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

func wizardTools(cfg *wizardConfig) error {
	return huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Which built-in tools should be enabled?").
			Options(
				huh.NewOption("Filesystem", "filesystem").Selected(true),
				huh.NewOption("Exec", "exec").Selected(true),
				huh.NewOption("Search", "search").Selected(true),
				huh.NewOption("Git", "git").Selected(true),
				huh.NewOption("HTTP", "http").Selected(true),
			).
			Value(&cfg.Tools),
	)).Run()
}

func wizardFeatures(cfg *wizardConfig) error {
	return huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Enable shared state store?").Value(&cfg.StateEnabled),
		huh.NewConfirm().Title("Enable shared task board?").Value(&cfg.TasksEnabled),
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
	Providers    []providerYAML `yaml:"providers"`
	Agents       []agentYAML    `yaml:"agents"`
	EntryAgent   string         `yaml:"entry_agent"`
	Filesystem   toolYAML       `yaml:"filesystem"`
	Exec         toolYAML       `yaml:"exec"`
	Search       toolYAML       `yaml:"search"`
	Git          toolYAML       `yaml:"git"`
	HTTP         toolYAML       `yaml:"http"`
	StateEnabled bool           `yaml:"state_enabled,omitempty"`
	TasksEnabled bool           `yaml:"tasks_enabled,omitempty"`
}

type providerYAML struct {
	Name      string         `yaml:"name"`
	Kind      string         `yaml:"kind"`
	APIKey    string         `yaml:"api_key"` //nolint:gosec // env var reference, not a secret
	Model     string         `yaml:"model"`
	RateLimit *rateLimitYAML `yaml:"rate_limit,omitempty"`
}

type rateLimitYAML struct {
	TPM        int    `yaml:"tpm,omitempty"`
	MaxRetries int    `yaml:"max_retries,omitempty"`
	BaseDelay  string `yaml:"base_delay,omitempty"`
}

type agentYAML struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	Instructions string        `yaml:"instructions"`
	Provider     string        `yaml:"provider"`
	Options      agentOptsYAML `yaml:"options"`
}

type agentOptsYAML struct {
	MaxIterations      int `yaml:"max_iterations"`
	MaxDelegationDepth int `yaml:"max_delegation_depth"`
}

type toolYAML struct {
	Enabled bool `yaml:"enabled"`
}

func marshalWizardConfig(cfg wizardConfig) ([]byte, error) {
	toolSet := make(map[string]bool, len(cfg.Tools))
	for _, t := range cfg.Tools {
		toolSet[t] = true
	}

	yc := configYAML{
		EntryAgent:   cfg.EntryAgent,
		Filesystem:   toolYAML{Enabled: toolSet["filesystem"]},
		Exec:         toolYAML{Enabled: toolSet["exec"]},
		Search:       toolYAML{Enabled: toolSet["search"]},
		Git:          toolYAML{Enabled: toolSet["git"]},
		HTTP:         toolYAML{Enabled: toolSet["http"]},
		StateEnabled: cfg.StateEnabled,
		TasksEnabled: cfg.TasksEnabled,
	}

	for _, p := range cfg.Providers {
		py := providerYAML{
			Name:   p.Name,
			Kind:   p.Kind,
			APIKey: p.APIKey,
			Model:  p.Model,
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

	for _, a := range cfg.Agents {
		yc.Agents = append(yc.Agents, agentYAML{
			Name:         a.Name,
			Description:  a.Description,
			Instructions: a.Instructions,
			Provider:     a.Provider,
			Options: agentOptsYAML{
				MaxIterations:      a.MaxIterations,
				MaxDelegationDepth: a.MaxDelegationDepth,
			},
		})
	}

	return yaml.Marshal(yc)
}
