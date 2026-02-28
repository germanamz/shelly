package configwizard

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/engine"
)

// -----------------------------------------------------------------------
// Agent List Screen
// -----------------------------------------------------------------------

type agentListScreen struct {
	cfg          *engine.Config
	provKinds    []string
	toolboxNames []string
	effectKinds  []string
	cursor       int
	form         *agentFormScreen
}

func newAgentListScreen(cfg *engine.Config, provKinds, toolboxNames, effectKinds []string) *agentListScreen {
	return &agentListScreen{
		cfg:          cfg,
		provKinds:    provKinds,
		toolboxNames: toolboxNames,
		effectKinds:  effectKinds,
	}
}

func (s *agentListScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if s.form != nil {
		updated, cmd := s.form.Update(msg)
		if updated == nil {
			s.form = nil
			return s, nil
		}
		if f, ok := updated.(*agentFormScreen); ok {
			s.form = f
		} else {
			s.form = nil
		}
		return s, cmd
	}

	kmsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	n := len(s.cfg.Agents)
	switch kmsg.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < n {
			s.cursor++
		}
	case "enter":
		providerNames := providerNames(s.cfg)
		allToolboxes := make([]string, 0, len(s.toolboxNames)+len(s.cfg.MCPServers))
		allToolboxes = append(allToolboxes, s.toolboxNames...)
		allToolboxes = append(allToolboxes, mcpNames(s.cfg)...)
		if s.cursor < n {
			s.form = newAgentFormScreen(&s.cfg.Agents[s.cursor], providerNames, allToolboxes, s.effectKinds, false)
		} else {
			s.cfg.Agents = append(s.cfg.Agents, engine.AgentConfig{})
			s.form = newAgentFormScreen(&s.cfg.Agents[len(s.cfg.Agents)-1], providerNames, allToolboxes, s.effectKinds, true)
		}
		return s, s.form.init()
	case "d":
		if s.cursor < n {
			s.cfg.Agents = append(s.cfg.Agents[:s.cursor], s.cfg.Agents[s.cursor+1:]...)
			if s.cursor >= len(s.cfg.Agents) && s.cursor > 0 {
				s.cursor--
			}
		}
	case "esc":
		return nil, nil
	}
	return s, nil
}

