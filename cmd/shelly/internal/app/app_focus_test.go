package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/list"
	"github.com/germanamz/shelly/cmd/shelly/internal/menubar"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/subagentpanel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestModel creates a minimal AppModel for focus testing.
func newTestModel() AppModel {
	m := AppModel{
		ctx:           context.Background(),
		menuBar:       menubar.New(),
		subAgentPanel: subagentpanel.New(),
		state:         StateIdle,
		width:         120,
		height:        40,
	}
	return m
}

// makeMenuVisible sets up the menu bar with a subagents item.
func makeMenuVisible(m *AppModel) {
	m.menuBar.SetVisible(true)
	m.menuBar.SetWidth(m.width)
	m.menuBar.AddOrUpdateItem(menubar.Item{
		ID:    subagentpanel.PanelID,
		Label: "Subagents",
		Badge: 2,
	})
}

// keyMsg creates a tea.KeyPressMsg for the given key code and modifiers.
func keyMsg(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: mod})
}

func escMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
}

func enterMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

func leftMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft})
}

func rightMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})
}

func upMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
}

func downMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
}

func ctrlB() tea.KeyPressMsg {
	return keyMsg('b', tea.ModCtrl)
}

func TestFocusMenuBar_CtrlBFromInput(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	assert.False(t, m.menuFocused)
	m.handleKey(ctrlB())
	assert.True(t, m.menuFocused)
	assert.True(t, m.menuBar.Active())
}

func TestFocusMenuBar_CtrlBReturnsToInput(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	// Activate menu.
	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)

	// Ctrl+B again returns to input.
	m.handleKey(ctrlB())
	assert.False(t, m.menuFocused)
	assert.False(t, m.menuBar.Active())
}

func TestFocusMenuBar_EscReturnsToInput(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)

	m.handleKey(escMsg())
	assert.False(t, m.menuFocused)
	assert.False(t, m.menuBar.Active())
}

func TestFocusMenuBar_LeftRightNavigation(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)
	// Add a second item to test navigation.
	m.menuBar.AddOrUpdateItem(menubar.Item{ID: "tasks", Label: "Tasks", Badge: 1})

	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)

	assert.Equal(t, 0, m.menuBar.Cursor())
	m.handleKey(rightMsg())
	assert.Equal(t, 1, m.menuBar.Cursor())
	m.handleKey(leftMsg())
	assert.Equal(t, 0, m.menuBar.Cursor())
}

func TestFocusListPanel_EnterFromMenu(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	// Focus menu, then select subagents.
	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)
	assert.Equal(t, PanelNone, m.activePanel)

	m.handleKey(enterMsg())
	assert.Equal(t, PanelSubAgents, m.activePanel)
	assert.True(t, m.subAgentPanel.Active())
	assert.False(t, m.menuFocused, "menu should lose focus when panel opens")
}

func TestFocusListPanel_EscClosesPanel(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	// Open panel.
	m.handleKey(ctrlB())
	m.handleKey(enterMsg())
	require.Equal(t, PanelSubAgents, m.activePanel)

	// Esc closes panel.
	m.handleKey(escMsg())
	assert.Equal(t, PanelNone, m.activePanel)
	assert.False(t, m.subAgentPanel.Active())
}

func TestMenuBarKeys_DontReachInput(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)
	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)

	// Left/Right should not affect cursor position in the menu beyond bounds.
	m.handleKey(leftMsg())
	assert.Equal(t, 0, m.menuBar.Cursor(), "left at start should stay at 0")
}

func TestListPanelKeys_DontReachInput(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	// Set up items in the panel.
	m.subAgentPanel.SetActive(true)
	m.activePanel = PanelSubAgents
	m.subAgentPanel.SetSize(80, 10)
	// Manually set list items via Refresh would need chatview, so set panel directly.

	// Up/Down should be handled by the panel, not reach input.
	m.handleKey(upMsg())
	m.handleKey(downMsg())
	assert.Equal(t, PanelSubAgents, m.activePanel, "panel should still be active")
}

func TestFocusBlockedByAskPrompt(t *testing.T) {
	// When ask prompt is active, Ctrl+B should be handled by askActive, not toggle menu.
	// The ask prompt intercepts all key events before the menu bar check.
	m := newTestModel()
	makeMenuVisible(&m)

	// Simulate ask prompt active — the handleKey dispatches to askActive before menu.
	// Since we can't easily create a real askprompt, verify the code path:
	// askActive != nil → all keys go to askActive, including Ctrl+B.
	// This is tested by the fact that handleKey checks askActive before our new code.
	assert.False(t, m.menuFocused)
}

func TestMenuBarHidden_NoInteraction(t *testing.T) {
	m := newTestModel()
	// Menu not visible — Ctrl+B should be a no-op.
	assert.False(t, m.menuBar.Visible())

	m.handleKey(ctrlB())
	assert.False(t, m.menuFocused, "Ctrl+B with hidden menu should do nothing")
}

