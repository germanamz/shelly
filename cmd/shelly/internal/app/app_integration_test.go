package app

import (
	"fmt"
	"testing"

	"github.com/germanamz/shelly/cmd/shelly/internal/chatview"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/subagentpanel"
	"github.com/germanamz/shelly/cmd/shelly/internal/taskpanel"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tasks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newIntegrationModel creates an AppModel with a sized chatview for integration testing.
func newIntegrationModel() AppModel {
	m := newTestModel()
	cv := chatview.New()
	cv, _ = cv.Update(msgs.ChatViewSetWidthMsg{Width: 120})
	cv, _ = cv.Update(msgs.ChatViewSetHeightMsg{Height: 30})
	m.chatView = cv
	return m
}

// spawnAgent simulates an agent start event flowing through both ChatView and AppModel.
func spawnAgent(m *AppModel, agent, parent, provider string) {
	msg := msgs.AgentStartMsg{
		Agent:         agent,
		Prefix:        "🦾",
		Parent:        parent,
		ProviderLabel: provider,
	}
	m.chatView, _ = m.chatView.Update(msg)
	m.onAgentStart(msg)
}

// endAgent simulates an agent end event flowing through both ChatView and AppModel.
func endAgent(m *AppModel, agent, parent string, u *usage.TokenCount) {
	msg := msgs.AgentEndMsg{
		Agent:  agent,
		Parent: parent,
		Usage:  u,
	}
	m.chatView, _ = m.chatView.Update(msg)
	m.onAgentEnd(msg)
}

// --- Integration Test 1: Spawn and Browse ---

func TestIntegration_SpawnAndBrowse(t *testing.T) {
	m := newIntegrationModel()

	// Start root agent (top-level, should not affect menu bar).
	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	m.onAgentStart(rootMsg)
	assert.False(t, m.menuBar.Visible(), "top-level agent should not show menu bar")

	// Spawn a sub-agent.
	spawnAgent(&m, "coder-auth-7", "root", "anthropic/claude-sonnet-4")

	// Menu bar should now be visible with badge=1.
	require.True(t, m.menuBar.Visible())
	items := m.menuBar.Items()
	require.Len(t, items, 1)
	assert.Equal(t, "Subagents", items[0].Label)
	assert.Equal(t, 1, items[0].Badge)

	// Open the sub-agent panel via Ctrl+B → Enter.
	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)
	m.handleKey(enterMsg())
	require.Equal(t, PanelSubAgents, m.activePanel)
	require.True(t, m.subAgentPanel.Active())

	// Select the agent via Enter.
	m.handleKey(enterMsg())

	// Chat view should now be focused on the agent.
	assert.Equal(t, "coder-auth-7", m.chatView.ViewedAgent())
	assert.Equal(t, PanelNone, m.activePanel, "panel should close after selection")

	// Agent completes.
	finalUsage := usage.TokenCount{InputTokens: 500, OutputTokens: 100}
	endAgent(&m, "coder-auth-7", "root", &finalUsage)

	// Badge should update to 0.
	items = m.menuBar.Items()
	require.Len(t, items, 1)
	assert.Equal(t, 0, items[0].Badge)

	// Usage should be frozen.
	info, ok := m.agentUsage["coder-auth-7"]
	require.True(t, ok)
	assert.True(t, info.Ended)
	assert.Equal(t, 500, info.Usage.InputTokens)
}

// --- Integration Test 2: Nested Navigation ---

func TestIntegration_NestedNavigation(t *testing.T) {
	m := newIntegrationModel()

	// Build agent hierarchy: root → parent → child.
	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "orchestrator-42", "root", "anthropic/claude-sonnet-4")
	spawnAgent(&m, "coder-impl-7", "orchestrator-42", "anthropic/claude-sonnet-4")

	// Badge should be 2.
	items := m.menuBar.Items()
	require.Len(t, items, 1)
	assert.Equal(t, 2, items[0].Badge)

	// Navigate to parent.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "orchestrator-42"})
	assert.Equal(t, "orchestrator-42", m.chatView.ViewedAgent())

	// Navigate deeper to child.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "coder-impl-7"})
	assert.Equal(t, "coder-impl-7", m.chatView.ViewedAgent())
	assert.Len(t, m.chatView.ViewStack(), 2)

	// Breadcrumb should contain both.
	bc := m.chatView.RenderBreadcrumb()
	assert.Contains(t, bc, "root")
	assert.Contains(t, bc, "orchestrator-42")
	assert.Contains(t, bc, "coder-impl-7")

	// Esc back to parent (input is empty by default in test model).
	m.handleKey(escMsg())
	assert.Equal(t, "orchestrator-42", m.chatView.ViewedAgent())

	// Esc back to root.
	m.handleKey(escMsg())
	assert.Empty(t, m.chatView.ViewedAgent())
	assert.Empty(t, m.chatView.ViewStack())
}

