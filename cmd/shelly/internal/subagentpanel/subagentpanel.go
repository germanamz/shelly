package subagentpanel

import (
	"github.com/germanamz/shelly/cmd/shelly/internal/chatview"
	"github.com/germanamz/shelly/cmd/shelly/internal/list"
	"github.com/germanamz/shelly/cmd/shelly/internal/panel"
)

const PanelID = "subagents"

// SubAgentSelectedMsg is emitted when the user selects a sub-agent from the list.
type SubAgentSelectedMsg struct {
	AgentID string
}

// Model composes panel.Model + list.Model (selectable) to provide
// a sub-agent browser panel. It uses the ChatViewModel.SubAgents() API
// to populate its list.
type Model struct {
	panel panel.Model
	list  list.Model
}

// New creates a new sub-agent panel Model.
func New() Model {
	return Model{
		panel: panel.New(PanelID, "Subagents"),
		list:  list.New(PanelID, true),
	}
}

// Active returns whether the panel is open.
func (m Model) Active() bool { return m.panel.Active() }

// SetActive opens or closes the panel.
func (m *Model) SetActive(active bool) { m.panel.SetActive(active) }

// SetSize updates the panel and list dimensions.
func (m *Model) SetSize(width, height int) {
	m.panel.SetSize(width, height)
	m.list.SetSize(width-2, m.panel.ContentHeight()) // -2 for borders
}

// Height returns the panel's total height (including borders).
// Returns 0 when inactive.
func (m Model) Height() int {
	if !m.panel.Active() {
		return 0
	}
	return m.panel.Height()
}

// MoveUp moves the cursor up in the list.
func (m *Model) MoveUp() { m.list.MoveUp() }

// MoveDown moves the cursor down in the list.
func (m *Model) MoveDown() { m.list.MoveDown() }

// Select returns a SubAgentSelectedMsg for the focused item, or nil.
func (m Model) Select() *SubAgentSelectedMsg {
	sel := m.list.Select()
	if sel == nil {
		return nil
	}
	return &SubAgentSelectedMsg{AgentID: sel.ItemID}
}

// AdvanceSpinner increments the spinner frame counter.
func (m *Model) AdvanceSpinner() { m.list.AdvanceSpinner() }

// Refresh rebuilds the list from the chatview's sub-agent data.
func (m *Model) Refresh(cv chatview.ChatViewModel) {
	agents := cv.SubAgents()
	items := make([]list.Item, len(agents))
	for i, sa := range agents {
		items[i] = list.Item{
			ID:     sa.ID,
			Label:  sa.Label,
			Detail: sa.Provider,
			Status: agentStatusToListStatus(sa.Status),
			Color:  sa.Color,
			Indent: sa.Depth,
		}
	}
	m.list.SetItems(items)
}

// View renders the panel with the list content.
func (m Model) View() string {
	return m.panel.View(m.list.View())
}

func agentStatusToListStatus(status string) list.Status {
	switch status {
	case "running":
		return list.StatusRunning
	case "done":
		return list.StatusDone
	default:
		return list.StatusNone
	}
}