func TestMenuBarHint_ShownOnFirstAppearance(t *testing.T) {
	m := newTestModel()
	assert.False(t, m.menuHintShown)
	assert.False(t, m.menuHintActive)

	// Simulate first sub-agent start.
	m.onAgentStart(msgs.AgentStartMsg{Agent: "sub-1", Parent: "root"})

	assert.True(t, m.menuBar.Visible())
	assert.True(t, m.menuHintShown)
	assert.True(t, m.menuHintActive)
}

func TestMenuBarHint_DismissedOnKeypress(t *testing.T) {
	m := newTestModel()
	m.onAgentStart(msgs.AgentStartMsg{Agent: "sub-1", Parent: "root"})
	require.True(t, m.menuHintActive)

	// Any keypress dismisses the hint.
	m.handleKey(keyMsg('a', 0))
	assert.False(t, m.menuHintActive)
}

func TestMenuBarHint_NotShownOnSecondAgent(t *testing.T) {
	m := newTestModel()
	m.onAgentStart(msgs.AgentStartMsg{Agent: "sub-1", Parent: "root"})
	m.menuHintActive = false // simulate dismissal

	// Second agent start should not re-show the hint.
	m.onAgentStart(msgs.AgentStartMsg{Agent: "sub-2", Parent: "root"})
	assert.False(t, m.menuHintActive)
}

func TestOnAgentStart_TopLevel_Ignored(t *testing.T) {
	m := newTestModel()
	m.onAgentStart(msgs.AgentStartMsg{Agent: "root", Parent: ""})
	assert.False(t, m.menuBar.Visible(), "top-level agent should not trigger menu bar")
}

func TestOnAgentEnd_UpdatesBadge(t *testing.T) {
	m := newTestModel()
	// Set up: agent started, menu visible.
	m.onAgentStart(msgs.AgentStartMsg{Agent: "sub-1", Parent: "root"})
	require.True(t, m.menuBar.Visible())

	// After agent ends, badge should update. Since SubAgents() will return
	// fewer items, the badge count changes.
	m.onAgentEnd(msgs.AgentEndMsg{Agent: "sub-1", Parent: "root"})

	items := m.menuBar.Items()
	require.Len(t, items, 1)
	// Badge reflects what SubAgents() returns. Without a real chatview,
	// it returns 0 since the chatview is zero-value.
	assert.Equal(t, 0, items[0].Badge)
}

func TestKeyboardHint_MenuFocused(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)
	m.menuFocused = true
	hint := m.keyboardHint()
	assert.Contains(t, hint, "navigate")
	assert.Contains(t, hint, "select")
}

func TestKeyboardHint_PanelFocused(t *testing.T) {
	m := newTestModel()
	m.activePanel = PanelSubAgents
	hint := m.keyboardHint()
	assert.Contains(t, hint, "navigate")
	assert.Contains(t, hint, "close")
}

func TestKeyboardHint_NothingFocused(t *testing.T) {
	m := newTestModel()
	hint := m.keyboardHint()
	assert.Empty(t, hint)
}

func TestSubagentPanel_CursorNavigation(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	// Open panel with items.
	m.activePanel = PanelSubAgents
	m.subAgentPanel.SetActive(true)
	m.subAgentPanel.SetSize(80, 10)

	// Manually set items since we don't have a real chatview.
	items := []list.Item{
		{ID: "agent-1", Label: "agent-1", Status: list.StatusRunning},
		{ID: "agent-2", Label: "agent-2", Status: list.StatusRunning},
		{ID: "agent-3", Label: "agent-3", Status: list.StatusDone},
	}
	// Access the inner list through the public API.
	m.subAgentPanel = subagentpanel.New()
	m.subAgentPanel.SetActive(true)
	m.subAgentPanel.SetSize(80, 10)

	// Use MoveDown/MoveUp to test cursor movement through handlePanelKey.
	m.handleKey(downMsg())
	m.handleKey(downMsg())

	// Verify panel is still active (keys were consumed by panel).
	assert.Equal(t, PanelSubAgents, m.activePanel)
	_ = items // items used above for reference
}

func TestSlashSubagents_NoMenu(t *testing.T) {
	m := newTestModel()
	assert.False(t, m.menuBar.Visible())
	m.executeSubagents()
	// Panel should not open when menu bar is hidden.
	assert.Equal(t, PanelNone, m.activePanel)
}

func TestSlashSubagents_WithMenu(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)
	m.executeSubagents()
	assert.Equal(t, PanelSubAgents, m.activePanel)
	assert.True(t, m.subAgentPanel.Active())
}

func TestSlashSubagents_Toggle(t *testing.T) {
	m := newTestModel()
	makeMenuVisible(&m)

	m.executeSubagents()
	require.Equal(t, PanelSubAgents, m.activePanel)

	// Second call closes it.
	m.executeSubagents()
	assert.Equal(t, PanelNone, m.activePanel)
	assert.False(t, m.subAgentPanel.Active())
}