// --- Integration Test 3: Input Routing Flow ---

func TestIntegration_InputRoutingFlow(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")

	// Navigate to sub-agent.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "sub-1"})
	assert.Equal(t, "sub-1", m.chatView.ViewedAgent())

	// Submitting to a completed agent should be rejected.
	// First, mark agent as ended.
	m.recordAgentUsage("sub-1", usage.TokenCount{InputTokens: 100}, true)

	// handleSubAgentSubmit should detect ended agent and append error to viewport.
	viewBefore := m.chatView.View()
	m.handleSubAgentSubmit("sub-1", msgs.InputSubmitMsg{Text: "hello"})
	// Error message should have been appended (view content changed).
	viewAfter := m.chatView.View()
	assert.NotEqual(t, viewBefore, viewAfter, "view should change after error is appended")
}

// --- Integration Test 4: Rapid Agent Churn ---

func TestIntegration_RapidAgentChurn(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)

	// Spawn 10 agents.
	for i := range 10 {
		name := fmt.Sprintf("agent-%d", i)
		spawnAgent(&m, name, "root", "anthropic/claude-sonnet-4")
	}

	assert.Equal(t, 10, m.menuBar.Items()[0].Badge)
	assert.Len(t, m.chatView.SubAgents(), 10)

	// Complete all 10.
	for i := range 10 {
		name := fmt.Sprintf("agent-%d", i)
		endAgent(&m, name, "root", nil)
	}

	assert.Equal(t, 0, m.menuBar.Items()[0].Badge)
	// Completed sub-agents are retained for post-completion browsing.
	agents := m.chatView.SubAgents()
	assert.Len(t, agents, 10)
	for _, a := range agents {
		assert.Equal(t, "done", a.Status)
	}

	// Menu bar should still be visible (persists for session).
	assert.True(t, m.menuBar.Visible())
}

// --- Integration Test 5: Status Bar Context ---

func TestIntegration_StatusBarContext(t *testing.T) {
	m := newIntegrationModel()

	// Set session-level data.
	m.tokenCount = "15.2k"
	m.sessionCost = "$0.25"
	m.cacheInfo = "cache 60%"

	// Root view shows session stats.
	segments := m.sessionStatusSegments()
	assert.Contains(t, segments, "15.2k tokens")
	assert.Contains(t, segments, "$0.25")

	// Spawn agent and navigate to it.
	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("sub-1", usage.TokenCount{InputTokens: 5000, OutputTokens: 1200}, false)

	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "sub-1"})
	require.Equal(t, "sub-1", m.chatView.ViewedAgent())

	// Status bar should show agent-specific data.
	agentSegments := m.agentStatusSegments("sub-1")
	assert.Contains(t, agentSegments, "sub-1")
	assert.Contains(t, agentSegments, "anthropic/claude-sonnet-4")
	hasTokens := false
	for _, s := range agentSegments {
		if len(s) >= 6 && s[len(s)-6:] == "tokens" {
			hasTokens = true
		}
	}
	assert.True(t, hasTokens, "agent status should show token count")

	// Keyboard hint should say "esc back to parent".
	hint := m.keyboardHint()
	assert.Contains(t, hint, "esc back to parent")

	// Navigate back — session stats should be restored.
	m.handleKey(escMsg())
	assert.Empty(t, m.chatView.ViewedAgent())
	hint = m.keyboardHint()
	assert.NotContains(t, hint, "esc back to parent")
}

// --- Integration Test 6: Panel and Menu Lifecycle ---

