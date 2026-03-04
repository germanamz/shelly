package input

import (
	"bytes"
	"os"
	"path/filepath"
)

const historyMaxSize = 500

// History provides shell-like input history with persistent storage.
// Entries are stored null-byte delimited in a plain text file so that
// multi-line messages are preserved without escaping.
type History struct {
	entries  []string // oldest first
	index    int      // len(entries) = "new input" position
	draft    string   // saves in-progress text when navigating
	filePath string
	maxSize  int
}

// NewHistory loads history from disk (if it exists) and returns a History.
func NewHistory(path string) *History {
	h := &History{
		filePath: path,
		maxSize:  historyMaxSize,
	}
	h.load()
	return h
}

// Add appends an entry to history (memory + disk) and resets navigation.
func (h *History) Add(entry string) {
	if entry == "" {
		return
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
	h.ResetNavigation()
	h.save()
}

// Up moves back in history. On the first call it saves currentText as the draft.
// Returns the history entry and true, or ("", false) if already at the oldest entry.
func (h *History) Up(currentText string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.index <= 0 {
		return "", false
	}
	// Save draft on first navigation away from the "new input" position.
	if h.index == len(h.entries) {
		h.draft = currentText
	}
	h.index--
	return h.entries[h.index], true
}

// Down moves forward in history. At the end it returns the saved draft.
// Returns the text and true, or ("", false) if already past the newest entry.
func (h *History) Down() (string, bool) {
	if h.index >= len(h.entries) {
		return "", false
	}
	h.index++
	if h.index == len(h.entries) {
		return h.draft, true
	}
	return h.entries[h.index], true
}

// ResetNavigation resets the navigation index and clears the draft.
func (h *History) ResetNavigation() {
	h.index = len(h.entries)
	h.draft = ""
}

func (h *History) load() {
	data, err := os.ReadFile(h.filePath)
	if err != nil {
		return
	}
	if len(data) == 0 {
		return
	}
	for p := range bytes.SplitSeq(data, []byte{0}) {
		if s := string(p); s != "" {
			h.entries = append(h.entries, s)
		}
	}
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
	h.index = len(h.entries)
}

func (h *History) save() {
	if h.filePath == "" {
		return
	}
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(h.filePath), 0o750); err != nil {
		return
	}
	var buf bytes.Buffer
	for i, e := range h.entries {
		if i > 0 {
			buf.WriteByte(0)
		}
		buf.WriteString(e)
	}
	_ = os.WriteFile(h.filePath, buf.Bytes(), 0o600)
}
