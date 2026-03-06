package taskpanel

import (
	"testing"

	"github.com/germanamz/shelly/cmd/shelly/internal/list"
	"github.com/germanamz/shelly/pkg/tasks"
	"github.com/stretchr/testify/assert"
)

func TestTaskPanelEmpty(t *testing.T) {
	tp := New()
	assert.False(t, tp.HasActiveTasks())
	assert.Equal(t, 0, tp.ActiveTaskCount())
}

func TestTaskPanelWithTasks(t *testing.T) {
	tp := New()
	tp.SetTasks([]tasks.Task{
		{ID: "1", Title: "Do stuff", Status: tasks.StatusPending},
		{ID: "2", Title: "More stuff", Status: tasks.StatusInProgress},
	})

	assert.True(t, tp.HasActiveTasks())
	assert.Equal(t, 2, tp.ActiveTaskCount())
}

func TestTaskPanelAllDone(t *testing.T) {
	tp := New()
	tp.SetTasks([]tasks.Task{
		{ID: "1", Title: "Done thing", Status: tasks.StatusCompleted},
	})

	assert.False(t, tp.HasActiveTasks())
	assert.Equal(t, 0, tp.ActiveTaskCount())
}

func TestTaskPanelAdvanceSpinner(t *testing.T) {
	tp := New()
	// No panic on advance.
	tp.AdvanceSpinner()
}

func TestTaskPanelSetSize(t *testing.T) {
	tp := New()
	tp.SetSize(120, 10)
	assert.Equal(t, 0, tp.Height(), "inactive panel should have 0 height")

	tp.SetActive(true)
	tp.SetSize(120, 10)
	assert.Equal(t, 10, tp.Height())
}

func TestTaskPanelAssigneeDisplay(t *testing.T) {
	tp := New()
	tp.SetActive(true)
	tp.SetSize(80, 10)
	tp.SetTasks([]tasks.Task{
		{ID: "1", Title: "Research", Status: tasks.StatusInProgress, Assignee: "agent-1"},
	})

	view := tp.View()
	assert.Contains(t, view, "agent-1")
	assert.Contains(t, view, "Research")
}

func TestTaskPanelSortOrder(t *testing.T) {
	tp := New()
	tp.SetActive(true)
	tp.SetSize(80, 10)
	tp.SetTasks([]tasks.Task{
		{ID: "1", Title: "Completed", Status: tasks.StatusCompleted},
		{ID: "2", Title: "Pending", Status: tasks.StatusPending},
		{ID: "3", Title: "InProgress", Status: tasks.StatusInProgress},
	})

	view := tp.View()
	// Pending should appear before InProgress, which should appear before Completed.
	pendingIdx := indexOf(view, "Pending")
	inProgressIdx := indexOf(view, "InProgress")
	completedIdx := indexOf(view, "Completed")
	assert.Less(t, pendingIdx, inProgressIdx)
	assert.Less(t, inProgressIdx, completedIdx)
}

func TestTaskPanelStatusMapping(t *testing.T) {
	tests := []struct {
		status   tasks.Status
		expected list.Status
	}{
		{tasks.StatusPending, list.StatusPending},
		{tasks.StatusInProgress, list.StatusRunning},
		{tasks.StatusCompleted, list.StatusDone},
		{tasks.StatusFailed, list.StatusFailed},
		{tasks.StatusCanceled, list.StatusNone},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, taskStatusToListStatus(tt.status))
		})
	}
}

func TestTaskPanelViewInactive(t *testing.T) {
	tp := New()
	tp.SetTasks([]tasks.Task{
		{ID: "1", Title: "Task", Status: tasks.StatusPending},
	})

	// Panel is inactive by default — View returns empty from panel chrome.
	assert.Empty(t, tp.View())
}

func TestTaskPanelViewActive(t *testing.T) {
	tp := New()
	tp.SetActive(true)
	tp.SetSize(80, 10)
	tp.SetTasks([]tasks.Task{
		{ID: "1", Title: "Task A", Status: tasks.StatusPending},
	})

	view := tp.View()
	assert.Contains(t, view, "Tasks") // panel title
	assert.Contains(t, view, "Task A")
}

func TestTaskPanelHeightInactive(t *testing.T) {
	tp := New()
	assert.Equal(t, 0, tp.Height())
}

func TestTaskPanelMoveUpDown(t *testing.T) {
	tp := New()
	tp.SetActive(true)
	tp.SetSize(80, 2) // only 2 visible rows
	tp.SetTasks([]tasks.Task{
		{ID: "1", Title: "A", Status: tasks.StatusPending},
		{ID: "2", Title: "B", Status: tasks.StatusPending},
		{ID: "3", Title: "C", Status: tasks.StatusPending},
	})
	// Should not panic when scrolling.
	tp.MoveDown()
	tp.MoveDown()
	tp.MoveUp()
}

func indexOf(s, substr string) int {
	for i := range len(s) - len(substr) + 1 {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
