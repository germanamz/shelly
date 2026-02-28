package input

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/mattn/go-runewidth"
)

const (
	InputMinHeight = 1
	InputMaxHeight = 5
)

// InputModel wraps a textarea in a rounded border box.
type InputModel struct {
	textarea   textarea.Model
	FilePicker FilePickerModel
	CmdPicker  CmdPickerModel
	Enabled    bool
	width      int
	TokenCount string // formatted total session tokens, updated externally
}

// New creates a new InputModel.
func New() InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (@ for files, / for commands)"
	ta.ShowLineNumbers = false
	ta.SetHeight(InputMinHeight)
	ta.CharLimit = 0
	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Blurred.CursorLine = lipgloss.NewStyle()
	s.Focused.Prompt = lipgloss.NewStyle()
	s.Blurred.Prompt = lipgloss.NewStyle()
	ta.SetStyles(s)
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))
	// Don't focus yet — we focus after the drain delay in appModel.Init().

	return InputModel{
		textarea:   ta,
		FilePicker: NewFilePicker(),
		CmdPicker:  NewCmdPicker(),
		Enabled:    false,
	}
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if !m.Enabled {
		return m, nil
	}

	keyMsg, isKey := msg.(tea.KeyPressMsg)

	// Route keys to file picker when active.
	if isKey && m.FilePicker.Active {
		consumed, sel := m.FilePicker.HandleKey(keyMsg)
		if sel != "" {
			m.insertFileSelection(sel)
			return m, nil
		}
		if consumed {
			return m, nil
		}
	}

	// Route keys to command picker when active.
	if isKey && m.CmdPicker.Active {
		consumed, sel := m.CmdPicker.HandleKey(keyMsg)
		if sel != "" {
			m.textarea.SetValue(sel + " ")
			m.CmdPicker.Dismiss()
			return m, nil
		}
		if consumed {
			return m, nil
		}
	}

	// Handle enter submission (Shift+Enter and Alt+Enter are consumed by the
	// textarea's InsertNewline binding before reaching here).
	if isKey && keyMsg.Key().Code == tea.KeyEnter && keyMsg.Key().Mod&tea.ModAlt == 0 && !m.FilePicker.Active && !m.CmdPicker.Active {
		text := strings.TrimSpace(m.textarea.Value())
		if text != "" {
			m.textarea.Reset()
			m.textarea.SetHeight(InputMinHeight)
			m.FilePicker.Dismiss()
			m.CmdPicker.Dismiss()
			return m, func() tea.Msg { return msgs.InputSubmitMsg{Text: text} }
		}
		return m, nil
	}

	// Capture text before update to detect '@' or '/' insertion.
	prevVal := m.textarea.Value()

	// Pre-set max height so the textarea has room and won't scroll its
	// viewport during Update. After processing, shrink to the actual content.
	m.textarea.SetHeight(InputMaxHeight)

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	// Auto-grow height based on visual lines (hard newlines + soft wraps).
	lines := m.visualLineCount()
	h := min(max(lines, InputMinHeight), InputMaxHeight)
	m.textarea.SetHeight(h)

	// Detect '@' or '/' insertion or update picker query.
	newVal := m.textarea.Value()
	if isKey {
		cmd = m.updatePickerState(prevVal, newVal, cmd)
	}

	return m, cmd
}

// updatePickerState detects '@' or '/' insertion and updates the picker query.
func (m *InputModel) updatePickerState(prevVal, newVal string, existingCmd tea.Cmd) tea.Cmd {
	// File picker active — update query.
	if m.FilePicker.Active {
		runes := []rune(newVal)
		if m.FilePicker.AtPos < len(runes) {
			queryStart := m.FilePicker.AtPos + 1 // skip '@'
			queryEnd := queryStart
			for queryEnd < len(runes) && runes[queryEnd] != ' ' && runes[queryEnd] != '\n' {
				queryEnd++
			}
			m.FilePicker.SetQuery(string(runes[queryStart:queryEnd]))
		} else {
			m.FilePicker.Dismiss()
		}
		return existingCmd
	}

	// Command picker active — update query.
	if m.CmdPicker.Active {
		runes := []rune(newVal)
		if m.CmdPicker.SlashPos < len(runes) {
			queryStart := m.CmdPicker.SlashPos + 1 // skip '/'
			queryEnd := queryStart
			for queryEnd < len(runes) && runes[queryEnd] != ' ' && runes[queryEnd] != '\n' {
				queryEnd++
			}
			m.CmdPicker.SetQuery(string(runes[queryStart:queryEnd]))
		} else {
			m.CmdPicker.Dismiss()
		}
		return existingCmd
	}

	// Detect new '@' character.
	if strings.Count(newVal, "@") > strings.Count(prevVal, "@") {
		atIdx := strings.LastIndex(newVal, "@")
		atRunePos := len([]rune(newVal[:atIdx]))
		pickerCmd := m.FilePicker.Activate(atRunePos)
		if pickerCmd != nil {
			return tea.Batch(existingCmd, pickerCmd)
		}
		return existingCmd
	}

	// Detect '/' at start of input (command picker trigger).
	if newVal == "/" && prevVal == "" {
		m.CmdPicker.Activate(0)
		return existingCmd
	}

	return existingCmd
}

