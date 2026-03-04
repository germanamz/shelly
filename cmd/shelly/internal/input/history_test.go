package input

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tempHistoryPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "local", "history")
}

func TestNewHistory_EmptyFile(t *testing.T) {
	h := NewHistory(tempHistoryPath(t))
	assert.Empty(t, h.entries)
	assert.Equal(t, 0, h.index)
}

func TestNewHistory_LoadExisting(t *testing.T) {
	path := tempHistoryPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("hello\x00world"), 0o600))

	h := NewHistory(path)
	assert.Equal(t, []string{"hello", "world"}, h.entries)
	assert.Equal(t, 2, h.index)
}

func TestAdd_AppendsAndPersists(t *testing.T) {
	path := tempHistoryPath(t)
	h := NewHistory(path)

	h.Add("first")
	h.Add("second")

	assert.Equal(t, []string{"first", "second"}, h.entries)
	assert.Equal(t, 2, h.index)

	// Verify persistence.
	h2 := NewHistory(path)
	assert.Equal(t, []string{"first", "second"}, h2.entries)
}

func TestAdd_IgnoresEmpty(t *testing.T) {
	h := NewHistory(tempHistoryPath(t))
	h.Add("")
	assert.Empty(t, h.entries)
}

func TestAdd_TruncatesAtMaxSize(t *testing.T) {
	h := NewHistory(tempHistoryPath(t))
	h.maxSize = 3
	for i := range 5 {
		h.Add(strings.Repeat("x", i+1))
	}
	assert.Len(t, h.entries, 3)
	assert.Equal(t, "xxx", h.entries[0])
}

func TestUpDown_Navigation(t *testing.T) {
	h := NewHistory(tempHistoryPath(t))
	h.Add("a")
	h.Add("b")
	h.Add("c")

	// Navigate up through history.
	text, ok := h.Up("draft")
	assert.True(t, ok)
	assert.Equal(t, "c", text)

	text, ok = h.Up("")
	assert.True(t, ok)
	assert.Equal(t, "b", text)

	text, ok = h.Up("")
	assert.True(t, ok)
	assert.Equal(t, "a", text)

	// At oldest — can't go further up.
	_, ok = h.Up("")
	assert.False(t, ok)

	// Navigate back down.
	text, ok = h.Down()
	assert.True(t, ok)
	assert.Equal(t, "b", text)

	text, ok = h.Down()
	assert.True(t, ok)
	assert.Equal(t, "c", text)

	// Back to draft.
	text, ok = h.Down()
	assert.True(t, ok)
	assert.Equal(t, "draft", text)

	// Can't go further down.
	_, ok = h.Down()
	assert.False(t, ok)
}

func TestDraftPreservation(t *testing.T) {
	h := NewHistory(tempHistoryPath(t))
	h.Add("old")

	// Start typing something, then navigate up.
	text, ok := h.Up("my draft text")
	assert.True(t, ok)
	assert.Equal(t, "old", text)

	// Navigate back down — should restore draft.
	text, ok = h.Down()
	assert.True(t, ok)
	assert.Equal(t, "my draft text", text)
}

func TestResetNavigation(t *testing.T) {
	h := NewHistory(tempHistoryPath(t))
	h.Add("a")
	h.Add("b")

	h.Up("draft")
	h.ResetNavigation()

	assert.Equal(t, 2, h.index)
	assert.Empty(t, h.draft)
}

func TestUpDown_EmptyHistory(t *testing.T) {
	h := NewHistory(tempHistoryPath(t))

	_, ok := h.Up("text")
	assert.False(t, ok)

	_, ok = h.Down()
	assert.False(t, ok)
}

func TestMultiLineEntry(t *testing.T) {
	path := tempHistoryPath(t)
	h := NewHistory(path)

	h.Add("line1\nline2\nline3")
	h.Add("single")

	// Reload and verify multi-line preserved.
	h2 := NewHistory(path)
	assert.Equal(t, []string{"line1\nline2\nline3", "single"}, h2.entries)
}

func TestLoadTruncates(t *testing.T) {
	path := tempHistoryPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))

	// Write more entries than maxSize.
	var entries []string
	for i := range 600 {
		entries = append(entries, strings.Repeat("x", i%10+1))
	}
	data := strings.Join(entries, "\x00")
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))

	h := NewHistory(path)
	assert.Len(t, h.entries, historyMaxSize)
	// Should keep the newest entries.
	assert.Equal(t, entries[100], h.entries[0])
}
