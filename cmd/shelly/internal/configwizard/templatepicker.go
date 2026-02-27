package configwizard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/cmd/shelly/internal/templates"
)

// TemplatePickerModel is a bubbletea model for interactive template selection.
type TemplatePickerModel struct {
	shellyDir string
	force     bool
	templates []templates.TemplateMeta
	cursor    int
	done      bool
	err       string
}

// NewTemplatePickerModel creates a new template picker.
func NewTemplatePickerModel(shellyDir string, force bool) TemplatePickerModel {
	return TemplatePickerModel{
		shellyDir: shellyDir,
		force:     force,
		templates: templates.List(),
	}
}

func (m TemplatePickerModel) Init() tea.Cmd {
	return nil
}

func (m TemplatePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.templates)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.templates) == 0 {
				return m, nil
			}
			selected := m.templates[m.cursor]
			t, err := templates.Get(selected.Name)
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			if err := templates.Apply(t, m.shellyDir, m.force); err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m TemplatePickerModel) View() tea.View {
	if m.done {
		selected := m.templates[m.cursor]
		return tea.NewView(styles.AskTabDone.Render(
			fmt.Sprintf("Initialized %q template in %s", selected.Name, m.shellyDir),
		) + "\n")
	}

	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorAccent)
	b.WriteString(title.Render("Select a template"))
	b.WriteString("\n\n")

	if len(m.templates) == 0 {
		b.WriteString(styles.DimStyle.Render("No templates available"))
		b.WriteString("\n")
	} else {
		for i, t := range m.templates {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			line := fmt.Sprintf("%s%s", cursor, t.Name)
			if i == m.cursor {
				b.WriteString(styles.AskSelStyle.Render(line))
				b.WriteString("  ")
				b.WriteString(styles.DimStyle.Render(t.Description))
			} else {
				b.WriteString(styles.AskOptStyle.Render(line))
			}
			b.WriteString("\n")
		}
	}

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(styles.ToolErrorStyle.Render("Error: " + m.err))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.DimStyle.Render("↑/↓: navigate  Enter: select  q: quit"))

	return tea.NewView(b.String())
}