// insertFileSelection replaces @query with the selected file path.
func (m *InputModel) insertFileSelection(sel string) {
	runes := []rune(m.textarea.Value())
	atPos := m.FilePicker.AtPos

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

// PickerActive returns true if any picker is open.
func (m InputModel) PickerActive() bool {
	return m.FilePicker.Active || m.CmdPicker.Active
}

func (m InputModel) View() string {
	inputView := m.viewInput()
	var parts []string

	if m.FilePicker.Active {
		m.FilePicker.Width = m.width
		parts = append(parts, m.FilePicker.View())
	}
	if m.CmdPicker.Active {
		m.CmdPicker.Width = m.width
		parts = append(parts, m.CmdPicker.View())
	}

	parts = append(parts, inputView)

	// Token counter below input (hidden when any picker is open).
	if !m.PickerActive() && m.TokenCount != "" {
		parts = append(parts, styles.StatusStyle.Render(fmt.Sprintf(" %s tokens", m.TokenCount)))
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m InputModel) viewInput() string {
	border := styles.FocusedBorder
	if !m.Enabled {
		border = styles.DisabledBorder
	}

	innerWidth := max(m.width-4, 10) // account for border padding
	m.textarea.SetWidth(innerWidth)
	border = border.Width(innerWidth)

	return border.Render(m.textarea.View())
}

func (m *InputModel) SetWidth(w int) {
	m.width = w
	innerWidth := max(w-4, 10) // account for border padding
	m.textarea.SetWidth(innerWidth)
}

// visualLineCount returns the number of visual lines the current text occupies,
// accounting for both hard newlines and soft wraps at the textarea width.
// This mirrors the textarea's internal word-wrap function so the height
// calculation stays in sync with what the textarea actually renders.
func (m InputModel) visualLineCount() int {
	text := m.textarea.Value()
	if text == "" {
		return 1
	}

	width := max(m.textarea.Width(), 1)

	total := 0
	for line := range strings.SplitSeq(text, "\n") {
		total += wordWrapLineCount(line, width)
	}

	return total
}

// wordWrapLineCount returns the number of visual lines a single hard line
// occupies when word-wrapped at the given width. The algorithm mirrors the
// textarea's internal wrap() function from charm.land/bubbles/v2.
func wordWrapLineCount(text string, width int) int {
	runes := []rune(text)
	if len(runes) == 0 {
		return 1
	}

	lines := 1
	lineWidth := 0 // visual width of the current line so far
	var wordRunes []rune
	spaces := 0

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			wordRunes = append(wordRunes, r)
		}

		if spaces > 0 {
			wordWidth := runewidth.StringWidth(string(wordRunes))
			if lineWidth+wordWidth+spaces > width {
				// Word doesn't fit on current line — wrap.
				lines++
				lineWidth = wordWidth + spaces
			} else {
				lineWidth += wordWidth + spaces
			}
			spaces = 0
			wordRunes = wordRunes[:0]
		} else {
			// Check if a single word exceeds the line width and must be broken.
			lastCharLen := runewidth.RuneWidth(wordRunes[len(wordRunes)-1])
			wordWidth := runewidth.StringWidth(string(wordRunes))
			if wordWidth+lastCharLen > width {
				if lineWidth > 0 {
					lines++
				}
				lineWidth = wordWidth
				wordRunes = wordRunes[:0]
			}
		}
	}

	// Handle remaining text after the loop — mirrors the textarea's >= boundary.
	wordWidth := runewidth.StringWidth(string(wordRunes))
	if lineWidth+wordWidth+spaces >= width {
		lines++
	}

	return lines
}

func (m *InputModel) Enable() tea.Cmd {
	return m.textarea.Focus()
}

func (m *InputModel) Reset() {
	m.textarea.Reset()
	m.textarea.SetHeight(InputMinHeight)
	m.Enabled = true
	m.FilePicker.Dismiss()
	m.CmdPicker.Dismiss()
}

// ViewHeight returns the height of the input box area.
func (m InputModel) ViewHeight() int {
	// Border (2) + textarea lines + token counter (1).
	lines := m.visualLineCount()
	h := min(max(lines, InputMinHeight), InputMaxHeight)
	return h + 2 + 1
}
