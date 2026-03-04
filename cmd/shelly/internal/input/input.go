package input

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/basetextarea"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

const (
	InputMinHeight = 1
	InputMaxHeight = 5
)

// InputModel wraps a textarea in a rounded border box.
type InputModel struct {
	textarea   basetextarea.Model
	FilePicker FilePickerModel
	CmdPicker  CmdPickerModel
	history    *History
	Enabled    bool
	width      int
}

// New creates a new InputModel with persistent history at the given path.
func New(historyPath string) InputModel {
	ta := basetextarea.New("Type a message... (@ for files, / for commands)", InputMinHeight, InputMaxHeight)
	ta.TA.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))
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
	m.textarea.TA, cmd = m.textarea.TA.Update(msg)

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
			lines := m.textarea.VisualLineCount()
			m.textarea.SetHeight(min(max(lines, InputMinHeight), InputMaxHeight))
		}
		return m, nil
	}
	if keyMsg.Key().Code == tea.KeyDown && !m.FilePicker.Active && !m.CmdPicker.Active && m.cursorOnLastLine() {
		if text, ok := m.history.Down(); ok {
			m.textarea.SetValue(text)
			lines := m.textarea.VisualLineCount()
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
			m.FilePicker.Active = false
			m.CmdPicker.Active = false
			return m, func() tea.Msg { return msgs.InputSubmitMsg{Text: text} }
		}
		return m, nil
	}

	// Capture text before update to detect '@' or '/' insertion.
	prevVal := m.textarea.Value()

	// Auto-grow update: basetextarea handles pre-set max, update, shrink.
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(keyMsg)

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

// cursorOnFirstLine returns true when the cursor is on the first visual line
// (hard line 0, soft-wrap row 0).
func (m InputModel) cursorOnFirstLine() bool {
	return m.textarea.TA.Line() == 0 && m.textarea.TA.LineInfo().RowOffset == 0
}

// cursorOnLastLine returns true when the cursor is on the last visual line
// (last hard line, last soft-wrap row).
func (m InputModel) cursorOnLastLine() bool {
	li := m.textarea.TA.LineInfo()
	return m.textarea.TA.Line() == m.textarea.TA.LineCount()-1 && li.RowOffset == li.Height-1
}

// ViewHeight returns the height of the input box area.
func (m InputModel) ViewHeight() int {
	// Border (2) + textarea lines.
	lines := m.textarea.VisualLineCount()
	h := min(max(lines, InputMinHeight), InputMaxHeight)
	return h + 2
}