func TestIntegration_PanelAndMenuLifecycle(t *testing.T) {
	m := newIntegrationModel()

	// Fresh session: no menu bar.
	assert.False(t, m.menuBar.Visible())
	assert.Equal(t, 0, m.menuBar.Height())

	// First agent spawns — menu bar appears with hint.
	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")

	assert.True(t, m.menuBar.Visible())
	assert.True(t, m.menuHintActive, "hint should be active on first appearance")
	assert.True(t, m.menuHintShown)

	// Any keypress dismisses hint.
	m.handleKey(keyMsg('x', 0))
	assert.False(t, m.menuHintActive)

	// Open panel.
	m.handleKey(ctrlB())
	m.handleKey(enterMsg())
	require.Equal(t, PanelSubAgents, m.activePanel)
	assert.False(t, m.menuFocused, "menu loses focus when panel opens")

	// Agent completes.
	endAgent(&m, "sub-1", "root", nil)
	assert.Equal(t, 0, m.menuBar.Items()[0].Badge)

	// Close panel.
	m.handleKey(escMsg())
	assert.Equal(t, PanelNone, m.activePanel)

	// Menu bar still visible with dimmed badge (persists for session).
	assert.True(t, m.menuBar.Visible())
	assert.Equal(t, 0, m.menuBar.Items()[0].Badge)
}

// --- Integration Test 7: Compact While Viewing Sub-Agent ---

func TestIntegration_CompactWhileViewingSubAgent(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "child", "root", "anthropic/claude-sonnet-4")

	// Navigate to child.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})
	require.Equal(t, "child", m.chatView.ViewedAgent())
	require.Len(t, m.chatView.ViewStack(), 1)

	// Simulate /compact clearing the view (ChatViewClearMsg resets view stack).
	m.chatView, _ = m.chatView.Update(msgs.ChatViewClearMsg{})

	assert.Empty(t, m.chatView.ViewedAgent())
	assert.Empty(t, m.chatView.ViewStack())
	assert.Equal(t, 0, m.chatView.HeaderHeight())
}

// --- Integration Test 8: View Stack Depth Cap ---

func TestIntegration_ViewStackDepthCap(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)

	// Build a deep chain of agents and navigate to each.
	const maxDepth = 32
	for i := range maxDepth {
		name := fmt.Sprintf("agent-%d", i)
		parent := "root"
		if i > 0 {
			parent = fmt.Sprintf("agent-%d", i-1)
		}
		spawnAgent(&m, name, parent, "anthropic/claude-sonnet-4")
		m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: name})
	}

	assert.Len(t, m.chatView.ViewStack(), maxDepth)
	assert.Equal(t, "agent-31", m.chatView.ViewedAgent())

	// One more push should be rejected.
	spawnAgent(&m, "overflow", "root", "anthropic/claude-sonnet-4")
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "overflow"})
	assert.Len(t, m.chatView.ViewStack(), maxDepth) // unchanged
	assert.Equal(t, "agent-31", m.chatView.ViewedAgent())

	// Can still navigate back through the full stack.
	for i := maxDepth - 1; i >= 0; i-- {
		expected := ""
		if i > 0 {
			expected = fmt.Sprintf("agent-%d", i-1)
		}
		m.handleKey(escMsg())
		assert.Equal(t, expected, m.chatView.ViewedAgent())
	}
	assert.Empty(t, m.chatView.ViewStack())
}

// --- Integration Test: Tasks Panel Integration ---

func TestIntegration_TasksPanelLifecycle(t *testing.T) {
	m := newIntegrationModel()

	// No menu bar initially.
	assert.False(t, m.menuBar.Visible())

	// Task arrives — menu bar appears with "Tasks" item.
	taskList := []tasks.Task{
		{ID: "t-1", Title: "Design API", Status: tasks.StatusPending},
		{ID: "t-2", Title: "Implement auth", Status: tasks.StatusInProgress},
	}
	m.taskPanel.SetTasks(taskList)
	m.onTasksChanged()

	require.True(t, m.menuBar.Visible())
	found := false
	for _, item := range m.menuBar.Items() {
		if item.ID == taskpanel.PanelID {
			found = true
			assert.Equal(t, 2, item.Badge) // 1 pending + 1 in-progress
		}
	}
	assert.True(t, found, "should have Tasks menu item")

	// Open tasks panel via menu.
	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)
	// Navigate to Tasks item (might be second item).
	for _, item := range m.menuBar.Items() {
		if item.ID == taskpanel.PanelID {
			break
		}
		m.handleKey(rightMsg())
	}
	m.handleKey(enterMsg())
	assert.Equal(t, PanelTasks, m.activePanel)
	assert.True(t, m.taskPanel.Active())

	// Close via Esc.
	m.handleKey(escMsg())
	assert.Equal(t, PanelNone, m.activePanel)
}

