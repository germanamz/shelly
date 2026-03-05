package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/stretchr/testify/assert"
)

func TestCmdPickerActivate(t *testing.T) {
	cp := NewCmdPicker()
	cp, _ = cp.Update(msgs.CmdPickerActivateMsg{SlashPos: 0})

	assert.True(t, cp.Active)
	assert.Equal(t, 0, cp.SlashPos)
	assert.Len(t, cp.filtered, len(AvailableCommands))
}

func TestCmdPickerDismiss(t *testing.T) {
	cp := NewCmdPicker()
	cp, _ = cp.Update(msgs.CmdPickerActivateMsg{SlashPos: 0})
	cp, _ = cp.Update(msgs.CmdPickerDismissMsg{})

	assert.False(t, cp.Active)
}

func TestCmdPickerQuery(t *testing.T) {
	cp := NewCmdPicker()
	cp, _ = cp.Update(msgs.CmdPickerActivateMsg{SlashPos: 0})
	cp, _ = cp.Update(msgs.CmdPickerQueryMsg{Query: "hel"})

	assert.Len(t, cp.filtered, 1)
	assert.Equal(t, "/help", cp.filtered[0].Name)
}

func TestCmdPickerKeyNavigation(t *testing.T) {
	cp := NewCmdPicker()
	cp, _ = cp.Update(msgs.CmdPickerActivateMsg{SlashPos: 0})

	assert.Equal(t, 0, cp.cursor)

	cp, _ = cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 1, cp.cursor)

	cp, _ = cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, cp.cursor)
}

func TestCmdPickerSelection(t *testing.T) {
	cp := NewCmdPicker()
	cp, _ = cp.Update(msgs.CmdPickerActivateMsg{SlashPos: 0})

	cp, cmd := cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	assert.False(t, cp.Active)
	assert.NotNil(t, cmd)

	result := cmd()
	sel, ok := result.(msgs.CmdPickerSelectionMsg)
	assert.True(t, ok)
	assert.Equal(t, "/help", sel.Command)
}

func TestCmdPickerEscapeDismisses(t *testing.T) {
	cp := NewCmdPicker()
	cp, _ = cp.Update(msgs.CmdPickerActivateMsg{SlashPos: 0})
	cp, _ = cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))

	assert.False(t, cp.Active)
}

func TestCmdPickerViewInactive(t *testing.T) {
	cp := NewCmdPicker()
	assert.Empty(t, cp.View())
}
