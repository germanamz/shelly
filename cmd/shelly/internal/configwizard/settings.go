package configwizard

import (
	"strconv"

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

	// Context window fields — one per known provider kind.
	anthropicCWField := NewIntField("Anthropic Context Window", "default: 200000", false)
	openaiCWField := NewIntField("OpenAI Context Window", "default: 128000", false)
	grokCWField := NewIntField("Grok Context Window", "default: 131072", false)
	geminiCWField := NewIntField("Gemini Context Window", "default: 1048576", false)

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
	if v, ok := cfg.DefaultContextWindows["anthropic"]; ok {
		anthropicCWField.SetValue(strconv.Itoa(v))
	}
	if v, ok := cfg.DefaultContextWindows["openai"]; ok {
		openaiCWField.SetValue(strconv.Itoa(v))
	}
	if v, ok := cfg.DefaultContextWindows["grok"]; ok {
		grokCWField.SetValue(strconv.Itoa(v))
	}
	if v, ok := cfg.DefaultContextWindows["gemini"]; ok {
		geminiCWField.SetValue(strconv.Itoa(v))
	}

	form := NewFormModel("Settings", []FormField{
		entryField, permFileField, gitWorkDirField, headlessField,
		anthropicCWField, openaiCWField, grokCWField, geminiCWField,
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

	// Apply context window fields.
	cwKinds := []string{"anthropic", "openai", "grok", "gemini"}
	for i, kind := range cwKinds {
		if f, ok := s.form.Fields[4+i].(*IntField); ok {
			if v, set := f.IntValue(); set {
				if s.cfg.DefaultContextWindows == nil {
					s.cfg.DefaultContextWindows = make(map[string]int)
				}
				s.cfg.DefaultContextWindows[kind] = v
			} else {
				delete(s.cfg.DefaultContextWindows, kind)
			}
		}
	}
}

func (s *settingsScreen) View() string {
	return s.form.View().Content
}
