package input

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

const (
	FilePickerMaxShow    = 4
	FilePickerMaxEntries = 1000
)

// FilePickerModel displays an autocomplete popup for @-mentions.
type FilePickerModel struct {
	Active   bool
	query    string   // text typed after '@'
	AtPos    int      // rune position of '@' in input value
	entries  []string // cached file paths from WalkDir
	filtered []string // filtered by query
	cursor   int      // highlighted entry index
	maxShow  int
	Width    int
}

// NewFilePicker creates a new FilePickerModel.
func NewFilePicker() FilePickerModel {
	return FilePickerModel{maxShow: FilePickerMaxShow}
}

// Activate opens the picker at the given '@' position.
func (fp *FilePickerModel) Activate(atPos int) tea.Cmd {
	fp.Active = true
	fp.AtPos = atPos
	fp.query = ""
	fp.cursor = 0
	fp.filtered = nil
	if len(fp.entries) > 0 {
		fp.applyFilter()
		return nil
	}
	return DiscoverFilesCmd
}

// Dismiss closes the picker.
func (fp *FilePickerModel) Dismiss() {
	fp.Active = false
	fp.query = ""
	fp.filtered = nil
	fp.cursor = 0
}

// SetEntries caches the discovered file list and applies the current filter.
func (fp *FilePickerModel) SetEntries(entries []string) {
	fp.entries = entries
	if fp.Active {
		fp.applyFilter()
	}
}

// SetQuery updates the filter query and re-filters.
func (fp *FilePickerModel) SetQuery(q string) {
	fp.query = q
	fp.cursor = 0
	fp.applyFilter()
}

// selected returns the currently highlighted entry, or "" if none.
func (fp *FilePickerModel) selected() string {
	if len(fp.filtered) == 0 {
		return ""
	}
	return fp.filtered[fp.cursor]
}

// HandleKey processes navigation keys while the picker is active.
func (fp *FilePickerModel) HandleKey(msg tea.KeyPressMsg) (consumed bool, sel string) {
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
			fp.Dismiss()
			return true, sel
		}
		return true, ""
	case tea.KeyEsc:
		fp.Dismiss()
		return true, ""
	}
	return false, ""
}

// View renders the picker popup.
func (fp FilePickerModel) View() string {
	if !fp.Active {
		return ""
	}

	innerWidth := max(fp.Width-4, 20)

	var sb strings.Builder
	sb.WriteString(styles.PickerHintStyle.Render("  files matching: @" + fp.query))
	sb.WriteString("\n")

	if len(fp.filtered) == 0 {
		sb.WriteString(styles.PickerDimStyle.Render("  No files"))
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

func (fp *FilePickerModel) applyFilter() {
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

// DiscoverFilesCmd walks the working directory to discover files.
func DiscoverFilesCmd() tea.Msg {
	wd, err := os.Getwd()
	if err != nil {
		return msgs.FilePickerEntriesMsg{}
	}

	var entries []string
	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		".shelly":      true,
	}

	_ = filepath.WalkDir(wd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(entries) >= FilePickerMaxEntries {
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(wd, path)
		if relErr != nil {
			rel = path
		}
		entries = append(entries, rel)
		return nil
	})

	sort.Strings(entries)
	return msgs.FilePickerEntriesMsg{Entries: entries}
}
