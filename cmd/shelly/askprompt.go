package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// askEntry represents a single question within a batch.
type askEntry struct {
	question    askUserMsg
	header      string // tab label
	isChoice    bool
	multiSelect bool
	options     []string
	checked     []bool // for multi-select: which options are checked
	cursor      int
	textarea    textarea.Model
	customMode  bool
	answered    bool
	response    string
}

// askBatchModel handles one or more batched ask-user interactions.
type askBatchModel struct {
	entries   []askEntry
	activeTab int
	onConfirm bool // true when on the Confirm tab
	width     int

	// Confirm tab state.
	confirmCursor int  // 0=Yes, 1=No, 2=custom
	confirmCustom bool // true when typing custom text
	confirmTA     textarea.Model
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
			options[len(q.question.Options)] = "(custom input)"
		}

		header := q.question.Header
		if header == "" {
			header = fmt.Sprintf("Q%d", i+1)
		}

		entries[i] = askEntry{
			question:    q,
			header:      header,
			isChoice:    isChoice,
			multiSelect: q.question.MultiSelect,
			options:     options,
			checked:     make([]bool, len(options)),
			textarea:    ta,
		}
	}

	// Focus first entry's textarea if it's free-form.
	if len(entries) > 0 && !entries[0].isChoice {
		entries[0].textarea.Focus()
	}

	confirmTA := newAskTextarea()
	confirmTA.Placeholder = "Type a custom answer..."
	confirmTA.Blur()

	return askBatchModel{
		entries:   entries,
		width:     width,
		confirmTA: confirmTA,
	}
}

func newAskTextarea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Your answer..."
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.CharLimit = 0
	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Blurred.CursorLine = lipgloss.NewStyle()
	s.Focused.Prompt = lipgloss.NewStyle()
	s.Blurred.Prompt = lipgloss.NewStyle()
	ta.SetStyles(s)
	ta.Focus()
	return ta
}

func (m askBatchModel) Update(msg tea.Msg) (askBatchModel, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyPressMsg)
	if !isKey {
		return m.updateTextarea(msg)
	}

	k := keyMsg.Key()

	// Escape dismisses the entire questions UI.
	if k.Code == tea.KeyEsc {
		// If in custom mode, first exit custom mode.
		if m.onConfirm && m.confirmCustom {
			m.confirmCustom = false
			m.confirmTA.Blur()
			return m, nil
		}
		if !m.onConfirm && m.activeTab < len(m.entries) {
			e := &m.entries[m.activeTab]
			if e.customMode {
				e.customMode = false
				e.textarea.Reset()
				e.textarea.Blur()
				return m, nil
			}
		}
		// Dismiss: send empty rejection.
		return m, func() tea.Msg {
			return askBatchAnsweredMsg{answers: nil}
		}
	}

	if m.onConfirm {
		return m.handleConfirmKey(keyMsg)
	}

	return m.handleEntryKey(keyMsg)
}

func (m askBatchModel) handleEntryKey(msg tea.KeyPressMsg) (askBatchModel, tea.Cmd) {
	e := &m.entries[m.activeTab]
	k := msg.Key()

	// Tab navigation: Left/Right switch between question tabs.
	switch k.Code {
	case tea.KeyLeft:
		return m.prevTab(), nil
	case tea.KeyRight:
		return m.nextTab(), nil
	}

	// Custom text mode.
	if !e.isChoice || e.customMode {
		return m.handleTextKey(msg, e)
	}

	// Choice mode.
	return m.handleChoiceKey(msg, e)
}

func (m askBatchModel) handleTextKey(msg tea.KeyPressMsg, e *askEntry) (askBatchModel, tea.Cmd) {
	k := msg.Key()
	if k.Code == tea.KeyEnter && k.Mod&tea.ModAlt == 0 {
		text := strings.TrimSpace(e.textarea.Value())
		if text != "" {
			e.answered = true
			e.response = text
			return m.advanceOrConfirm()
		}
		return m, nil
	}
	var cmd tea.Cmd
	e.textarea, cmd = e.textarea.Update(msg)
	return m, cmd
}

