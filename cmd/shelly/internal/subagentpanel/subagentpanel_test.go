package subagentpanel

import (
	"testing"

	"github.com/germanamz/shelly/cmd/shelly/internal/list"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	m := New()
	assert.False(t, m.Active())
	assert.Equal(t, PanelID, m.panel.PanelID())
	assert.Equal(t, 0, m.Height())
}

func TestSetActive(t *testing.T) {
	m := New()
	m.SetActive(true)
	assert.True(t, m.Active())
	m.SetActive(false)
	assert.False(t, m.Active())
}

func TestHeight_Inactive(t *testing.T) {
	m := New()
	m.SetSize(80, 10)
	assert.Equal(t, 0, m.Height(), "inactive panel should report 0 height")
}

func TestHeight_Active(t *testing.T) {
	m := New()
	m.SetActive(true)
	m.SetSize(80, 10)
	assert.Equal(t, 10, m.Height())
}

func TestSelect_Empty(t *testing.T) {
	m := New()
	m.SetActive(true)
	assert.Nil(t, m.Select())
}

func TestSelect_WithItems(t *testing.T) {
	m := New()
	m.SetActive(true)
	m.SetSize(80, 10)
	m.list.SetItems([]list.Item{
		{ID: "agent-1", Label: "agent-1"},
		{ID: "agent-2", Label: "agent-2"},
	})

	sel := m.Select()
	assert.NotNil(t, sel)
	assert.Equal(t, "agent-1", sel.AgentID)

	m.MoveDown()
	sel = m.Select()
	assert.NotNil(t, sel)
	assert.Equal(t, "agent-2", sel.AgentID)
}

func TestAgentStatusMapping(t *testing.T) {
	tests := []struct {
		status   string
		expected list.Status
	}{
		{"running", list.StatusRunning},
		{"done", list.StatusDone},
		{"unknown", list.StatusNone},
		{"", list.StatusNone},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, agentStatusToListStatus(tt.status), "status %q", tt.status)
	}
}

func TestView_Inactive(t *testing.T) {
	m := New()
	assert.Empty(t, m.View())
}

func TestView_Active(t *testing.T) {
	m := New()
	m.SetActive(true)
	m.SetSize(80, 5)
	view := m.View()
	assert.NotEmpty(t, view, "active panel should render something")
}
