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
	askTabActive  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Underline(true)
	askTabDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	askTabInact   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	askReviewHdr  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
)

// askEntry represents a single question within a batch.
type askEntry struct {
	question   ask.Question
	agent      string
	isChoice   bool
	options    []string
	cursor     int
	textarea   textarea.Model
	customMode bool
	answered   bool
	response   string
}

// askBatchModel handles one or more batched ask-user interactions.
type askBatchModel struct {
	entries   []askEntry
	activeTab int
	reviewing bool
	width     int
}

func newAskBatch(questions []askUserMsg, width int) askBatchModel {
	entries := make([]askEntry, len(questions))
	for i, q := range questions {
		ta := newAskTextarea()

		isChoice := len(q.question.Options) > 0
		var options []string
		if isChoice {
			options = make([]string, len(q.question.Options)+1)
			copy(options, q.question.Options)
			options[len(q.question.Options)] = "Other (custom input)"
			ta.Blur()
		}

		entries[i] = askEntry{
			question: q.question,
			agent:    q.agent,
			isChoice: isChoice,
			options:  options,
			textarea: ta,
		}
	}

	// Focus first entry's textarea if it's free-form.
	if len(entries) > 0 && !entries[0].isChoice {
		entries[0].textarea.Focus()
	}

	return askBatchModel{
		entries: entries,
		width:   width,
	}
}

func newAskTextarea() textarea.Model {
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
	return ta
}

func (m askBatchModel) Update(msg tea.Msg) (askBatchModel, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return m.updateTextarea(msg)
	}

	if m.reviewing {
		return m.handleReviewKey(keyMsg)
	}

	return m.handleEntryKey(keyMsg)
}

func (m askBatchModel) handleEntryKey(msg tea.KeyMsg) (askBatchModel, tea.Cmd) {
	e := &m.entries[m.activeTab]

	// Tab navigation (only when multiple questions).
	if len(m.entries) > 1 {
		switch {
		case msg.Type == tea.KeyTab && !e.customMode && e.isChoice:
			return m.nextTab(), nil
		case msg.Type == tea.KeyShiftTab:
			return m.prevTab(), nil
		}
	}

	// Custom text mode.
	if !e.isChoice || e.customMode {
		return m.handleTextKey(msg, e)
	}

	// Choice mode.
	return m.handleChoiceKey(msg, e)
}

func (m askBatchModel) handleTextKey(msg tea.KeyMsg, e *askEntry) (askBatchModel, tea.Cmd) {
	if msg.Type == tea.KeyEnter && !msg.Alt {
		text := strings.TrimSpace(e.textarea.Value())
		if text != "" {
			e.answered = true
			e.response = text
			return m.advanceOrReview()
		}
		return m, nil
	}
	if msg.Type == tea.KeyEsc && e.customMode {
		e.customMode = false
		e.textarea.Reset()
		e.textarea.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	e.textarea, cmd = e.textarea.Update(msg)
	return m, cmd
}

func (m askBatchModel) handleChoiceKey(msg tea.KeyMsg, e *askEntry) (askBatchModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if e.cursor > 0 {
			e.cursor--
		}
	case tea.KeyDown:
		if e.cursor < len(e.options)-1 {
			e.cursor++
		}
	case tea.KeyEnter:
		choice := e.options[e.cursor]
		if choice == "Other (custom input)" {
			e.customMode = true
			e.textarea.Focus()
			return m, nil
		}
		e.answered = true
		e.response = choice
		return m.advanceOrReview()
	}
	return m, nil
}

func (m askBatchModel) handleReviewKey(msg tea.KeyMsg) (askBatchModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		// Confirm all — emit batch answered.
		answers := make([]askAnswer, len(m.entries))
		for i, e := range m.entries {
			answers[i] = askAnswer{questionID: e.question.ID, response: e.response}
		}
		return m, func() tea.Msg { return askBatchAnsweredMsg{answers: answers} }
	case tea.KeyTab:
		// Go back to edit the active tab.
		m.reviewing = false
		return m, nil
	}

	// Number keys to jump to specific question.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		idx := int(msg.Runes[0] - '1')
		if idx >= 0 && idx < len(m.entries) {
			m.reviewing = false
			m.switchToTab(idx)
			return m, nil
		}
	}

	return m, nil
}

