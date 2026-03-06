package panel

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// PanelSetSizeMsg resizes the panel.
type PanelSetSizeMsg struct {
	PanelID string
	Width   int
	Height  int
}

// PanelClosedMsg is emitted by consumers when they decide to close a panel.
type PanelClosedMsg struct {
	PanelID string
}

// Model is a generic container that provides visual chrome (border + title)
// around arbitrary content. It wraps any inner content passed as a string to
// View(). The panel only handles chrome, not content or key handling.
type Model struct {
	title   string
	active  bool
	width   int
	height  int // max height including borders (content may be shorter)
	panelID string
}

// New creates a new panel Model.
func New(panelID, title string) Model {
	return Model{
		panelID: panelID,
		title:   title,
	}
}

// PanelID returns the panel's identifier.
func (m Model) PanelID() string { return m.panelID }

// Active returns whether the panel is currently open/visible.
func (m Model) Active() bool { return m.active }

// SetActive sets the panel's open/close state.
func (m *Model) SetActive(active bool) { m.active = active }

// SetSize updates the panel dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Width returns the panel's width.
func (m Model) Width() int { return m.width }

// Height returns the panel's height (including borders).
func (m Model) Height() int { return m.height }

// ContentHeight returns the number of rows available for content (height minus
// top and bottom border lines).
func (m Model) ContentHeight() int {
	return max(m.height-2, 0)
}

// View wraps the given content string in a bordered box with the panel title.
// If content is empty, a centered "No items" message is shown.
// Returns an empty string when the panel is not active.
func (m Model) View(content string) string {
	if !m.active {
		return ""
	}

	innerWidth := max(m.width-2, 0) // subtract left+right border

	if strings.TrimSpace(content) == "" {
		content = m.renderEmpty(innerWidth)
	}

	// Truncate content lines to fit within the content height.
	contentHeight := m.ContentHeight()
	if contentHeight > 0 {
		lines := strings.Split(content, "\n")
		if len(lines) > contentHeight {
			lines = lines[:contentHeight]
		}
		content = strings.Join(lines, "\n")
	}

	border := lipgloss.RoundedBorder()
	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(styles.ColorAccent).
		Width(innerWidth)

	// Build title into top border.
	titleStr := ""
	if m.title != "" {
		titleStr = " " + lipgloss.NewStyle().Bold(true).Foreground(styles.ColorAccent).Render(m.title) + " "
	}

	rendered := style.Render(content)

	// Replace the top border's first segment with the title.
	if titleStr != "" {
		lines := strings.SplitN(rendered, "\n", 2)
		if len(lines) >= 1 {
			top := lines[0]
			runes := []rune(top)
			// Insert title after the top-left corner (first rune).
			if len(runes) > 1 {
				top = string(runes[0]) + titleStr + string(runes[1+min(len([]rune(titleStr)), len(runes)-1):])
			}
			if len(lines) == 2 {
				rendered = top + "\n" + lines[1]
			} else {
				rendered = top
			}
		}
	}

	return rendered
}

// ViewError wraps an error message in the panel chrome with error styling.
// Returns an empty string when the panel is not active.
func (m Model) ViewError(errMsg string) string {
	if !m.active {
		return ""
	}
	innerWidth := max(m.width-2, 0)
	styled := lipgloss.NewStyle().
		Foreground(styles.ColorError).
		Width(innerWidth).
		Align(lipgloss.Center).
		Render(errMsg)
	return m.View(styled)
}

func (m Model) renderEmpty(width int) string {
	return lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Width(width).
		Align(lipgloss.Center).
		Render("No items")
}
