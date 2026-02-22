package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
)

var (
	askBorder     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("3"))
	askTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	askOptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	askSelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
)

// askPromptModel handles a single ask-user interaction.
type askPromptModel struct {
	question   ask.Question
	agent      string
	isChoice   bool
	options    []string
	cursor     int
	textarea   textarea.Model
	customMode bool // true when "Other" is selected in choice mode
	width      int
}

func newAskPrompt(q ask.Question, agent string, width int) askPromptModel {
	ta := textarea.New()
	ta.Placeholder = "Your answer..."
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.CharLimit = 0
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Prompt = lipgloss.NewStyle()
	ta.Focus()

	isChoice := len(q.Options) > 0
	var options []string
	if isChoice {
		options = make([]string, len(q.Options)+1)
		copy(options, q.Options)
		options[len(q.Options)] = "Other (custom input)"
		ta.Blur()
	}

	return askPromptModel{
		question: q,
		agent:    agent,
		isChoice: isChoice,
		options:  options,
		textarea: ta,
		width:    width,
	}
}

func (m askPromptModel) Update(msg tea.Msg) (askPromptModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		if !m.isChoice || m.customMode {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Custom text mode (either free-form or "Other" selected).
	if !m.isChoice || m.customMode {
		if keyMsg.Type == tea.KeyEnter && !keyMsg.Alt {
			text := strings.TrimSpace(m.textarea.Value())
			if text != "" {
				return m, func() tea.Msg {
					return askAnsweredMsg{questionID: m.question.ID, response: text}
				}
			}
			return m, nil
		}
		if keyMsg.Type == tea.KeyEsc && m.customMode {
			m.customMode = false
			m.textarea.Reset()
			m.textarea.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	// Choice mode.
	switch keyMsg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		choice := m.options[m.cursor]
		if choice == "Other (custom input)" {
			m.customMode = true
			m.textarea.Focus()
			return m, nil
		}
		return m, func() tea.Msg {
			return askAnsweredMsg{questionID: m.question.ID, response: choice}
		}
	}

	return m, nil
}

func (m askPromptModel) View() string {
	innerWidth := max(m.width-4, 10)

	var sb strings.Builder
	sb.WriteString(askTitleStyle.Render(fmt.Sprintf("[%s asks]", m.agent)))
	sb.WriteString(" ")
	sb.WriteString(m.question.Text)
	sb.WriteString("\n\n")

	switch {
	case m.customMode:
		sb.WriteString("  Custom input (Esc to go back):\n")
		m.textarea.SetWidth(innerWidth - 2)
		sb.WriteString("  ")
		sb.WriteString(m.textarea.View())
	case m.isChoice:
		for i, opt := range m.options {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
				sb.WriteString(askSelStyle.Render(prefix + opt))
			} else {
				sb.WriteString(askOptStyle.Render(prefix + opt))
			}
			sb.WriteString("\n")
		}
	default:
		m.textarea.SetWidth(innerWidth)
		sb.WriteString(m.textarea.View())
	}

	border := askBorder.Width(innerWidth)
	return border.Render(sb.String())
}