func (m askBatchModel) handleChoiceKey(msg tea.KeyPressMsg, e *askEntry) (askBatchModel, tea.Cmd) {
	k := msg.Key()
	switch k.Code {
	case tea.KeyUp:
		if e.cursor > 0 {
			e.cursor--
		}
	case tea.KeyDown:
		if e.cursor < len(e.options)-1 {
			e.cursor++
		}
	case tea.KeySpace:
		// Toggle for multi-select.
		if e.multiSelect && e.cursor < len(e.options)-1 {
			e.checked[e.cursor] = !e.checked[e.cursor]
		}
	case tea.KeyEnter:
		if e.multiSelect {
			// For multi-select: if cursor is on custom option, enter custom mode.
			if e.cursor == len(e.options)-1 {
				e.customMode = true
				e.textarea.Focus()
				return m, nil
			}
			// Collect checked options as the response.
			var selected []string
			for i, opt := range e.options[:len(e.options)-1] {
				if e.checked[i] {
					selected = append(selected, opt)
				}
			}
			if len(selected) > 0 {
				e.answered = true
				e.response = strings.Join(selected, ", ")
				return m.advanceOrConfirm()
			}
			return m, nil
		}

		// Single-select.
		choice := e.options[e.cursor]
		if choice == "(custom input)" {
			e.customMode = true
			e.textarea.Focus()
			return m, nil
		}
		e.answered = true
		e.response = choice
		return m.advanceOrConfirm()
	}
	return m, nil
}

