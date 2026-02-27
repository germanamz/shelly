package configwizard

import (
	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/pkg/engine"
)

// -----------------------------------------------------------------------
// Settings Screen
// -----------------------------------------------------------------------

type settingsScreen struct {
	cfg  *engine.Config
	form *FormModel
}

func newSettingsScreen(cfg *engine.Config, _ []string) *settingsScreen {
	// Build agent name list for entry_agent selector.
	agentNames := make([]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		agentNames[i] = a.Name
	}
	if len(agentNames) == 0 {
		agentNames = []string{"(no agents)"}
	}

	entryField := NewSelectField("Entry Agent", agentNames)
	permFileField := NewTextField("Permissions File", "e.g. permissions.json", false)
	gitWorkDirField := NewTextField("Git Work Dir", "e.g. .", false)
	headlessField := NewBoolField("Browser Headless")

	// Pre-fill.
	if cfg.EntryAgent != "" {
		entryField.SetValue(cfg.EntryAgent)
	}
	if cfg.Filesystem.PermissionsFile != "" {
		permFileField.SetValue(cfg.Filesystem.PermissionsFile)
	}
	if cfg.Git.WorkDir != "" {
		gitWorkDirField.SetValue(cfg.Git.WorkDir)
	}
	if cfg.Browser.Headless {
		headlessField.SetValue("true")
	}

	form := NewFormModel("Settings", []FormField{
		entryField, permFileField, gitWorkDirField, headlessField,
	})

	return &settingsScreen{cfg: cfg, form: form}
}

func (s *settingsScreen) init() tea.Cmd {
	return s.form.Init()
}

func (s *settingsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg.(type) {
	case formSubmitMsg:
		s.applySettings()
		return nil, nil
	case formCancelMsg:
		return nil, nil
	}

	_, cmd := s.form.Update(msg)
	return s, cmd
}

func (s *settingsScreen) applySettings() {
	s.cfg.EntryAgent = s.form.Fields[0].Value()
	if s.cfg.EntryAgent == "(no agents)" {
		s.cfg.EntryAgent = ""
	}
	s.cfg.Filesystem.PermissionsFile = s.form.Fields[1].Value()
	s.cfg.Git.WorkDir = s.form.Fields[2].Value()
	if f, ok := s.form.Fields[3].(*BoolField); ok {
		s.cfg.Browser.Headless = f.BoolValue()
	}
}

func (s *settingsScreen) View() string {
	return s.form.View().Content
}
