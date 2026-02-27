package configwizard

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// FormField is the interface every wizard field implements.
// Fields are not tea.Model — they are managed by FormModel.
type FormField interface {
	Label() string
	Value() string
	SetValue(string)
	Focus() tea.Cmd
	Blur()
	Validate() error
	Update(tea.Msg) (FormField, tea.Cmd)
	View() string
}

// -----------------------------------------------------------------------
// TextField
// -----------------------------------------------------------------------

// TextField wraps a textinput for single-line string input.
type TextField struct {
	label    string
	input    textinput.Model
	required bool
}

// defaultInputWidth is the text content width for single-line inputs.
// bubbles v2 textinput defaults to Width=0 which causes placeholderView
// to render only the first character. Setting an explicit width avoids this.
const defaultInputWidth = 50

// NewTextField creates a new single-line text field.
func NewTextField(label, placeholder string, required bool) *TextField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	ti.SetWidth(defaultInputWidth)
	return &TextField{label: label, input: ti, required: required}
}

func (f *TextField) Label() string     { return f.label }
func (f *TextField) Value() string     { return f.input.Value() }
func (f *TextField) SetValue(v string) { f.input.SetValue(v) }
func (f *TextField) Focus() tea.Cmd    { return f.input.Focus() }
func (f *TextField) Blur()             { f.input.Blur() }

func (f *TextField) Validate() error {
	if f.required && strings.TrimSpace(f.input.Value()) == "" {
		return fmt.Errorf("%s is required", f.label)
	}
	return nil
}

func (f *TextField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *TextField) View() string {
	return f.input.View()
}

// -----------------------------------------------------------------------
// IntField
// -----------------------------------------------------------------------

// IntField wraps a textinput that accepts only integer values.
type IntField struct {
	label    string
	input    textinput.Model
	required bool
}

// NewIntField creates an integer input field.
func NewIntField(label, placeholder string, required bool) *IntField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 20
	ti.SetWidth(defaultInputWidth)
	return &IntField{label: label, input: ti, required: required}
}

func (f *IntField) Label() string     { return f.label }
func (f *IntField) Value() string     { return f.input.Value() }
func (f *IntField) SetValue(v string) { f.input.SetValue(v) }
func (f *IntField) Focus() tea.Cmd    { return f.input.Focus() }
func (f *IntField) Blur()             { f.input.Blur() }

func (f *IntField) Validate() error {
	v := strings.TrimSpace(f.input.Value())
	if v == "" {
		if f.required {
			return fmt.Errorf("%s is required", f.label)
		}
		return nil
	}
	if _, err := strconv.Atoi(v); err != nil {
		return fmt.Errorf("%s must be an integer", f.label)
	}
	return nil
}

// IntValue returns the parsed integer value and whether it was set.
func (f *IntField) IntValue() (int, bool) {
	v := strings.TrimSpace(f.input.Value())
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (f *IntField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *IntField) View() string {
	return f.input.View()
}

// -----------------------------------------------------------------------
// FloatField
// -----------------------------------------------------------------------

// FloatField wraps a textinput for float64 values.
type FloatField struct {
	label    string
	input    textinput.Model
	required bool
}

// NewFloatField creates a float input field.
func NewFloatField(label, placeholder string, required bool) *FloatField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 20
	ti.SetWidth(defaultInputWidth)
	return &FloatField{label: label, input: ti, required: required}
}

func (f *FloatField) Label() string     { return f.label }
func (f *FloatField) Value() string     { return f.input.Value() }
func (f *FloatField) SetValue(v string) { f.input.SetValue(v) }
func (f *FloatField) Focus() tea.Cmd    { return f.input.Focus() }
func (f *FloatField) Blur()             { f.input.Blur() }

func (f *FloatField) Validate() error {
	v := strings.TrimSpace(f.input.Value())
	if v == "" {
		if f.required {
			return fmt.Errorf("%s is required", f.label)
		}
		return nil
	}
	if _, err := strconv.ParseFloat(v, 64); err != nil {
		return fmt.Errorf("%s must be a number", f.label)
	}
	return nil
}

// FloatValue returns the parsed float value and whether it was set.
func (f *FloatField) FloatValue() (float64, bool) {
	v := strings.TrimSpace(f.input.Value())
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (f *FloatField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *FloatField) View() string {
	return f.input.View()
}

// -----------------------------------------------------------------------
// BoolField
// -----------------------------------------------------------------------

// BoolField is a simple toggle field.
type BoolField struct {
	label   string
	val     bool
	focused bool
}

// NewBoolField creates a boolean toggle field.
func NewBoolField(label string) *BoolField {
	return &BoolField{label: label}
}

func (f *BoolField) Label() string { return f.label }

func (f *BoolField) Value() string {
	if f.val {
		return "true"
	}
	return "false"
}

func (f *BoolField) SetValue(v string) { f.val = v == "true" }
func (f *BoolField) BoolValue() bool   { return f.val }

func (f *BoolField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *BoolField) Blur()           { f.focused = false }
func (f *BoolField) Validate() error { return nil }

func (f *BoolField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter", "space":
			f.val = !f.val
		}
	}
	return f, nil
}

