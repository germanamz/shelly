package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

const (
	inputMinHeight = 1
	inputMaxHeight = 5
)

// inputModel wraps a textarea in a rounded border box.
type inputModel struct {
	textarea   textarea.Model
	filePicker filePickerModel
	cmdPicker  cmdPickerModel
	enabled    bool
	width      int
	tokenCount string // formatted total session tokens, updated externally
}

func newInput() inputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (@ for files, / for commands)"
	ta.ShowLineNumbers = false
	ta.SetHeight(inputMinHeight)
	ta.CharLimit = 0
	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Blurred.CursorLine = lipgloss.NewStyle()
	s.Focused.Prompt = lipgloss.NewStyle()
	s.Blurred.Prompt = lipgloss.NewStyle()
	ta.SetStyles(s)
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))
	// Don't focus yet — we focus after the drain delay in appModel.Init().

	return inputModel{
		textarea:   ta,
		filePicker: newFilePicker(),
		cmdPicker:  newCmdPicker(),
		enabled:    false,
	}
}

func (m inputModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	if !m.enabled {
		return m, nil
	}

	keyMsg, isKey := msg.(tea.KeyPressMsg)

	// Route keys to file picker when active.
	if isKey && m.filePicker.active {
		consumed, sel := m.filePicker.handleKey(keyMsg)
		if sel != "" {
			m.insertFileSelection(sel)
			return m, nil
		}
		if consumed {
			return m, nil
		}
	}

	// Route keys to command picker when active.
	if isKey && m.cmdPicker.active {
		consumed, sel := m.cmdPicker.handleKey(keyMsg)
		if sel != "" {
			m.textarea.SetValue(sel + " ")
			m.cmdPicker.dismiss()
			return m, nil
		}
		if consumed {
			return m, nil
		}
	}

	// Handle enter submission (Shift+Enter and Alt+Enter are consumed by the
	// textarea's InsertNewline binding before reaching here).
	if isKey && keyMsg.Key().Code == tea.KeyEnter && keyMsg.Key().Mod&tea.ModAlt == 0 && !m.filePicker.active && !m.cmdPicker.active {
		text := strings.TrimSpace(m.textarea.Value())
		if text != "" {
			m.textarea.Reset()
			m.textarea.SetHeight(inputMinHeight)
			m.filePicker.dismiss()
			m.cmdPicker.dismiss()
			return m, func() tea.Msg { return inputSubmitMsg{text: text} }
		}
		return m, nil
	}

	// Capture text before update to detect '@' or '/' insertion.
	prevVal := m.textarea.Value()

	// Pre-set max height so the textarea has room and won't scroll its
	// viewport during Update. After processing, shrink to the actual content.
	m.textarea.SetHeight(inputMaxHeight)

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	// Auto-grow height based on visual lines (hard newlines + soft wraps).
	lines := m.visualLineCount()
	h := min(max(lines, inputMinHeight), inputMaxHeight)
	m.textarea.SetHeight(h)

	// Detect '@' or '/' insertion or update picker query.
	newVal := m.textarea.Value()
	if isKey {
		cmd = m.updatePickerState(prevVal, newVal, cmd)
	}

	return m, cmd
}

// updatePickerState detects '@' or '/' insertion and updates the picker query.
func (m *inputModel) updatePickerState(prevVal, newVal string, existingCmd tea.Cmd) tea.Cmd {
	// File picker active — update query.
	if m.filePicker.active {
		runes := []rune(newVal)
		if m.filePicker.atPos < len(runes) {
			queryStart := m.filePicker.atPos + 1 // skip '@'
			queryEnd := queryStart
			for queryEnd < len(runes) && runes[queryEnd] != ' ' && runes[queryEnd] != '\n' {
				queryEnd++
			}
			m.filePicker.setQuery(string(runes[queryStart:queryEnd]))
		} else {
			m.filePicker.dismiss()
		}
		return existingCmd
	}

	// Command picker active — update query.
	if m.cmdPicker.active {
		runes := []rune(newVal)
		if m.cmdPicker.slashPos < len(runes) {
			queryStart := m.cmdPicker.slashPos + 1 // skip '/'
			queryEnd := queryStart
			for queryEnd < len(runes) && runes[queryEnd] != ' ' && runes[queryEnd] != '\n' {
				queryEnd++
			}
			m.cmdPicker.setQuery(string(runes[queryStart:queryEnd]))
		} else {
			m.cmdPicker.dismiss()
		}
		return existingCmd
	}

	// Detect new '@' character.
	if strings.Count(newVal, "@") > strings.Count(prevVal, "@") {
		atIdx := strings.LastIndex(newVal, "@")
		atRunePos := len([]rune(newVal[:atIdx]))
		pickerCmd := m.filePicker.activate(atRunePos)
		if pickerCmd != nil {
			return tea.Batch(existingCmd, pickerCmd)
		}
		return existingCmd
	}

	// Detect '/' at start of input (command picker trigger).
	if newVal == "/" && prevVal == "" {
		m.cmdPicker.activate(0)
		return existingCmd
	}

	return existingCmd
}

// insertFileSelection replaces @query with the selected file path.
func (m *inputModel) insertFileSelection(sel string) {
	runes := []rune(m.textarea.Value())
	atPos := m.filePicker.atPos

	// Find end of @query.
	queryEnd := atPos + 1
	for queryEnd < len(runes) && runes[queryEnd] != ' ' && runes[queryEnd] != '\n' {
		queryEnd++
	}

	// Replace @query with selected path.
	newRunes := make([]rune, 0, len(runes)+len(sel))
	newRunes = append(newRunes, runes[:atPos]...)
	newRunes = append(newRunes, []rune(sel)...)
	newRunes = append(newRunes, runes[queryEnd:]...)

	m.textarea.SetValue(string(newRunes))
}

// pickerActive returns true if any picker is open.
func (m inputModel) pickerActive() bool {
	return m.filePicker.active || m.cmdPicker.active
}

func (m inputModel) View() string {
	input := m.viewInput()
	var parts []string

	if m.filePicker.active {
		m.filePicker.width = m.width
		parts = append(parts, m.filePicker.View())
	}
	if m.cmdPicker.active {
		m.cmdPicker.width = m.width
		parts = append(parts, m.cmdPicker.View())
	}

	parts = append(parts, input)

	// Token counter below input (hidden when any picker is open).
	if !m.pickerActive() && m.tokenCount != "" {
		parts = append(parts, statusStyle.Render(fmt.Sprintf(" %s tokens", m.tokenCount)))
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m inputModel) viewInput() string {
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
	for line := range strings.SplitSeq(text, "\n") {
		w := runewidth.StringWidth(line)
		if w == 0 {
			total++
			continue
		}
		total += (w-1)/wrapWidth + 1
	}

	return total
}

func (m *inputModel) enable() tea.Cmd {
	return m.textarea.Focus()
}

func (m *inputModel) Reset() {
	m.textarea.Reset()
	m.textarea.SetHeight(inputMinHeight)
	m.enabled = true
	m.filePicker.dismiss()
	m.cmdPicker.dismiss()
}