func (m askBatchModel) handleConfirmKey(msg tea.KeyPressMsg) (askBatchModel, tea.Cmd) {
	k := msg.Key()

	// Tab navigation still works on confirm tab.
	switch k.Code {
	case tea.KeyLeft:
		return m.prevTab(), nil
	case tea.KeyRight:
		return m, nil // already on last tab
	}

	if m.confirmCustom {
		if k.Code == tea.KeyEnter && k.Mod&tea.ModAlt == 0 {
			text := strings.TrimSpace(m.confirmTA.Value())
			if text != "" {
				return m, m.buildAnsweredCmd(text)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.confirmTA, cmd = m.confirmTA.Update(msg)
		return m, cmd
	}

	switch k.Code {
	case tea.KeyUp:
		if m.confirmCursor > 0 {
			m.confirmCursor--
		}
	case tea.KeyDown:
		if m.confirmCursor < 2 {
			m.confirmCursor++
		}
	case tea.KeyEnter:
		switch m.confirmCursor {
		case 0: // Yes
			return m, m.buildAnsweredCmd("")
		case 1: // No — rejection
			return m, func() tea.Msg {
				return askBatchAnsweredMsg{answers: nil}
			}
		case 2: // Custom
			m.confirmCustom = true
			m.confirmTA.Focus()
			return m, nil
		}
	}

	return m, nil
}

func (m askBatchModel) buildAnsweredCmd(customText string) tea.Cmd {
	answers := make([]askAnswer, len(m.entries))
	for i, e := range m.entries {
		resp := e.response
		if customText != "" && i == len(m.entries)-1 {
			resp += "\n" + customText
		}
		answers[i] = askAnswer{questionID: e.question.question.ID, response: resp}
	}
	if customText != "" && len(answers) > 0 {
		answers[len(answers)-1].response = m.entries[len(m.entries)-1].response
		if customText != "" {
			answers[len(answers)-1].response += " (" + customText + ")"
		}
	}
	return func() tea.Msg { return askBatchAnsweredMsg{answers: answers} }
}

func (m askBatchModel) updateTextarea(msg tea.Msg) (askBatchModel, tea.Cmd) {
	if m.onConfirm {
		if m.confirmCustom {
			var cmd tea.Cmd
			m.confirmTA, cmd = m.confirmTA.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if m.activeTab >= len(m.entries) {
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

// advanceOrConfirm moves to the next unanswered question, or enters the Confirm tab.
func (m askBatchModel) advanceOrConfirm() (askBatchModel, tea.Cmd) {
	// Single question — emit immediately.
	if len(m.entries) == 1 {
		e := m.entries[0]
		return m, func() tea.Msg {
			return askBatchAnsweredMsg{answers: []askAnswer{{questionID: e.question.question.ID, response: e.response}}}
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

	// All answered — enter Confirm tab.
	m.onConfirm = true
	m.confirmCursor = 0
	m.confirmCustom = false
	return m, nil
}

func (m *askBatchModel) switchToTab(idx int) {
	if !m.onConfirm && m.activeTab < len(m.entries) {
		m.entries[m.activeTab].textarea.Blur()
	}
	m.onConfirm = false
	m.confirmCustom = false
	m.confirmTA.Blur()
	m.activeTab = idx
	e := &m.entries[idx]
	e.answered = false // allow re-editing
	if !e.isChoice || e.customMode {
		e.textarea.Focus()
	}
}

func (m askBatchModel) nextTab() askBatchModel {
	if m.onConfirm {
		return m
	}
	if m.activeTab < len(m.entries)-1 {
		m.switchToTab(m.activeTab + 1)
	} else {
		allAnswered := true
		for _, e := range m.entries {
			if !e.answered {
				allAnswered = false
				break
			}
		}
		if allAnswered {
			m.onConfirm = true
			m.confirmCursor = 0
		}
	}
	return m
}

func (m askBatchModel) prevTab() askBatchModel {
	if m.onConfirm {
		m.onConfirm = false
		m.confirmCustom = false
		m.confirmTA.Blur()
		if len(m.entries) > 0 {
			m.activeTab = len(m.entries) - 1
		}
		return m
	}
	if m.activeTab > 0 {
		m.switchToTab(m.activeTab - 1)
	}
	return m
}

func (m askBatchModel) View() string {
	innerWidth := max(m.width-4, 10)

	var sb strings.Builder

	// Tab bar.
	sb.WriteString(m.renderTabBar())
	sb.WriteString("\n\n")

	if m.onConfirm {
		sb.WriteString(m.renderConfirm(innerWidth))
	} else {
		sb.WriteString(m.renderEntry(innerWidth))
	}

	// Keyboard hints.
	sb.WriteString("\n\n")
	sb.WriteString(m.renderHints())

	border := askBorder.Width(innerWidth)
	return border.Render(sb.String())
}

func (m askBatchModel) renderTabBar() string {
	var tabs []string
	for i, e := range m.entries {
		label := e.header
		switch {
		case !m.onConfirm && i == m.activeTab:
			tabs = append(tabs, askTabActive.Render("*"+label+"*"))
		case e.answered:
			tabs = append(tabs, askTabDone.Render("["+label+"]"))
		default:
			tabs = append(tabs, askTabInact.Render("["+label+"]"))
		}
	}

	if m.onConfirm {
		tabs = append(tabs, askTabActive.Render("*Confirm*"))
	} else {
		tabs = append(tabs, askTabInact.Render("[Confirm]"))
	}

	return strings.Join(tabs, " ")
}

func (m askBatchModel) renderEntry(innerWidth int) string {
	e := m.entries[m.activeTab]

	var sb strings.Builder
	sb.WriteString(e.question.question.Text)
	sb.WriteString("\n\n")

	switch {
	case e.customMode:
		sb.WriteString("  Custom input (Esc to go back):\n")
		e.textarea.SetWidth(innerWidth - 2)
		sb.WriteString("  ")
		sb.WriteString(e.textarea.View())
	case e.isChoice:
		for i, opt := range e.options {
			num := fmt.Sprintf("%d. ", i+1)
			if e.multiSelect && i < len(e.options)-1 {
				check := "[ ]"
				if e.checked[i] {
					check = "[X]"
				}
				if i == e.cursor {
					sb.WriteString(askSelStyle.Render(fmt.Sprintf("%s%s %s", num, check, opt)))
				} else {
					sb.WriteString(askOptStyle.Render(fmt.Sprintf("%s%s %s", num, check, opt)))
				}
			} else {
				if i == e.cursor {
					sb.WriteString(askSelStyle.Render(num + opt))
				} else {
					sb.WriteString(askOptStyle.Render(num + opt))
				}
			}
			sb.WriteString("\n")
		}
	default:
		e.textarea.SetWidth(innerWidth)
		sb.WriteString(e.textarea.View())
	}

	return sb.String()
}

func (m askBatchModel) renderConfirm(innerWidth int) string {
	var sb strings.Builder
	sb.WriteString(askTitleStyle.Render("Confirm your answers:"))
	sb.WriteString("\n\n")

	for i, e := range m.entries {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, e.question.question.Text)
		fmt.Fprintf(&sb, " %s%s\n", treeCorner, askSelStyle.Render(e.response))
	}

	sb.WriteString("\nAre you happy with your answers?\n")

	confirmOpts := []string{"Yes", "No", "(custom input)"}
	for i, opt := range confirmOpts {
		num := fmt.Sprintf("%d. ", i+1)
		if m.confirmCustom && i == 2 {
			sb.WriteString(askSelStyle.Render(num + opt))
			sb.WriteString("\n")
			m.confirmTA.SetWidth(innerWidth - 4)
			sb.WriteString("  ")
			sb.WriteString(m.confirmTA.View())
			sb.WriteString("\n")
			continue
		}
		if i == m.confirmCursor {
			sb.WriteString(askSelStyle.Render(num + opt))
		} else {
			sb.WriteString(askOptStyle.Render(num + opt))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m askBatchModel) renderHints() string {
	hints := "← Left tab, → Right tab, ↑ Up, ↓ Down"
	if !m.onConfirm && m.activeTab < len(m.entries) {
		e := m.entries[m.activeTab]
		if e.multiSelect && e.isChoice && !e.customMode {
			hints += ", Space Toggle"
		}
	}
	hints += ", ↵ Confirm, Esc Dismiss"
	return askHintStyle.Render(hints)
}