func (f *BoolField) View() string {
	indicator := "[ ]"
	if f.val {
		indicator = "[x]"
	}
	if f.focused {
		return styles.AskSelStyle.Render(indicator)
	}
	return styles.AskOptStyle.Render(indicator)
}

// -----------------------------------------------------------------------
// SelectField
// -----------------------------------------------------------------------

// SelectField lets the user pick one option from a list.
type SelectField struct {
	label   string
	options []string
	cursor  int
	focused bool
}

// NewSelectField creates a single-select field.
func NewSelectField(label string, options []string) *SelectField {
	return &SelectField{label: label, options: options}
}

func (f *SelectField) Label() string { return f.label }

func (f *SelectField) Value() string {
	if f.cursor >= 0 && f.cursor < len(f.options) {
		return f.options[f.cursor]
	}
	return ""
}

func (f *SelectField) SetValue(v string) {
	for i, opt := range f.options {
		if opt == v {
			f.cursor = i
			return
		}
	}
}

func (f *SelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *SelectField) Blur()           { f.focused = false }
func (f *SelectField) Validate() error { return nil }

func (f *SelectField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "left", "h":
			if f.cursor > 0 {
				f.cursor--
			}
		case "right", "l":
			if f.cursor < len(f.options)-1 {
				f.cursor++
			}
		}
	}
	return f, nil
}

func (f *SelectField) View() string {
	var b strings.Builder
	for i, opt := range f.options {
		if i > 0 {
			b.WriteString("  ")
		}
		if i == f.cursor {
			if f.focused {
				b.WriteString(styles.AskSelStyle.Render(opt))
			} else {
				b.WriteString(lipgloss.NewStyle().Bold(true).Render(opt))
			}
		} else {
			b.WriteString(styles.AskOptStyle.Render(opt))
		}
	}
	return b.String()
}

// -----------------------------------------------------------------------
// MultiSelectField
// -----------------------------------------------------------------------

// MultiSelectField lets the user toggle multiple options.
type MultiSelectField struct {
	label    string
	options  []string
	selected map[string]bool
	cursor   int
	focused  bool
}

// NewMultiSelectField creates a multi-select field.
func NewMultiSelectField(label string, options []string) *MultiSelectField {
	return &MultiSelectField{
		label:    label,
		options:  options,
		selected: make(map[string]bool),
	}
}

func (f *MultiSelectField) Label() string { return f.label }

func (f *MultiSelectField) Value() string {
	var sel []string
	for _, opt := range f.options {
		if f.selected[opt] {
			sel = append(sel, opt)
		}
	}
	return strings.Join(sel, ",")
}

func (f *MultiSelectField) SetValue(v string) {
	f.selected = make(map[string]bool)
	if v == "" {
		return
	}
	for s := range strings.SplitSeq(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			f.selected[s] = true
		}
	}
}

// SelectedItems returns the selected options as a slice.
func (f *MultiSelectField) SelectedItems() []string {
	var sel []string
	for _, opt := range f.options {
		if f.selected[opt] {
			sel = append(sel, opt)
		}
	}
	return sel
}

func (f *MultiSelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *MultiSelectField) Blur()           { f.focused = false }
func (f *MultiSelectField) Validate() error { return nil }

func (f *MultiSelectField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if f.cursor > 0 {
				f.cursor--
			}
		case "down", "j":
			if f.cursor < len(f.options)-1 {
				f.cursor++
			}
		case "space":
			opt := f.options[f.cursor]
			f.selected[opt] = !f.selected[opt]
		}
	}
	return f, nil
}