func (s *agentListScreen) View() string {
	if s.form != nil {
		return s.form.View()
	}

	var b strings.Builder
	title := styles.AskTitleStyle.Render("Agents")
	b.WriteString(title + "\n\n")

	for i, a := range s.cfg.Agents {
		cursor := "  "
		if i == s.cursor {
			cursor = "> "
		}
		desc := a.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		line := fmt.Sprintf("%s%s", cursor, a.Name)
		if desc != "" {
			line += " - " + desc
		}
		if i == s.cursor {
			b.WriteString(styles.AskSelStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	cursor := "  "
	if s.cursor == len(s.cfg.Agents) {
		cursor = "> "
	}
	addLine := cursor + "+ Add new agent"
	if s.cursor == len(s.cfg.Agents) {
		b.WriteString(styles.AskSelStyle.Render(addLine))
	} else {
		b.WriteString(styles.DimStyle.Render(addLine))
	}
	b.WriteString("\n\n")
	b.WriteString(styles.DimStyle.Render("Enter: edit  d: delete  Esc: back"))

	return b.String()
}

// -----------------------------------------------------------------------
// Agent Form Screen
// -----------------------------------------------------------------------

type agentFormScreen struct {
	agent *engine.AgentConfig
	form  *FormModel
	isNew bool
}

func newAgentFormScreen(a *engine.AgentConfig, providerNames, toolboxes, effectKinds []string, isNew bool) *agentFormScreen {
	nameField := NewTextField("Name", "e.g. coder", true)
	descField := NewTextField("Description", "short description", false)
	instrField := NewTextAreaField("Instructions", "system instructions", false)
	provField := NewSelectField("Provider", providerNames)
	prefixField := NewTextField("Prefix", "e.g. emoji", false)
	tbField := NewMultiSelectField("Toolboxes", toolboxes)
	effField := NewMultiSelectField("Effects", effectKinds)
	maxIterField := NewIntField("Max Iterations", "e.g. 20", false)
	maxDelegField := NewIntField("Max Delegation", "e.g. 3", false)
	ctxThreshField := NewFloatField("Context Threshold", "e.g. 0.8", false)

	// Pre-fill.
	if a.Name != "" {
		nameField.SetValue(a.Name)
	}
	if a.Description != "" {
		descField.SetValue(a.Description)
	}
	if a.Instructions != "" {
		instrField.SetValue(a.Instructions)
	}
	if a.Provider != "" {
		provField.SetValue(a.Provider)
	}
	if a.Prefix != "" {
		prefixField.SetValue(a.Prefix)
	}
	if len(a.Toolboxes) > 0 {
		tbField.SetValue(strings.Join(a.Toolboxes, ","))
	}
	if len(a.Effects) > 0 {
		var kinds []string
		for _, e := range a.Effects {
			kinds = append(kinds, e.Kind)
		}
		effField.SetValue(strings.Join(kinds, ","))
	}
	if a.Options.MaxIterations > 0 {
		maxIterField.SetValue(strconv.Itoa(a.Options.MaxIterations))
	}
	if a.Options.MaxDelegationDepth > 0 {
		maxDelegField.SetValue(strconv.Itoa(a.Options.MaxDelegationDepth))
	}
	if a.Options.ContextThreshold > 0 {
		ctxThreshField.SetValue(strconv.FormatFloat(a.Options.ContextThreshold, 'f', -1, 64))
	}

	title := "Edit Agent"
	if isNew {
		title = "Add Agent"
	}

	form := NewFormModel(title, []FormField{
		nameField, descField, instrField, provField, prefixField,
		tbField, effField, maxIterField, maxDelegField, ctxThreshField,
	})

	return &agentFormScreen{agent: a, form: form, isNew: isNew}
}

func (s *agentFormScreen) init() tea.Cmd {
	return s.form.Init()
}

func (s *agentFormScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg.(type) {
	case formSubmitMsg:
		s.applyToAgent()
		return nil, nil
	case formCancelMsg:
		return nil, nil
	}

	_, cmd := s.form.Update(msg)
	return s, cmd
}

func (s *agentFormScreen) applyToAgent() {
	s.agent.Name = s.form.Fields[0].Value()
	s.agent.Description = s.form.Fields[1].Value()
	s.agent.Instructions = s.form.Fields[2].Value()
	s.agent.Provider = s.form.Fields[3].Value()
	s.agent.Prefix = s.form.Fields[4].Value()

	if f, ok := s.form.Fields[5].(*MultiSelectField); ok {
		s.agent.Toolboxes = f.SelectedItems()
	}

	if f, ok := s.form.Fields[6].(*MultiSelectField); ok {
		kinds := f.SelectedItems()
		effects := make([]engine.EffectConfig, len(kinds))
		for i, k := range kinds {
			effects[i] = engine.EffectConfig{Kind: k}
		}
		s.agent.Effects = effects
	}

	if f, ok := s.form.Fields[7].(*IntField); ok {
		if v, set := f.IntValue(); set {
			s.agent.Options.MaxIterations = v
		}
	}
	if f, ok := s.form.Fields[8].(*IntField); ok {
		if v, set := f.IntValue(); set {
			s.agent.Options.MaxDelegationDepth = v
		}
	}
	if f, ok := s.form.Fields[9].(*FloatField); ok {
		if v, set := f.FloatValue(); set {
			s.agent.Options.ContextThreshold = v
		}
	}
}

func (s *agentFormScreen) View() string {
	return s.form.View().Content
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func providerNames(cfg *engine.Config) []string {
	names := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		names[i] = p.Name
	}
	return names
}

func mcpNames(cfg *engine.Config) []string {
	names := make([]string, len(cfg.MCPServers))
	for i, m := range cfg.MCPServers {
		names[i] = m.Name
	}
	return names
}
