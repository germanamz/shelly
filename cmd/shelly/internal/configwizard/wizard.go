package configwizard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/engine"
)

// screen is the interface for each wizard screen.
type screen interface {
	Update(tea.Msg) (screen, tea.Cmd)
	View() string
}

// WizardModel is the root bubbletea model for the config wizard.
type WizardModel struct {
	cfg           engine.Config
	configPath    string
	shellyDir     string
	stack         []screen
	providerKinds []string
	effectKinds   []string
	toolboxNames  []string
	cursor        int
	width         int
	height        int
	saved         bool
	err           string
}

// NewWizardModel creates a wizard, optionally pre-filled from an existing config.
func NewWizardModel(cfg engine.Config, configPath, shellyDir string) WizardModel {
	return WizardModel{
		cfg:           cfg,
		configPath:    configPath,
		shellyDir:     shellyDir,
		providerKinds: engine.KnownProviderKinds(),
		effectKinds:   engine.KnownEffectKinds(),
		toolboxNames:  engine.BuiltinToolboxNames(),
	}
}

func (m WizardModel) Init() tea.Cmd {
	return nil
}

func (m WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// Global quit.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// If we have screens on the stack, delegate.
		if len(m.stack) > 0 {
			top := m.stack[len(m.stack)-1]
			updated, cmd := top.Update(msg)
			if updated == nil {
				// Screen wants to pop itself.
				m.stack = m.stack[:len(m.stack)-1]
				return m, nil
			}
			m.stack[len(m.stack)-1] = updated
			return m, cmd
		}

		// Main menu navigation.
		return m.handleMainMenu(msg)

	case configSavedMsg:
		m.saved = true
		m.err = ""
		return m, tea.Quit
	}

	// Delegate to top screen for non-key messages.
	if len(m.stack) > 0 {
		top := m.stack[len(m.stack)-1]
		updated, cmd := top.Update(msg)
		if updated == nil {
			m.stack = m.stack[:len(m.stack)-1]
			return m, nil
		}
		m.stack[len(m.stack)-1] = updated
		return m, cmd
	}

	return m, nil
}

// menuItems defines the main menu entries.
var menuItems = []string{
	"Providers",
	"Agents",
	"MCP Servers",
	"Settings",
	"Review & Save",
	"Quit",
}

func (m WizardModel) handleMainMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(menuItems)-1 {
			m.cursor++
		}
	case "enter":
		return m.selectMenuItem()
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m WizardModel) selectMenuItem() (tea.Model, tea.Cmd) {
	switch menuItems[m.cursor] {
	case "Providers":
		s := newProviderListScreen(&m.cfg, m.providerKinds)
		m.stack = append(m.stack, s)
	case "Agents":
		s := newAgentListScreen(&m.cfg, m.providerKinds, m.toolboxNames, m.effectKinds)
		m.stack = append(m.stack, s)
	case "MCP Servers":
		s := newMCPListScreen(&m.cfg)
		m.stack = append(m.stack, s)
	case "Settings":
		s := newSettingsScreen(&m.cfg, m.providerKinds)
		m.stack = append(m.stack, s)
		return m, s.init()
	case "Review & Save":
		s := newReviewScreen(&m.cfg, m.configPath, m.shellyDir)
		m.stack = append(m.stack, s)
	case "Quit":
		return m, tea.Quit
	}
	return m, nil
}

func (m WizardModel) View() tea.View {
	if m.saved {
		return tea.NewView(styles.AskTabDone.Render("Config saved successfully!") + "\n")
	}

	if len(m.stack) > 0 {
		return tea.NewView(m.stack[len(m.stack)-1].View())
	}

	return tea.NewView(m.mainMenuView())
}

func (m WizardModel) mainMenuView() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorAccent)
	b.WriteString(title.Render("Shelly Config Wizard"))
	b.WriteString("\n\n")

	summary := m.configSummary()
	if summary != "" {
		b.WriteString(styles.DimStyle.Render(summary))
		b.WriteString("\n\n")
	}

	for i, item := range menuItems {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		if i == m.cursor {
			b.WriteString(styles.AskSelStyle.Render(cursor + item))
		} else {
			b.WriteString(styles.AskOptStyle.Render(cursor + item))
		}
		b.WriteString("\n")
	}

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(styles.ToolErrorStyle.Render("Error: " + m.err))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.DimStyle.Render("↑/↓: navigate  Enter: select  q: quit"))

	return b.String()
}

func (m WizardModel) configSummary() string {
	var parts []string
	if n := len(m.cfg.Providers); n > 0 {
		parts = append(parts, pluralize(n, "provider"))
	}
	if n := len(m.cfg.Agents); n > 0 {
		parts = append(parts, pluralize(n, "agent"))
	}
	if n := len(m.cfg.MCPServers); n > 0 {
		parts = append(parts, pluralize(n, "MCP server"))
	}
	if len(parts) == 0 {
		return "No configuration loaded"
	}
	return strings.Join(parts, ", ")
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// configSavedMsg signals a successful save.
type configSavedMsg struct{}

// configSaveErrMsg signals a save error.
type configSaveErrMsg struct {
	err error
}