// --- Integration Test: Both Panels Mutual Exclusion ---

func TestIntegration_PanelMutualExclusion(t *testing.T) {
	m := newIntegrationModel()

	// Set up both sub-agents and tasks.
	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")

	taskList := []tasks.Task{
		{ID: "t-1", Title: "Task 1", Status: tasks.StatusPending},
	}
	m.taskPanel.SetTasks(taskList)
	m.onTasksChanged()

	// Open subagent panel.
	m.handleMenuItemSelected(subagentpanel.PanelID)
	require.Equal(t, PanelSubAgents, m.activePanel)
	assert.True(t, m.subAgentPanel.Active())

	// Switch to tasks panel — should close subagent panel.
	m.handleMenuItemSelected(taskpanel.PanelID)
	assert.Equal(t, PanelTasks, m.activePanel)
	assert.False(t, m.subAgentPanel.Active())
	assert.True(t, m.taskPanel.Active())

	// Toggle tasks panel closed.
	m.handleMenuItemSelected(taskpanel.PanelID)
	assert.Equal(t, PanelNone, m.activePanel)
	assert.False(t, m.taskPanel.Active())
}

// --- Integration Test: Agent Disposal with View Stack ---

func TestIntegration_AgentDisposalWithViewStack(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "child-a", "root", "anthropic/claude-sonnet-4")
	spawnAgent(&m, "child-b", "root", "openai/gpt-4o")

	// Initialize usage for both.
	m.initAgentUsage("child-a", "anthropic/claude-sonnet-4")
	m.initAgentUsage("child-b", "openai/gpt-4o")
	m.recordAgentUsage("child-a", usage.TokenCount{InputTokens: 200}, false)
	m.recordAgentUsage("child-b", usage.TokenCount{InputTokens: 300}, false)

	// Navigate to child-b.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child-b"})
	require.Equal(t, "child-b", m.chatView.ViewedAgent())

	// child-a ends while viewing child-b.
	endAgent(&m, "child-a", "root", &usage.TokenCount{InputTokens: 200})

	// child-a usage should be cleaned up (ended, not on view stack).
	_, hasA := m.agentUsage["child-a"]
	assert.False(t, hasA, "ended agent not on view stack should be cleaned up")

	// child-b usage should still exist (currently viewed).
	_, hasB := m.agentUsage["child-b"]
	assert.True(t, hasB, "currently viewed agent should keep usage")

	// Badge should be 1 (only child-b still running).
	assert.Equal(t, 1, m.menuBar.Items()[0].Badge)

	// child-b ends while we're viewing it — pinned pointer keeps view valid.
	endAgent(&m, "child-b", "root", &usage.TokenCount{InputTokens: 300})
	assert.Equal(t, "child-b", m.chatView.ViewedAgent(), "pinned pointer should keep view")
	assert.Equal(t, 0, m.menuBar.Items()[0].Badge)

	// child-b usage kept because it's still viewed.
	_, hasB = m.agentUsage["child-b"]
	assert.True(t, hasB, "viewed ended agent should keep usage until navigate away")

	// Navigate back — usage should be cleaned up.
	m.handleKey(escMsg())
	assert.Empty(t, m.chatView.ViewedAgent())

	// Trigger cleanup (normally done on next agent end; simulate manually).
	m.cleanupAgentUsage()
	_, hasB = m.agentUsage["child-b"]
	assert.False(t, hasB, "after navigating away, ended agent usage should be cleaned")
}

// --- Integration Test: SubAgent Panel Refresh ---

func TestIntegration_SubAgentPanelRefresh(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")

	// Open panel.
	m.handleKey(ctrlB())
	m.handleKey(enterMsg())
	require.Equal(t, PanelSubAgents, m.activePanel)

	// Panel should have items after refresh (refresh happens in onAgentStart when panel is open).
	m.subAgentPanel.Refresh(m.chatView)
	// The panel's list should reflect the sub-agents.
	agents := m.chatView.SubAgents()
	assert.Len(t, agents, 1)
	assert.Equal(t, "sub-1", agents[0].ID)

	// Spawn another agent while panel is open.
	spawnAgent(&m, "sub-2", "root", "openai/gpt-4o")
	// onAgentStart calls Refresh when panel is active.
	agents = m.chatView.SubAgents()
	assert.Len(t, agents, 2)
}

