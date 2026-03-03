package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/stretchr/testify/assert"
)

func TestFilePickerActivate(t *testing.T) {
	fp := NewFilePicker()
	fp, cmd := fp.Update(msgs.FilePickerActivateMsg{AtPos: 5})

	assert.True(t, fp.Active)
	assert.Equal(t, 5, fp.AtPos)
	assert.NotNil(t, cmd, "should return DiscoverFilesCmd when no entries cached")
}

func TestFilePickerActivateWithCachedEntries(t *testing.T) {
	fp := NewFilePicker()
	fp, _ = fp.Update(msgs.FilePickerEntriesMsg{Entries: []string{"a.go", "b.go"}})
	fp, cmd := fp.Update(msgs.FilePickerActivateMsg{AtPos: 0})

	assert.True(t, fp.Active)
	assert.Nil(t, cmd, "should not discover files when entries are cached")
	assert.Len(t, fp.filtered, 2)
}

func TestFilePickerDismiss(t *testing.T) {
	fp := NewFilePicker()
	fp, _ = fp.Update(msgs.FilePickerActivateMsg{AtPos: 0})
	fp, _ = fp.Update(msgs.FilePickerDismissMsg{})

	assert.False(t, fp.Active)
}

func TestFilePickerQuery(t *testing.T) {
	fp := NewFilePicker()
	fp, _ = fp.Update(msgs.FilePickerEntriesMsg{Entries: []string{"main.go", "test.go", "readme.md"}})
	fp, _ = fp.Update(msgs.FilePickerActivateMsg{AtPos: 0})
	fp, _ = fp.Update(msgs.FilePickerQueryMsg{Query: "main"})

	assert.Len(t, fp.filtered, 1)
	assert.Equal(t, "main.go", fp.filtered[0])
}

func TestFilePickerKeyNavigation(t *testing.T) {
	fp := NewFilePicker()
	fp, _ = fp.Update(msgs.FilePickerEntriesMsg{Entries: []string{"a.go", "b.go", "c.go"}})
	fp, _ = fp.Update(msgs.FilePickerActivateMsg{AtPos: 0})

	assert.Equal(t, 0, fp.cursor)

	fp, _ = fp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 1, fp.cursor)

	fp, _ = fp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, fp.cursor)
}

func TestFilePickerSelection(t *testing.T) {
	fp := NewFilePicker()
	fp, _ = fp.Update(msgs.FilePickerEntriesMsg{Entries: []string{"a.go", "b.go"}})
	fp, _ = fp.Update(msgs.FilePickerActivateMsg{AtPos: 0})

	fp, cmd := fp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	assert.False(t, fp.Active)
	assert.NotNil(t, cmd)

	result := cmd()
	sel, ok := result.(msgs.FilePickerSelectionMsg)
	assert.True(t, ok)
	assert.Equal(t, "a.go", sel.Path)
}

func TestFilePickerEscapeDismisses(t *testing.T) {
	fp := NewFilePicker()
	fp, _ = fp.Update(msgs.FilePickerActivateMsg{AtPos: 0})
	fp, _ = fp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))

	assert.False(t, fp.Active)
}

func TestFilePickerViewInactive(t *testing.T) {
	fp := NewFilePicker()
	assert.Empty(t, fp.View())
}

func TestFilePickerSetEntries(t *testing.T) {
	fp := NewFilePicker()
	fp, _ = fp.Update(msgs.FilePickerEntriesMsg{Entries: []string{"x.go", "y.go"}})

	assert.Len(t, fp.entries, 2)
}
