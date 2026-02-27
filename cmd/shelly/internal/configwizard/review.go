package configwizard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/engine"
	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------
// Review Screen
// -----------------------------------------------------------------------

// reviewActions defines the action buttons on the review screen.
var reviewActions = []string{"Save", "Save & Quit", "Back"}

type reviewScreen struct {
	cfg        *engine.Config
	configPath string
	shellyDir  string
	scroll     int
	lines      []string
	validErrs  []string
	maxVisible int
	cursor     int
	status     string
}

func newReviewScreen(cfg *engine.Config, configPath, shellyDir string) *reviewScreen {
	s := &reviewScreen{
		cfg:        cfg,
		configPath: configPath,
		shellyDir:  shellyDir,
		maxVisible: 20,
	}
	s.refresh()
	return s
}

func (s *reviewScreen) refresh() {
	data, err := yaml.Marshal(s.cfg)
	if err != nil {
		s.lines = []string{"Error marshaling config: " + err.Error()}
		return
	}
	s.lines = strings.Split(string(data), "\n")

	// Validate.
	s.validErrs = nil
	if err := s.cfg.Validate(); err != nil {
		s.validErrs = []string{err.Error()}
	}
}

// configSavedStayMsg signals a successful save without quitting.
type configSavedStayMsg struct{}

func (s *reviewScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case configSavedStayMsg:
		s.status = "Config saved!"
		return s, nil
	case configSaveErrMsg:
		s.status = "Error: " + msg.err.Error()
		return s, nil
	case tea.KeyMsg:
		s.status = ""
		return s.handleKey(msg)
	}
	return s, nil
}

func (s *reviewScreen) handleKey(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if s.scroll > 0 {
			s.scroll--
		}
	case "down", "j":
		maxScroll := max(len(s.lines)-s.maxVisible, 0)
		if s.scroll < maxScroll {
			s.scroll++
		}
	case "left", "h":
		if s.cursor > 0 {
			s.cursor--
		}
	case "right", "l":
		if s.cursor < len(reviewActions)-1 {
			s.cursor++
		}
	case "enter":
		return s.executeAction()
	case "ctrl+s":
		if len(s.validErrs) > 0 {
			return s, nil
		}
		return s, s.saveStay()
	case "esc":
		return nil, nil
	}
	return s, nil
}

func (s *reviewScreen) executeAction() (screen, tea.Cmd) {
	switch reviewActions[s.cursor] {
	case "Save":
		if len(s.validErrs) > 0 {
			return s, nil
		}
		return s, s.saveStay()
	case "Save & Quit":
		if len(s.validErrs) > 0 {
			return s, nil
		}
		return s, s.saveQuit()
	case "Back":
		return nil, nil
	}
	return s, nil
}

func (s *reviewScreen) saveStay() tea.Cmd {
	cfg := *s.cfg
	path := s.configPath
	shellyDir := s.shellyDir
	return func() tea.Msg {
		if err := SaveConfig(cfg, path, shellyDir); err != nil {
			return configSaveErrMsg{err: err}
		}
		return configSavedStayMsg{}
	}
}

func (s *reviewScreen) saveQuit() tea.Cmd {
	cfg := *s.cfg
	path := s.configPath
	shellyDir := s.shellyDir
	return func() tea.Msg {
		if err := SaveConfig(cfg, path, shellyDir); err != nil {
			return configSaveErrMsg{err: err}
		}
		return configSavedMsg{}
	}
}

func (s *reviewScreen) View() string {
	var b strings.Builder

	title := styles.AskTitleStyle.Render("Review & Save")
	b.WriteString(title + "\n\n")

	// Show validation errors.
	if len(s.validErrs) > 0 {
		for _, e := range s.validErrs {
			b.WriteString(styles.ToolErrorStyle.Render("Validation: " + e))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// YAML preview with scrolling.
	end := min(s.scroll+s.maxVisible, len(s.lines))
	visible := s.lines[s.scroll:end]
	for _, line := range visible {
		b.WriteString(styles.DimStyle.Render(line))
		b.WriteString("\n")
	}

	if len(s.lines) > s.maxVisible {
		b.WriteString("\n")
		b.WriteString(styles.DimStyle.Render(
			fmt.Sprintf("↑/↓: scroll  showing lines %d-%d of %d", s.scroll+1, end, len(s.lines)),
		))
		b.WriteString("\n")
	}

	// Status message.
	if s.status != "" {
		b.WriteString("\n")
		if strings.HasPrefix(s.status, "Error:") {
			b.WriteString(styles.ToolErrorStyle.Render(s.status))
		} else {
			b.WriteString(styles.AskSelStyle.Render(s.status))
		}
		b.WriteString("\n")
	}

	// Action buttons.
	b.WriteString("\n")
	if len(s.validErrs) > 0 {
		b.WriteString(styles.ToolErrorStyle.Render("Fix validation errors before saving"))
	} else {
		for i, action := range reviewActions {
			if i > 0 {
				b.WriteString("  ")
			}
			label := fmt.Sprintf("[ %s ]", action)
			if i == s.cursor {
				b.WriteString(styles.AskSelStyle.Render(label))
			} else {
				b.WriteString(styles.AskOptStyle.Render(label))
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.DimStyle.Render("←/→: select action  Enter: confirm  Ctrl+S: save  Esc: back"))

	return b.String()
}
