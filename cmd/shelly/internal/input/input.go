package input

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/basetextarea"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/chats/content"
)

const (
	InputMinHeight = 1
	InputMaxHeight = 5
)

// InputModel wraps a textarea in a rounded border box.
type InputModel struct {
	textarea    basetextarea.Model
	FilePicker  FilePickerModel
	CmdPicker   CmdPickerModel
	history     *History
	attachments []Attachment // pending file attachments
	Enabled     bool
	width       int
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
		m.attachments = nil
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
		m.attachFileSelection(msg.Path)
		return m, nil
	case msgs.CmdPickerSelectionMsg:
		m.textarea.Reset()
		m.CmdPicker.Active = false
		return m, func() tea.Msg { return msgs.InputSubmitMsg{Text: msg.Command} }
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	// Intercept paste events for file paths.
	if pasteMsg, ok := msg.(tea.PasteMsg); ok {
		return m.handlePaste(pasteMsg.Content)
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

	// Ctrl+U clears all pending attachments.
	if keyMsg.Key().Code == 'u' && keyMsg.Key().Mod&tea.ModCtrl != 0 && len(m.attachments) > 0 {
		m.attachments = nil
		return m, nil
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
		parts := m.attachmentParts()
		if text != "" || len(parts) > 0 {
			if text != "" {
				m.history.Add(text)
			}
			m.textarea.Reset()
			m.FilePicker.Active = false
			m.CmdPicker.Active = false
			submitMsg := msgs.InputSubmitMsg{Text: text, Parts: parts}
			m.attachments = nil
			return m, func() tea.Msg { return submitMsg }
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

// attachFileSelection reads the selected file and adds it as an attachment,
// removing the @query text from the input.
func (m *InputModel) attachFileSelection(sel string) {
	// Remove @query from textarea.
	runes := []rune(m.textarea.Value())
	atPos := m.FilePicker.AtPos
	queryEnd := atPos + 1
	for queryEnd < len(runes) && runes[queryEnd] != ' ' && runes[queryEnd] != '\n' {
		queryEnd++
	}
	newRunes := make([]rune, 0, len(runes))
	newRunes = append(newRunes, runes[:atPos]...)
	newRunes = append(newRunes, runes[queryEnd:]...)
	m.textarea.SetValue(string(newRunes))

	// Resolve path to absolute.
	path := sel
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, path)
		}
	}

	att, err := ReadAttachment(path)
	if err != nil {
		// On error, insert the path as text instead (fallback).
		m.textarea.SetValue(string(runes[:atPos]) + sel + string(runes[queryEnd:]))
		return
	}
	// Use the relative path for display.
	att.Path = sel
	m.attachments = append(m.attachments, att)
}

// handlePaste intercepts paste events, detects file paths, and attaches them.
func (m InputModel) handlePaste(text string) (InputModel, tea.Cmd) {
	paths := DetectFilePaths(text)
	if len(paths) == 0 {
		// No file paths — forward as normal paste to textarea.
		var cmd tea.Cmd
		m.textarea.TA, cmd = m.textarea.TA.Update(tea.PasteMsg{Content: text})
		return m, cmd
	}

	// Attach each detected file.
	for _, p := range paths {
		att, err := ReadAttachment(p)
		if err != nil {
			continue
		}
		m.attachments = append(m.attachments, att)
	}

	// Insert remaining text (non-path portions) into textarea.
	remaining := removePathsFromText(text, paths)
	remaining = strings.TrimSpace(remaining)
	if remaining != "" {
		var cmd tea.Cmd
		m.textarea.TA, cmd = m.textarea.TA.Update(tea.PasteMsg{Content: remaining})
		return m, cmd
	}

	return m, nil
}

// removePathsFromText strips detected file paths from pasted text.
func removePathsFromText(text string, paths []string) string {
	result := text
	for _, p := range paths {
		// Try quoted variants first.
		result = strings.ReplaceAll(result, fmt.Sprintf("'%s'", p), "")
		result = strings.ReplaceAll(result, fmt.Sprintf("\"%s\"", p), "")
		result = strings.ReplaceAll(result, p, "")
	}
	return result
}

// attachmentParts converts pending attachments to content.Part slices.
func (m *InputModel) attachmentParts() []content.Part {
	if len(m.attachments) == 0 {
		return nil
	}
	parts := make([]content.Part, len(m.attachments))
	for i, att := range m.attachments {
		parts[i] = att.ToPart()
	}
	return parts
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

	content := m.textarea.View()
	if len(m.attachments) > 0 {
		tags := m.attachmentTagLine()
		content = content + "\n" + tags
	}

	return border.Render(content)
}

// attachmentTagLine renders a line showing all pending attachment names.
func (m InputModel) attachmentTagLine() string {
	var tags []string
	for _, att := range m.attachments {
		tags = append(tags, att.Label())
	}
	return styles.DimStyle.Render(strings.Join(tags, " "))
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

// IsEmpty returns true if the textarea has no text.
func (m InputModel) IsEmpty() bool {
	return strings.TrimSpace(m.textarea.Value()) == ""
}

// ViewHeight returns the height of the input box area.
func (m InputModel) ViewHeight() int {
	// Border (2) + textarea lines.
	lines := m.textarea.VisualLineCount()
	h := min(max(lines, InputMinHeight), InputMaxHeight)
	return h + 2
}