func (f *MultiSelectField) View() string {
	var b strings.Builder
	for i, opt := range f.options {
		check := "[ ]"
		if f.selected[opt] {
			check = "[x]"
		}
		line := fmt.Sprintf("%s %s", check, opt)
		switch {
		case i == f.cursor && f.focused:
			b.WriteString(styles.AskSelStyle.Render(line))
		case f.selected[opt]:
			b.WriteString(lipgloss.NewStyle().Bold(true).Render(line))
		default:
			b.WriteString(styles.AskOptStyle.Render(line))
		}
		if i < len(f.options)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// -----------------------------------------------------------------------
// TextAreaField
// -----------------------------------------------------------------------

// TextAreaField wraps a textarea for multi-line text input.
type TextAreaField struct {
	label    string
	input    textarea.Model
	required bool
}

// NewTextAreaField creates a multi-line text area field.
func NewTextAreaField(label, placeholder string, required bool) *TextAreaField {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.SetHeight(4)
	ta.CharLimit = 4096
	return &TextAreaField{label: label, input: ta, required: required}
}

func (f *TextAreaField) Label() string     { return f.label }
func (f *TextAreaField) Value() string     { return f.input.Value() }
func (f *TextAreaField) SetValue(v string) { f.input.SetValue(v) }
func (f *TextAreaField) Focus() tea.Cmd    { return f.input.Focus() }
func (f *TextAreaField) Blur()             { f.input.Blur() }

func (f *TextAreaField) Validate() error {
	if f.required && strings.TrimSpace(f.input.Value()) == "" {
		return fmt.Errorf("%s is required", f.label)
	}
	return nil
}

func (f *TextAreaField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *TextAreaField) View() string {
	return f.input.View()
}

// -----------------------------------------------------------------------
// FormModel — container with Tab/Shift-Tab navigation
// -----------------------------------------------------------------------

// formSubmitMsg signals the form was submitted.
type formSubmitMsg struct{}

// formCancelMsg signals the form was cancelled.
type formCancelMsg struct{}

// FormModel manages a slice of FormFields with keyboard navigation.
type FormModel struct {
	Title  string
	Fields []FormField
	focus  int
	err    string
}

// NewFormModel creates a new form container.
func NewFormModel(title string, fields []FormField) *FormModel {
	return &FormModel{Title: title, Fields: fields}
}

func (m *FormModel) Init() tea.Cmd {
	if len(m.Fields) > 0 {
		return m.Fields[0].Focus()
	}
	return nil
}

func (m *FormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		switch kmsg.String() {
		case "tab":
			return m, m.nextField()
		case "shift+tab":
			return m, m.prevField()
		case "ctrl+s":
			return m, m.submit()
		case "esc":
			return m, func() tea.Msg { return formCancelMsg{} }
		case "enter":
			switch m.Fields[m.focus].(type) {
			case *TextAreaField:
				// Let Enter pass through for newlines.
			case *BoolField:
				// Delegate to BoolField to toggle value.
			default:
				return m, m.nextField()
			}
		case "up", "k":
			switch m.Fields[m.focus].(type) {
			case *MultiSelectField, *TextAreaField:
				// Let arrows pass through for internal navigation.
			default:
				return m, m.prevField()
			}
		case "down", "j":
			switch m.Fields[m.focus].(type) {
			case *MultiSelectField, *TextAreaField:
				// Let arrows pass through for internal navigation.
			default:
				return m, m.nextField()
			}
		}
	}

	// Delegate to focused field.
	if m.focus >= 0 && m.focus < len(m.Fields) {
		updated, cmd := m.Fields[m.focus].Update(msg)
		m.Fields[m.focus] = updated
		return m, cmd
	}
	return m, nil
}

func (m *FormModel) nextField() tea.Cmd {
	if m.focus >= len(m.Fields)-1 {
		return m.submit()
	}
	m.Fields[m.focus].Blur()
	m.focus++
	return m.Fields[m.focus].Focus()
}

func (m *FormModel) prevField() tea.Cmd {
	if m.focus <= 0 {
		return nil
	}
	m.Fields[m.focus].Blur()
	m.focus--
	return m.Fields[m.focus].Focus()
}

func (m *FormModel) submit() tea.Cmd {
	for _, f := range m.Fields {
		if err := f.Validate(); err != nil {
			m.err = err.Error()
			return nil
		}
	}
	m.err = ""
	return func() tea.Msg { return formSubmitMsg{} }
}

func (m *FormModel) View() tea.View {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorAccent)
	b.WriteString(titleStyle.Render(m.Title))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Bold(true).Width(22)
	focusedLabel := lipgloss.NewStyle().Bold(true).Width(22).Foreground(styles.ColorAccent)

	for i, f := range m.Fields {
		ls := labelStyle
		if i == m.focus {
			ls = focusedLabel
		}
		b.WriteString(ls.Render(f.Label()))
		b.WriteString("  ")
		b.WriteString(f.View())
		b.WriteString("\n")

		// Add extra newline after multi-line fields for readability.
		switch f.(type) {
		case *TextAreaField, *MultiSelectField:
			b.WriteString("\n")
		}
	}

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(styles.ToolErrorStyle.Render("Error: " + m.err))
	}

	b.WriteString("\n")
	b.WriteString(styles.DimStyle.Render("↑/↓: navigate  Tab: next  Shift+Tab: prev  Ctrl+S: save  Esc: cancel"))

	return tea.NewView(b.String())
}
