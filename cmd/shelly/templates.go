package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

type templateProviderSlot struct {
	Name        string
	Description string
}

type templateEffect struct {
	Kind   string
	Params map[string]any
}

type templateAgent struct {
	Name               string
	Description        string
	Instructions       string
	ProviderSlot       string
	MaxIterations      int
	MaxDelegationDepth int
	Toolboxes          []string
	Effects            []templateEffect
}

type configTemplate struct {
	Name        string
	Description string
	Slots       []templateProviderSlot
	Agents      []templateAgent
	EntryAgent  string
}

var simpleAssistantTemplate = configTemplate{
	Name:        "simple-assistant",
	Description: "Single agent with all toolboxes — great starting point",
	Slots: []templateProviderSlot{
		{Name: "primary", Description: "The LLM provider for the assistant"},
	},
	Agents: []templateAgent{
		{
			Name:               "assistant",
			Description:        "A helpful assistant",
			Instructions:       "You are a helpful assistant. Be concise and accurate.",
			ProviderSlot:       "primary",
			MaxIterations:      10,
			MaxDelegationDepth: 2,
			Toolboxes:          []string{"filesystem", "exec", "search", "git", "http", "state", "tasks", "notes"},
		},
	},
	EntryAgent: "assistant",
}

var devTeamTemplate = configTemplate{
	Name:        "dev-team",
	Description: "Orchestrator + planner + coder — multi-agent dev workflow",
	Slots: []templateProviderSlot{
		{Name: "primary", Description: "Main LLM for orchestration and planning"},
		{Name: "fast", Description: "Fast LLM for coding tasks"},
	},
	Agents: []templateAgent{
		{
			Name:               "orchestrator",
			Description:        "Delegates tasks to planner and coder",
			Instructions:       "You are a task orchestrator. Break down user requests into planning and coding tasks, then delegate to the appropriate agent.",
			ProviderSlot:       "primary",
			MaxIterations:      15,
			MaxDelegationDepth: 3,
			Toolboxes:          []string{"state", "tasks"},
		},
		{
			Name:               "planner",
			Description:        "Analyzes code and creates implementation plans",
			Instructions:       "You are a code planner. Analyze the codebase, understand the architecture, and create detailed implementation plans.",
			ProviderSlot:       "primary",
			MaxIterations:      10,
			MaxDelegationDepth: 0,
			Toolboxes:          []string{"filesystem", "search", "git", "state", "tasks", "notes"},
		},
		{
			Name:               "coder",
			Description:        "Implements code changes",
			Instructions:       "You are a coder. Implement changes based on plans, write clean code, and run tests to verify your work.",
			ProviderSlot:       "fast",
			MaxIterations:      20,
			MaxDelegationDepth: 0,
			Toolboxes:          []string{"filesystem", "exec", "search", "git", "http", "state", "tasks", "notes"},
			Effects: []templateEffect{
				{Kind: "trim_tool_results"},
				{Kind: "compact", Params: map[string]any{"threshold": 0.8}},
			},
		},
	},
	EntryAgent: "orchestrator",
}

var templates = []configTemplate{simpleAssistantTemplate, devTeamTemplate}

func findTemplate(name string) *configTemplate {
	for i := range templates {
		if templates[i].Name == name {
			return &templates[i]
		}
	}

	return nil
}

func listTemplates() []configTemplate {
	return templates
}

func printTemplateList() {
	fmt.Println("Available templates:")
	fmt.Println()

	for _, t := range templates {
		fmt.Printf("  %s\n", t.Name)
		fmt.Printf("    %s\n", t.Description)

		fmt.Printf("    Slots:  ")
		for i, s := range t.Slots {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s (%s)", s.Name, s.Description)
		}
		fmt.Println()

		fmt.Printf("    Agents: ")
		for i, a := range t.Agents {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(a.Name)
		}
		fmt.Println()
		fmt.Println()
	}
}

func wizardSlotMapping(slots []templateProviderSlot, providerNames []string) (map[string]string, error) {
	mapping := make(map[string]string, len(slots))

	// Auto-map when there's exactly 1 slot and 1 provider.
	if len(slots) == 1 && len(providerNames) == 1 {
		mapping[slots[0].Name] = providerNames[0]
		return mapping, nil
	}

	opts := make([]huh.Option[string], len(providerNames))
	for i, n := range providerNames {
		opts[i] = huh.NewOption(n, n)
	}

	for _, slot := range slots {
		var selected string
		if len(providerNames) > 0 {
			selected = providerNames[0]
		}

		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Provider for %q slot (%s)", slot.Name, slot.Description)).
				Options(opts...).
				Value(&selected),
		)).Run(); err != nil {
			return nil, err
		}

		mapping[slot.Name] = selected
	}

	return mapping, nil
}

func applyTemplate(tmpl *configTemplate, providers []wizardProvider, slotMapping map[string]string) wizardConfig {
	cfg := wizardConfig{
		Providers:  providers,
		EntryAgent: tmpl.EntryAgent,
	}

	for _, ta := range tmpl.Agents {
		a := wizardAgent{
			Name:               ta.Name,
			Description:        ta.Description,
			Instructions:       ta.Instructions,
			Provider:           slotMapping[ta.ProviderSlot],
			MaxIterations:      ta.MaxIterations,
			MaxDelegationDepth: ta.MaxDelegationDepth,
			Toolboxes:          ta.Toolboxes,
		}

		for _, te := range ta.Effects {
			a.Effects = append(a.Effects, wizardEffect(te))
		}

		cfg.Agents = append(cfg.Agents, a)
	}

	return cfg
}

func runTemplateWizard(tmpl *configTemplate) ([]byte, error) {
	var cfg wizardConfig

	if err := wizardProviders(&cfg); err != nil {
		return nil, err
	}

	providerNames := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		providerNames[i] = p.Name
	}

	slotMapping, err := wizardSlotMapping(tmpl.Slots, providerNames)
	if err != nil {
		return nil, err
	}

	result := applyTemplate(tmpl, cfg.Providers, slotMapping)

	return marshalWizardConfig(result)
}