// --- Integration Test: Keyboard Hints Across States ---

func TestIntegration_KeyboardHintsAcrossStates(t *testing.T) {
	m := newIntegrationModel()

	// No hint initially.
	assert.Empty(t, m.keyboardHint())

	// First agent spawn shows ctrl+b hint.
	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")
	assert.Contains(t, m.keyboardHint(), "ctrl+b")

	// Dismiss hint.
	m.handleKey(keyMsg('x', 0))
	assert.Empty(t, m.keyboardHint())

	// Focus menu → menu hint.
	m.handleKey(ctrlB())
	assert.Contains(t, m.keyboardHint(), "navigate")
	assert.Contains(t, m.keyboardHint(), "select")

	// Open sub-agent panel → panel hint.
	m.handleKey(enterMsg())
	hint := m.keyboardHint()
	assert.Contains(t, hint, "navigate")
	assert.Contains(t, hint, "close")

	// Close panel.
	m.handleKey(escMsg())

	// Navigate to sub-agent → back hint.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "sub-1"})
	assert.Contains(t, m.keyboardHint(), "esc back to parent")

	// Navigate back.
	m.handleKey(escMsg())
	assert.Empty(t, m.keyboardHint())
}

// --- Integration Test: Focus Isolation ---

func TestIntegration_FocusIsolation(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")

	// Open panel with items.
	m.handleKey(ctrlB())
	m.handleKey(enterMsg())
	require.Equal(t, PanelSubAgents, m.activePanel)

	// Up/Down should be consumed by the panel.
	m.handleKey(downMsg())
	m.handleKey(upMsg())
	assert.Equal(t, PanelSubAgents, m.activePanel, "panel should stay open")

	// Esc closes panel, doesn't navigate back in view stack.
	m.handleKey(escMsg())
	assert.Equal(t, PanelNone, m.activePanel)
	assert.Empty(t, m.chatView.ViewedAgent(), "Esc from panel should not affect view stack")

	// Focus menu bar.
	m.handleKey(ctrlB())
	require.True(t, m.menuFocused)

	// Left/Right consumed by menu.
	m.handleKey(rightMsg())
	m.handleKey(leftMsg())
	assert.True(t, m.menuFocused, "menu should stay focused")

	// Esc from menu returns to input, doesn't affect view stack.
	m.handleKey(escMsg())
	assert.False(t, m.menuFocused)
	assert.Empty(t, m.chatView.ViewedAgent())
}

// --- Integration Test: SubAgent Selected Navigates to Agent ---

func TestIntegration_SubAgentSelectedNavigatesToAgent(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")
	spawnAgent(&m, "sub-2", "root", "openai/gpt-4o")

	// Open panel via /subagents.
	m.executeSubagents()
	require.Equal(t, PanelSubAgents, m.activePanel)

	// Move to second item and select.
	m.handleKey(downMsg())
	m.handleKey(enterMsg())

	// Should be viewing the selected agent and panel should be closed.
	assert.Equal(t, PanelNone, m.activePanel)
	assert.NotEmpty(t, m.chatView.ViewedAgent())

	// HeaderHeight should be 1 (breadcrumb visible).
	assert.Equal(t, 1, m.chatView.HeaderHeight())
}

// --- Integration Test: Slash Command /subagents Toggle ---

func TestIntegration_SlashSubagentsToggle(t *testing.T) {
	m := newIntegrationModel()

	// Before any agent: /subagents shows message but doesn't open panel.
	m.executeSubagents()
	assert.Equal(t, PanelNone, m.activePanel)

	// Spawn agent to make menu visible.
	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")

	// Now /subagents opens panel.
	m.executeSubagents()
	assert.Equal(t, PanelSubAgents, m.activePanel)

	// Again toggles it closed.
	m.executeSubagents()
	assert.Equal(t, PanelNone, m.activePanel)
}

// --- Integration Test: Usage Lifecycle ---

