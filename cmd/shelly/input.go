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
	textarea   textarea.Model
	filePicker filePickerModel
	enabled    bool
	width      int
}

func newInput() inputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (@ for files)"
	ta.ShowLineNumbers = false
	ta.SetHeight(inputMinHeight)
	ta.CharLimit = 0
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Prompt = lipgloss.NewStyle()
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter"))
	// Don't focus yet — the terminal may still be sending OSC responses
	// (e.g. background-color query) that bubbletea misinterprets as key
	// events.  We focus after a short drain delay in appModel.Init().

	return inputModel{
		textarea:   ta,
		filePicker: newFilePicker(),
		enabled:    false,
	}
}

func (m inputModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	if !m.enabled {
		return m, nil
	}

	// Handle file picker entries arriving.
	if entries, ok := msg.(filePickerEntriesMsg); ok {
		m.filePicker.setEntries(entries.entries)
		return m, nil
	}

	keyMsg, isKey := msg.(tea.KeyMsg)

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

	// Handle enter submission.
	if isKey && keyMsg.Type == tea.KeyEnter && !keyMsg.Alt && !m.filePicker.active {
		text := strings.TrimSpace(m.textarea.Value())
		if text != "" {
			m.textarea.Reset()
			m.textarea.SetHeight(inputMinHeight)
			m.filePicker.dismiss()
			return m, func() tea.Msg { return inputSubmitMsg{text: text} }
		}
		return m, nil
	}

	// Capture text before update to detect '@' insertion.
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

	// Detect '@' insertion or update picker query.
	newVal := m.textarea.Value()
	if isKey {
		cmd = m.updatePickerState(prevVal, newVal, cmd)
	}

	return m, cmd
}

// updatePickerState detects '@' insertion and updates the picker query.
func (m *inputModel) updatePickerState(prevVal, newVal string, existingCmd tea.Cmd) tea.Cmd {
	if m.filePicker.active {
		// Extract query from text after atPos.
		runes := []rune(newVal)
		if m.filePicker.atPos < len(runes) {
			// Find the end of the query (next space or end of string).
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

	// Detect new '@' character.
	if len(newVal) > len(prevVal) && strings.Contains(newVal, "@") && !strings.Contains(prevVal, "@") {
		atIdx := strings.LastIndex(newVal, "@")
		atRunePos := len([]rune(newVal[:atIdx]))
		pickerCmd := m.filePicker.activate(atRunePos)
		if pickerCmd != nil {
			return tea.Batch(existingCmd, pickerCmd)
		}
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

// totalHeight returns the full visual height of the input area including picker.
func (m inputModel) totalHeight() int {
	base := lipgloss.Height(m.viewInput())
	return base + m.filePicker.visibleHeight()
}

func (m inputModel) View() string {
	input := m.viewInput()
	if m.filePicker.active {
		m.filePicker.width = m.width
		picker := m.filePicker.View()
		return lipgloss.JoinVertical(lipgloss.Left, picker, input)
	}
	return input
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
	m.enabled = true
	return m.textarea.Focus()
}

func (m *inputModel) disable() {
	m.enabled = false
	m.textarea.Blur()
	m.filePicker.dismiss()
}