func (m askBatchModel) updateTextarea(msg tea.Msg) (askBatchModel, tea.Cmd) {
	if m.reviewing || m.activeTab >= len(m.entries) {
		return m, nil
	}
	e := &m.entries[m.activeTab]
	if !e.isChoice || e.customMode {
		var cmd tea.Cmd
		e.textarea, cmd = e.textarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

// advanceOrReview moves to the next unanswered question, or enters review.
func (m askBatchModel) advanceOrReview() (askBatchModel, tea.Cmd) {
	// Single question — emit immediately.
	if len(m.entries) == 1 {
		e := m.entries[0]
		return m, func() tea.Msg {
			return askBatchAnsweredMsg{answers: []askAnswer{{questionID: e.question.ID, response: e.response}}}
		}
	}

	// Find next unanswered.
	for i := range m.entries {
		idx := (m.activeTab + 1 + i) % len(m.entries)
		if !m.entries[idx].answered {
			m.switchToTab(idx)
			return m, nil
		}
	}

	// All answered — enter review.
	m.reviewing = true
	return m, nil
}

func (m *askBatchModel) switchToTab(idx int) {
	// Blur current.
	if m.activeTab < len(m.entries) {
		m.entries[m.activeTab].textarea.Blur()
	}
	m.activeTab = idx
	e := &m.entries[idx]
	e.answered = false // allow re-editing
	if !e.isChoice || e.customMode {
		e.textarea.Focus()
	}
}

func (m askBatchModel) nextTab() askBatchModel {
	next := (m.activeTab + 1) % len(m.entries)
	m.switchToTab(next)
	return m
}

func (m askBatchModel) prevTab() askBatchModel {
	prev := (m.activeTab - 1 + len(m.entries)) % len(m.entries)
	m.switchToTab(prev)
	return m
}

func (m askBatchModel) View() string {
	innerWidth := max(m.width-4, 10)

	var sb strings.Builder

	// Tab bar (only for multiple questions).
	if len(m.entries) > 1 {
		sb.WriteString(m.renderTabBar())
		sb.WriteString("\n\n")
	}

	if m.reviewing {
		sb.WriteString(m.renderReview())
	} else {
		sb.WriteString(m.renderEntry(innerWidth))
	}

	border := askBorder.Width(innerWidth)
	return border.Render(sb.String())
}

func (m askBatchModel) renderTabBar() string {
	var tabs []string
	for i, e := range m.entries {
		label := fmt.Sprintf("Q%d: %s", i+1, e.agent)
		switch {
		case i == m.activeTab && !m.reviewing:
			tabs = append(tabs, askTabActive.Render("["+label+"]"))
		case e.answered:
			tabs = append(tabs, askTabDone.Render("["+label+" \u2713]"))
		default:
			tabs = append(tabs, askTabInact.Render("["+label+"]"))
		}
	}
	return strings.Join(tabs, " ")
}

func (m askBatchModel) renderEntry(innerWidth int) string {
	e := m.entries[m.activeTab]

	var sb strings.Builder
	sb.WriteString(askTitleStyle.Render(fmt.Sprintf("[%s asks]", e.agent)))
	sb.WriteString(" ")
	sb.WriteString(e.question.Text)
	sb.WriteString("\n\n")

	switch {
	case e.customMode:
		sb.WriteString("  Custom input (Esc to go back):\n")
		e.textarea.SetWidth(innerWidth - 2)
		sb.WriteString("  ")
		sb.WriteString(e.textarea.View())
	case e.isChoice:
		for i, opt := range e.options {
			prefix := "  "
			if i == e.cursor {
				prefix = "> "
				sb.WriteString(askSelStyle.Render(prefix + opt))
			} else {
				sb.WriteString(askOptStyle.Render(prefix + opt))
			}
			sb.WriteString("\n")
		}
	default:
		e.textarea.SetWidth(innerWidth)
		sb.WriteString(e.textarea.View())
	}

	return sb.String()
}

func (m askBatchModel) renderReview() string {
	var sb strings.Builder
	sb.WriteString(askReviewHdr.Render("Review your answers"))
	sb.WriteString("\n\n")

	for i, e := range m.entries {
		sb.WriteString(askTitleStyle.Render(fmt.Sprintf("Q%d [%s]:", i+1, e.agent)))
		sb.WriteString(" ")
		sb.WriteString(truncate(e.question.Text, 80))
		sb.WriteString("\n")
		sb.WriteString(askSelStyle.Render("  A: " + e.response))
		sb.WriteString("\n\n")
	}

	sb.WriteString(dimStyle.Render("Enter = confirm all · Tab = edit · 1-9 = jump to question"))
	return sb.String()
}
