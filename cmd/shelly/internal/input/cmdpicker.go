package input

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

const CmdPickerMaxShow = 4

// AvailableCommands is the static list of supported slash commands.
var AvailableCommands = []string{
	"/help",
	"/clear",
	"/exit",
}

// CmdPickerModel displays an autocomplete popup for /-commands.
type CmdPickerModel struct {
	Active   bool
	query    string   // text typed after '/'
	SlashPos int      // rune position of '/' in input value
	filtered []string // filtered commands
	cursor   int      // highlighted entry index
	maxShow  int
	Width    int
}

// NewCmdPicker creates a new CmdPickerModel.
func NewCmdPicker() CmdPickerModel {
	return CmdPickerModel{maxShow: CmdPickerMaxShow}
}

// Activate opens the picker at the given '/' position.
func (cp *CmdPickerModel) Activate(slashPos int) {
	cp.Active = true
	cp.SlashPos = slashPos
	cp.query = ""
	cp.cursor = 0
	cp.applyFilter()
}

// Dismiss closes the picker.
func (cp *CmdPickerModel) Dismiss() {
	cp.Active = false
	cp.query = ""
	cp.filtered = nil
	cp.cursor = 0
}

// SetQuery updates the filter query and re-filters.
func (cp *CmdPickerModel) SetQuery(q string) {
	cp.query = q
	cp.cursor = 0
	cp.applyFilter()
}

// selected returns the currently highlighted entry, or "" if none.
func (cp *CmdPickerModel) selected() string {
	if len(cp.filtered) == 0 {
		return ""
	}
	return cp.filtered[cp.cursor]
}

// HandleKey processes navigation keys while the picker is active.
func (cp *CmdPickerModel) HandleKey(msg tea.KeyPressMsg) (consumed bool, sel string) {
	k := msg.Key()
	switch k.Code {
	case tea.KeyUp:
		if cp.cursor > 0 {
			cp.cursor--
		}
		return true, ""
	case tea.KeyDown:
		if cp.cursor < len(cp.filtered)-1 {
			cp.cursor++
		}
		return true, ""
	case tea.KeyEnter, tea.KeyTab:
		sel := cp.selected()
		if sel != "" {
			cp.Dismiss()
			return true, sel
		}
		return true, ""
	case tea.KeyEsc:
		cp.Dismiss()
		return true, ""
	}
	return false, ""
}

// View renders the command picker popup.
func (cp CmdPickerModel) View() string {
	if !cp.Active {
		return ""
	}

	innerWidth := max(cp.Width-4, 20)

	var sb strings.Builder

	if len(cp.filtered) == 0 {
		sb.WriteString(styles.PickerDimStyle.Render("  no matching commands"))
	} else {
		show := min(len(cp.filtered), cp.maxShow)
		start := 0
		if cp.cursor >= show {
			start = cp.cursor - show + 1
		}
		end := min(start+show, len(cp.filtered))

		for i := start; i < end; i++ {
			entry := cp.filtered[i]
			if i == cp.cursor {
				sb.WriteString(styles.PickerCurStyle.Render(entry))
			} else {
				sb.WriteString(styles.PickerDimStyle.Render(entry))
			}
			if i < end-1 {
				sb.WriteString("\n")
			}
		}
	}

	border := styles.PickerBorder.Width(innerWidth)
	return border.Render(sb.String())
}

func (cp *CmdPickerModel) applyFilter() {
	q := strings.ToLower(cp.query)
	if q == "" {
		cp.filtered = make([]string, len(AvailableCommands))
		copy(cp.filtered, AvailableCommands)
		return
	}

	var filtered []string
	for _, cmd := range AvailableCommands {
		// Match against the command without the leading '/'.
		name := strings.TrimPrefix(cmd, "/")
		if strings.Contains(strings.ToLower(name), q) {
			filtered = append(filtered, cmd)
		}
	}
	cp.filtered = filtered
}