func TestIntegration_UsageLifecycle(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")

	// Initial usage is zero.
	info := m.agentUsage["sub-1"]
	assert.Equal(t, 0, info.Usage.InputTokens)
	assert.False(t, info.Ended)

	// Live usage update.
	m.recordAgentUsage("sub-1", usage.TokenCount{InputTokens: 1000, OutputTokens: 500}, false)
	info = m.agentUsage["sub-1"]
	assert.Equal(t, 1000, info.Usage.InputTokens)
	assert.Equal(t, 500, info.Usage.OutputTokens)
	assert.False(t, info.Ended)
	// Provider info preserved.
	assert.Equal(t, "anthropic/claude-sonnet-4", info.ProviderLabel)

	// Navigate to agent so it stays on view stack after ending.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "sub-1"})

	// Agent ends with final usage (kept because it's on view stack).
	finalUsage := usage.TokenCount{InputTokens: 2000, OutputTokens: 800}
	endAgent(&m, "sub-1", "root", &finalUsage)

	info = m.agentUsage["sub-1"]
	assert.True(t, info.Ended)
	assert.Equal(t, 2000, info.Usage.InputTokens)
	assert.Equal(t, "anthropic/claude-sonnet-4", info.ProviderLabel)

	// Navigate away — cleanup removes usage.
	m.handleKey(escMsg())
	m.cleanupAgentUsage()
	_, exists := m.agentUsage["sub-1"]
	assert.False(t, exists, "usage should be cleaned up after navigating away")
}

// --- Helpers for readability ---

// committed is a test helper to access chatview committed lines count.
// Uses the exported ViewStack to verify navigation state without
// accessing private fields.
func assertViewStackDepth(t *testing.T, m *AppModel, expected int) {
	t.Helper()
	assert.Len(t, m.chatView.ViewStack(), expected)
}

func TestIntegration_ViewStackDepthHelper(t *testing.T) {
	m := newIntegrationModel()
	assertViewStackDepth(t, &m, 0)

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "child", "root", "anthropic/claude-sonnet-4")
	m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})

	assertViewStackDepth(t, &m, 1)
}

// Verify that committed field access works by using the chatview's
// exported HasMessages flag as a proxy.
func TestIntegration_ChatViewCommittedProxy(t *testing.T) {
	m := newIntegrationModel()
	assert.False(t, m.chatView.HasMessages)

	m.chatView, _ = m.chatView.Update(msgs.ChatViewCommitUserMsg{Text: "hello"})
	assert.True(t, m.chatView.HasMessages)
}

// --- Phase 12: Polish Integration Tests ---

// TestIntegration_SlashTasksToggle verifies /tasks command opens and toggles the task panel.
func TestIntegration_SlashTasksToggle(t *testing.T) {
	m := newIntegrationModel()

	// Before any tasks: /tasks shows message but doesn't open panel.
	m.executeTasks()
	assert.Equal(t, PanelNone, m.activePanel)

	// Add tasks to make menu visible.
	m.taskPanel.SetTasks([]tasks.Task{
		{ID: "t1", Title: "Fix bug", Status: tasks.StatusPending},
	})
	m.onTasksChanged()
	require.True(t, m.menuBar.Visible())

	// Now /tasks opens panel.
	m.executeTasks()
	assert.Equal(t, PanelTasks, m.activePanel)

	// Again toggles it closed.
	m.executeTasks()
	assert.Equal(t, PanelNone, m.activePanel)
}

// TestIntegration_SlashTasksMutualExclusion verifies /tasks closes an open subagent panel.
func TestIntegration_SlashTasksMutualExclusion(t *testing.T) {
	m := newIntegrationModel()

	rootMsg := msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"}
	m.chatView, _ = m.chatView.Update(rootMsg)
	spawnAgent(&m, "sub-1", "root", "anthropic/claude-sonnet-4")
	m.taskPanel.SetTasks([]tasks.Task{
		{ID: "t1", Title: "Fix bug", Status: tasks.StatusPending},
	})
	m.onTasksChanged()

	// Open subagent panel.
	m.executeSubagents()
	require.Equal(t, PanelSubAgents, m.activePanel)

	// Open tasks panel via menu — should close subagent panel.
	m.handleMenuItemSelected(taskpanel.PanelID)
	assert.Equal(t, PanelTasks, m.activePanel)
}

// TestIntegration_HelpTextContainsTasksAndNavigation verifies /help output includes
// /tasks command and sub-agent navigation section.
func TestIntegration_HelpTextContainsTasksAndNavigation(t *testing.T) {
	help := helpText()
	assert.Contains(t, help, "/tasks")
	assert.Contains(t, help, "View task board")
	assert.Contains(t, help, "Sub-Agent Navigation")
	assert.Contains(t, help, "Navigate back")
}
