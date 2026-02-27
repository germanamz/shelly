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
// Provider List Screen
// -----------------------------------------------------------------------

type providerListScreen struct {
	cfg    *engine.Config
	kinds  []string
	cursor int
	form   *providerFormScreen
}

func newProviderListScreen(cfg *engine.Config, kinds []string) *providerListScreen {
	return &providerListScreen{cfg: cfg, kinds: kinds}
}

func (s *providerListScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	// Delegate to form if active.
	if s.form != nil {
		updated, cmd := s.form.Update(msg)
		if updated == nil {
			s.form = nil
			return s, nil
		}
		s.form = updated.(*providerFormScreen)
		return s, cmd
	}

	kmsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	n := len(s.cfg.Providers)
	switch kmsg.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		// +1 for the "Add new" entry
		if s.cursor < n {
			s.cursor++
		}
	case "enter":
		if s.cursor < n {
			// Edit existing.
			s.form = newProviderFormScreen(&s.cfg.Providers[s.cursor], s.kinds, false)
		} else {
			// Add new.
			s.cfg.Providers = append(s.cfg.Providers, engine.ProviderConfig{})
			s.form = newProviderFormScreen(&s.cfg.Providers[len(s.cfg.Providers)-1], s.kinds, true)
		}
		return s, s.form.init()
	case "d":
		if s.cursor < n {
			s.cfg.Providers = append(s.cfg.Providers[:s.cursor], s.cfg.Providers[s.cursor+1:]...)
			if s.cursor >= len(s.cfg.Providers) && s.cursor > 0 {
				s.cursor--
			}
		}
	case "esc":
		return nil, nil
	}
	return s, nil
}

func (s *providerListScreen) View() string {
	if s.form != nil {
		return s.form.View()
	}

	var b strings.Builder
	title := styles.AskTitleStyle.Render("Providers")
	b.WriteString(title + "\n\n")

	for i, p := range s.cfg.Providers {
		cursor := "  "
		if i == s.cursor {
			cursor = "> "
		}
		line := fmt.Sprintf("%s%s (%s)", cursor, p.Name, p.Kind)
		if i == s.cursor {
			b.WriteString(styles.AskSelStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Add new entry.
	cursor := "  "
	if s.cursor == len(s.cfg.Providers) {
		cursor = "> "
	}
	addLine := cursor + "+ Add new provider"
	if s.cursor == len(s.cfg.Providers) {
		b.WriteString(styles.AskSelStyle.Render(addLine))
	} else {
		b.WriteString(styles.DimStyle.Render(addLine))
	}
	b.WriteString("\n\n")
	b.WriteString(styles.DimStyle.Render("Enter: edit  d: delete  Esc: back"))

	return b.String()
}

// -----------------------------------------------------------------------
// Provider Form Screen
// -----------------------------------------------------------------------

type providerFormScreen struct {
	provider *engine.ProviderConfig
	form     *FormModel
	isNew    bool
	kinds    []string
}

func newProviderFormScreen(p *engine.ProviderConfig, kinds []string, isNew bool) *providerFormScreen {
	kindField := NewSelectField("Kind", kinds)
	nameField := NewTextField("Name", "e.g. my-anthropic", true)
	apiKeyField := NewTextField("API Key", "${API_KEY}", false)
	modelField := NewTextField("Model", "e.g. claude-opus-4-6", false)
	baseURLField := NewTextField("Base URL", "optional", false)
	ctxWindowField := NewIntField("Context Window", "empty=default, 0=disabled", false)
	maxRetriesField := NewIntField("Rate Limit Retries", "e.g. 3", false)
	baseDelayField := NewTextField("Rate Limit Delay", "e.g. 1s", false)

	// Pre-fill from existing provider.
	if p.Kind != "" {
		kindField.SetValue(p.Kind)
	}
	if p.Name != "" {
		nameField.SetValue(p.Name)
	}
	if p.APIKey != "" {
		apiKeyField.SetValue(p.APIKey)
	}
	if p.Model != "" {
		modelField.SetValue(p.Model)
	}
	if p.BaseURL != "" {
		baseURLField.SetValue(p.BaseURL)
	}
	if p.ContextWindow != nil {
		ctxWindowField.SetValue(strconv.Itoa(*p.ContextWindow))
	}
	if p.RateLimit.MaxRetries > 0 {
		maxRetriesField.SetValue(strconv.Itoa(p.RateLimit.MaxRetries))
	}
	if p.RateLimit.BaseDelay != "" {
		baseDelayField.SetValue(p.RateLimit.BaseDelay)
	}

	title := "Edit Provider"
	if isNew {
		title = "Add Provider"
	}

	form := NewFormModel(title, []FormField{
		kindField, nameField, apiKeyField, modelField,
		baseURLField, ctxWindowField, maxRetriesField, baseDelayField,
	})

	return &providerFormScreen{provider: p, form: form, isNew: isNew, kinds: kinds}
}

func (s *providerFormScreen) init() tea.Cmd {
	return s.form.Init()
}

func (s *providerFormScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg.(type) {
	case formSubmitMsg:
		s.applyToProvider()
		return nil, nil
	case formCancelMsg:
		return nil, nil
	}

	_, cmd := s.form.Update(msg)
	return s, cmd
}

func (s *providerFormScreen) applyToProvider() {
	s.provider.Kind = s.form.Fields[0].Value()
	s.provider.Name = s.form.Fields[1].Value()
	s.provider.APIKey = s.form.Fields[2].Value()
	s.provider.Model = s.form.Fields[3].Value()
	s.provider.BaseURL = s.form.Fields[4].Value()

	// Context window: empty = nil, value = *int.
	if f, ok := s.form.Fields[5].(*IntField); ok {
		if v, set := f.IntValue(); set {
			s.provider.ContextWindow = &v
		} else {
			s.provider.ContextWindow = nil
		}
	}

	// Rate limit fields.
	if f, ok := s.form.Fields[6].(*IntField); ok {
		if v, set := f.IntValue(); set {
			s.provider.RateLimit.MaxRetries = v
		} else {
			s.provider.RateLimit.MaxRetries = 0
		}
	}
	s.provider.RateLimit.BaseDelay = s.form.Fields[7].Value()
}

func (s *providerFormScreen) View() string {
	return s.form.View().Content
}
