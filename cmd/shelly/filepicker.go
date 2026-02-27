package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	filePickerMaxShow    = 4
	filePickerMaxEntries = 1000
)

// filePickerModel displays an autocomplete popup for @-mentions.
type filePickerModel struct {
	active   bool
	query    string   // text typed after '@'
	atPos    int      // rune position of '@' in input value
	entries  []string // cached file paths from WalkDir
	filtered []string // filtered by query
	cursor   int      // highlighted entry index
	maxShow  int
	width    int
}

func newFilePicker() filePickerModel {
	return filePickerModel{maxShow: filePickerMaxShow}
}

// activate opens the picker at the given '@' position.
func (fp *filePickerModel) activate(atPos int) tea.Cmd {
	fp.active = true
	fp.atPos = atPos
	fp.query = ""
	fp.cursor = 0
	fp.filtered = nil
	if len(fp.entries) > 0 {
		fp.applyFilter()
		return nil
	}
	return discoverFilesCmd
}

// dismiss closes the picker.
func (fp *filePickerModel) dismiss() {
	fp.active = false
	fp.query = ""
	fp.filtered = nil
	fp.cursor = 0
}

// setEntries caches the discovered file list and applies the current filter.
func (fp *filePickerModel) setEntries(entries []string) {
	fp.entries = entries
	if fp.active {
		fp.applyFilter()
	}
}

// setQuery updates the filter query and re-filters.
func (fp *filePickerModel) setQuery(q string) {
	fp.query = q
	fp.cursor = 0
	fp.applyFilter()
}

// selected returns the currently highlighted entry, or "" if none.
func (fp *filePickerModel) selected() string {
	if len(fp.filtered) == 0 {
		return ""
	}
	return fp.filtered[fp.cursor]
}

// handleKey processes navigation keys while the picker is active.
func (fp *filePickerModel) handleKey(msg tea.KeyPressMsg) (consumed bool, sel string) {
	k := msg.Key()
	switch k.Code {
	case tea.KeyUp:
		if fp.cursor > 0 {
			fp.cursor--
		}
		return true, ""
	case tea.KeyDown:
		if fp.cursor < len(fp.filtered)-1 {
			fp.cursor++
		}
		return true, ""
	case tea.KeyEnter, tea.KeyTab:
		sel := fp.selected()
		if sel != "" {
			fp.dismiss()
			return true, sel
		}
		return true, ""
	case tea.KeyEsc:
		fp.dismiss()
		return true, ""
	}
	return false, ""
}

// View renders the picker popup.
func (fp filePickerModel) View() string {
	if !fp.active {
		return ""
	}

	innerWidth := max(fp.width-4, 20)

	var sb strings.Builder
	sb.WriteString(pickerHintStyle.Render("  files matching: @" + fp.query))
	sb.WriteString("\n")

	if len(fp.filtered) == 0 {
		sb.WriteString(pickerDimStyle.Render("  No files"))
	} else {
		show := min(len(fp.filtered), fp.maxShow)
		// Scroll window around cursor.
		start := 0
		if fp.cursor >= show {
			start = fp.cursor - show + 1
		}
		end := min(start+show, len(fp.filtered))

		for i := start; i < end; i++ {
			entry := fp.filtered[i]
			if i == fp.cursor {
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

func (fp *filePickerModel) applyFilter() {
	q := strings.ToLower(fp.query)
	if q == "" {
		fp.filtered = fp.entries
		if len(fp.filtered) > fp.maxShow*4 {
			fp.filtered = fp.filtered[:fp.maxShow*4]
		}
		return
	}

	var prefix, contains []string
	for _, e := range fp.entries {
		lower := strings.ToLower(e)
		base := strings.ToLower(filepath.Base(e))
		if strings.HasPrefix(base, q) {
			prefix = append(prefix, e)
		} else if strings.Contains(lower, q) {
			contains = append(contains, e)
		}
	}
	filtered := make([]string, 0, len(prefix)+len(contains))
	filtered = append(filtered, prefix...)
	filtered = append(filtered, contains...)
	fp.filtered = filtered
}

// discoverFilesCmd walks the working directory to discover files.
func discoverFilesCmd() tea.Msg {
	var entries []string
	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		".shelly":      true,
	}

	_ = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(entries) >= filePickerMaxEntries {
			return filepath.SkipAll
		}
		entries = append(entries, path)
		return nil
	})

	sort.Strings(entries)
	return filePickerEntriesMsg{entries: entries}
}
