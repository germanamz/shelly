package input

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
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
	history    *History
	Enabled    bool
	width      int
}

// New creates a new InputModel with persistent history at the given path.
func New(historyPath string) InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (@ for files, / for commands)"
	ta.ShowLineNumbers = false
	ta.SetHeight(InputMinHeight)
	ta.Prompt = ""
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
		history:    NewHistory(historyPath),
		Enabled:    false,
	}
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	// Handle lifecycle messages regardless of Enabled state.
	switch msg := msg.(type) {
	case msgs.InputEnableMsg:
		m.Enabled = true
		return m, m.textarea.Focus()
	case msgs.InputResetMsg:
		m.textarea.Reset()
		m.textarea.SetHeight(InputMinHeight)
		m.Enabled = true
		m.FilePicker.Active = false
		m.CmdPicker.Active = false
		return m, nil
	case msgs.InputSetWidthMsg:
		m.width = msg.Width
		innerWidth := max(msg.Width-4, 10)
		m.textarea.SetWidth(innerWidth)
		return m, nil
	}

	if !m.Enabled {
		return m, nil
	}

	// Handle picker selection messages from sub-models.
	switch msg := msg.(type) {
	case msgs.FilePickerSelectionMsg:
		m.insertFileSelection(msg.Path)
		return m, nil
	case msgs.CmdPickerSelectionMsg:
		m.textarea.Reset()
		m.CmdPicker.Active = false
		return m, func() tea.Msg { return msgs.InputSubmitMsg{Text: msg.Command} }
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	// Forward non-key messages to textarea and pickers.
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	var pickerCmd tea.Cmd
	m.FilePicker, pickerCmd = m.FilePicker.Update(msg)
	if pickerCmd != nil {
		cmd = tea.Batch(cmd, pickerCmd)
	}

	return m, cmd
}

func isPickerNavKey(code rune) bool {
	return code == tea.KeyUp || code == tea.KeyDown || code == tea.KeyEnter || code == tea.KeyTab || code == tea.KeyEsc
}

