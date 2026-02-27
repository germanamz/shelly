package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const cmdPickerMaxShow = 4

// availableCommands is the static list of supported slash commands.
var availableCommands = []string{
	"/help",
	"/clear",
	"/exit",
}

// cmdPickerModel displays an autocomplete popup for /-commands.
type cmdPickerModel struct {
	active   bool
	query    string   // text typed after '/'
	slashPos int      // rune position of '/' in input value
	filtered []string // filtered commands
	cursor   int      // highlighted entry index
	maxShow  int
	width    int
}

func newCmdPicker() cmdPickerModel {
	return cmdPickerModel{maxShow: cmdPickerMaxShow}
}

// activate opens the picker at the given '/' position.
func (cp *cmdPickerModel) activate(slashPos int) {
	cp.active = true
	cp.slashPos = slashPos
	cp.query = ""
	cp.cursor = 0
	cp.applyFilter()
}

// dismiss closes the picker.
func (cp *cmdPickerModel) dismiss() {
	cp.active = false
	cp.query = ""
	cp.filtered = nil
	cp.cursor = 0
}

// setQuery updates the filter query and re-filters.
func (cp *cmdPickerModel) setQuery(q string) {
	cp.query = q
	cp.cursor = 0
	cp.applyFilter()
}

// selected returns the currently highlighted entry, or "" if none.
func (cp *cmdPickerModel) selected() string {
	if len(cp.filtered) == 0 {
		return ""
	}
	return cp.filtered[cp.cursor]
}

// handleKey processes navigation keys while the picker is active.
func (cp *cmdPickerModel) handleKey(msg tea.KeyPressMsg) (consumed bool, sel string) {
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
			cp.dismiss()
			return true, sel
		}
		return true, ""
	case tea.KeyEsc:
		cp.dismiss()
		return true, ""
	}
	return false, ""
}

// View renders the command picker popup.
func (cp cmdPickerModel) View() string {
	if !cp.active {
		return ""
	}

	innerWidth := max(cp.width-4, 20)

	var sb strings.Builder

	if len(cp.filtered) == 0 {
		sb.WriteString(pickerDimStyle.Render("  no matching commands"))
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
				sb.WriteString(pickerCurStyle.Render(entry))
			} else {
				sb.WriteString(pickerDimStyle.Render(entry))
			}
			if i < end-1 {
				sb.WriteString("\n")
			}
		}
	}

	border := pickerBorder.Width(innerWidth)
	return border.Render(sb.String())
}

func (cp *cmdPickerModel) applyFilter() {
	q := strings.ToLower(cp.query)
	if q == "" {
		cp.filtered = make([]string, len(availableCommands))
		copy(cp.filtered, availableCommands)
		return
	}

	var filtered []string
	for _, cmd := range availableCommands {
		// Match against the command without the leading '/'.
		name := strings.TrimPrefix(cmd, "/")
		if strings.Contains(strings.ToLower(name), q) {
			filtered = append(filtered, cmd)
		}
	}
	cp.filtered = filtered
}
