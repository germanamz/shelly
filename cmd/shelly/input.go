package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	inputMinHeight = 1
	inputMaxHeight = 5
)

var (
	focusedBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("2")) // green
	disabledBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
)

// inputModel wraps a textarea in a rounded border box.
type inputModel struct {
	textarea textarea.Model
	enabled  bool
	width    int
}

func newInput() inputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.ShowLineNumbers = false
	ta.SetHeight(inputMinHeight)
	ta.CharLimit = 0
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Prompt = lipgloss.NewStyle()
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter"))
	ta.Focus()

	return inputModel{
		textarea: ta,
		enabled:  true,
	}
}

func (m inputModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	if !m.enabled {
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEnter && !keyMsg.Alt {
			text := strings.TrimSpace(m.textarea.Value())
			if text != "" {
				m.textarea.Reset()
				m.textarea.SetHeight(inputMinHeight)
				return m, func() tea.Msg { return inputSubmitMsg{text: text} }
			}
			return m, nil
		}
	}

	// Pre-set max height so the textarea has room and won't scroll its
	// viewport during Update. After processing, shrink to the actual content.
	m.textarea.SetHeight(inputMaxHeight)

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	// Auto-grow height based on visual lines (hard newlines + soft wraps).
	lines := m.visualLineCount()
	h := min(max(lines, inputMinHeight), inputMaxHeight)
	m.textarea.SetHeight(h)

	return m, cmd
}

func (m inputModel) View() string {
	border := focusedBorder
	if !m.enabled {
		border = disabledBorder
	}

	innerWidth := max(m.width-4, 10) // account for border padding
	m.textarea.SetWidth(innerWidth)
	border = border.Width(innerWidth)

	return border.Render(m.textarea.View())
}

func (m *inputModel) setWidth(w int) {
	m.width = w
	innerWidth := max(w-4, 10) // account for border padding
	m.textarea.SetWidth(innerWidth)
}

// visualLineCount returns the number of visual lines the current text occupies,
// accounting for both hard newlines and soft wraps at the textarea width.
func (m inputModel) visualLineCount() int {
	text := m.textarea.Value()
	if text == "" {
		return 1
	}

	wrapWidth := max(m.textarea.Width(), 1)

	total := 0
	for _, line := range strings.Split(text, "\n") {
		w := runewidth.StringWidth(line)
		if w == 0 {
			total++
			continue
		}
		total += (w-1)/wrapWidth + 1
	}

	return total
}

func (m *inputModel) enable() {
	m.enabled = true
	m.textarea.Focus()
}

func (m *inputModel) disable() {
	m.enabled = false
	m.textarea.Blur()
}