// handleKeyPress processes key presses, routing to pickers or textarea as appropriate.
func (m InputModel) handleKeyPress(keyMsg tea.KeyPressMsg) (InputModel, tea.Cmd) {
	// Route keys to file picker when active.
	if m.FilePicker.Active {
		var cmd tea.Cmd
		m.FilePicker, cmd = m.FilePicker.Update(keyMsg)
		if cmd != nil {
			return m, cmd
		}
		if isPickerNavKey(keyMsg.Key().Code) {
			return m, nil
		}
	}

	// Route keys to command picker when active.
	if m.CmdPicker.Active {
		var cmd tea.Cmd
		m.CmdPicker, cmd = m.CmdPicker.Update(keyMsg)
		if cmd != nil {
			return m, cmd
		}
		if isPickerNavKey(keyMsg.Key().Code) {
			return m, nil
		}
	}

	// History navigation: Up on first line, Down on last line.
	if keyMsg.Key().Code == tea.KeyUp && !m.FilePicker.Active && !m.CmdPicker.Active && m.cursorOnFirstLine() {
		if text, ok := m.history.Up(m.textarea.Value()); ok {
			m.textarea.SetValue(text)
			lines := m.visualLineCount()
			m.textarea.SetHeight(min(max(lines, InputMinHeight), InputMaxHeight))
		}
		return m, nil
	}
	if keyMsg.Key().Code == tea.KeyDown && !m.FilePicker.Active && !m.CmdPicker.Active && m.cursorOnLastLine() {
		if text, ok := m.history.Down(); ok {
			m.textarea.SetValue(text)
			lines := m.visualLineCount()
			m.textarea.SetHeight(min(max(lines, InputMinHeight), InputMaxHeight))
		}
		return m, nil
	}

	// Handle enter submission (Shift+Enter and Alt+Enter are consumed by the
	// textarea's InsertNewline binding before reaching here).
	if keyMsg.Key().Code == tea.KeyEnter && keyMsg.Key().Mod&tea.ModAlt == 0 && !m.FilePicker.Active && !m.CmdPicker.Active {
		text := strings.TrimSpace(m.textarea.Value())
		if text != "" {
			m.history.Add(text)
			m.textarea.Reset()
			m.textarea.SetHeight(InputMinHeight)
			m.FilePicker.Active = false
			m.CmdPicker.Active = false
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
	m.textarea, cmd = m.textarea.Update(keyMsg)

	// Auto-grow height based on visual lines (hard newlines + soft wraps).
	lines := m.visualLineCount()
	h := min(max(lines, InputMinHeight), InputMaxHeight)
	m.textarea.SetHeight(h)

	// Detect '@' or '/' insertion or update picker query.
	newVal := m.textarea.Value()
	cmd = m.updatePickerState(prevVal, newVal, cmd)

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
			m.FilePicker, _ = m.FilePicker.Update(msgs.FilePickerQueryMsg{Query: string(runes[queryStart:queryEnd])})
		} else {
			m.FilePicker, _ = m.FilePicker.Update(msgs.FilePickerDismissMsg{})
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
			m.CmdPicker, _ = m.CmdPicker.Update(msgs.CmdPickerQueryMsg{Query: string(runes[queryStart:queryEnd])})
		} else {
			m.CmdPicker, _ = m.CmdPicker.Update(msgs.CmdPickerDismissMsg{})
		}
		return existingCmd
	}

	// Detect new '@' character.
	if strings.Count(newVal, "@") > strings.Count(prevVal, "@") {
		atIdx := strings.LastIndex(newVal, "@")
		atRunePos := len([]rune(newVal[:atIdx]))
		var pickerCmd tea.Cmd
		m.FilePicker, pickerCmd = m.FilePicker.Update(msgs.FilePickerActivateMsg{AtPos: atRunePos})
		if pickerCmd != nil {
			return tea.Batch(existingCmd, pickerCmd)
		}
		return existingCmd
	}

	// Detect '/' at start of input (command picker trigger).
	if newVal == "/" && prevVal == "" {
		m.CmdPicker, _ = m.CmdPicker.Update(msgs.CmdPickerActivateMsg{SlashPos: 0})
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

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m InputModel) viewInput() string {
	border := styles.FocusedBorder
	if !m.Enabled {
		border = styles.DisabledBorder
	}

	innerWidth := max(m.width-4, 10) // account for border (2) + padding (2)
	m.textarea.SetWidth(innerWidth)
	border = border.Width(m.width)

	return border.Render(m.textarea.View())
}

// visualLineCount returns the number of visual lines the current text occupies,
// accounting for both hard newlines and soft wraps at the textarea width.
func (m InputModel) visualLineCount() int {
	text := m.textarea.Value()
	if text == "" {
		return 1
	}

	// Single hard-line (most common case): use the textarea's own wrap count
	// via LineInfo().Height to stay perfectly in sync with its rendering.
	if m.textarea.LineCount() == 1 {
		return m.textarea.LineInfo().Height
	}

	// Multi-line fallback: sum wrap counts per hard line.
	width := max(m.textarea.Width(), 1)

	total := 0
	for line := range strings.SplitSeq(text, "\n") {
		total += wordWrapLineCount(line, width)
	}

	return total
}

// wordWrapLineCount returns the number of visual lines a single hard line
// occupies when word-wrapped at the given width. The algorithm mirrors the
// textarea's internal wrap() function from charm.land/bubbles/v2, using the
// same width-measurement libraries (uniseg for string widths, runewidth for
// single-rune widths).
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
			wordWidth := uniseg.StringWidth(string(wordRunes))
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
			wordWidth := uniseg.StringWidth(string(wordRunes))
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
	wordWidth := uniseg.StringWidth(string(wordRunes))
	if lineWidth+wordWidth+spaces >= width {
		lines++
	}

	return lines
}

// cursorOnFirstLine returns true when the cursor is on the first visual line
// (hard line 0, soft-wrap row 0).
func (m InputModel) cursorOnFirstLine() bool {
	return m.textarea.Line() == 0 && m.textarea.LineInfo().RowOffset == 0
}

// cursorOnLastLine returns true when the cursor is on the last visual line
// (last hard line, last soft-wrap row).
func (m InputModel) cursorOnLastLine() bool {
	li := m.textarea.LineInfo()
	return m.textarea.Line() == m.textarea.LineCount()-1 && li.RowOffset == li.Height-1
}

// ViewHeight returns the height of the input box area.
func (m InputModel) ViewHeight() int {
	// Border (2) + textarea lines.
	lines := m.visualLineCount()
	h := min(max(lines, InputMinHeight), InputMaxHeight)
	return h + 2
}
