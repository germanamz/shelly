package taskpanel

import (
	"testing"

	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/pkg/tasks"
	"github.com/stretchr/testify/assert"
)

func TestTaskPanelEmpty(t *testing.T) {
	tp := New()
	assert.Empty(t, tp.View())
	assert.False(t, tp.HasActiveTasks())
}

func TestTaskPanelWithTasks(t *testing.T) {
	tp := New()
	tp, _ = tp.Update(msgs.TasksChangedMsg{
		Tasks: []tasks.Task{
			{ID: "1", Title: "Do stuff", Status: tasks.StatusPending},
			{ID: "2", Title: "More stuff", Status: tasks.StatusInProgress},
		},
	})

	assert.True(t, tp.HasActiveTasks())
	view := tp.View()
	assert.Contains(t, view, "Tasks")
	assert.Contains(t, view, "Do stuff")
	assert.Contains(t, view, "More stuff")
}

func TestTaskPanelAllDone(t *testing.T) {
	tp := New()
	tp, _ = tp.Update(msgs.TasksChangedMsg{
		Tasks: []tasks.Task{
			{ID: "1", Title: "Done thing", Status: tasks.StatusCompleted},
		},
	})

	assert.False(t, tp.HasActiveTasks())
	assert.Empty(t, tp.View(), "should not render when all tasks are done")
}

func TestTaskPanelTickAdvancesSpinner(t *testing.T) {
	tp := New()
	assert.Equal(t, 0, tp.spinnerIdx)

	tp, _ = tp.Update(msgs.TickMsg{})
	assert.Equal(t, 1, tp.spinnerIdx)
}

func TestTaskPanelSetWidth(t *testing.T) {
	tp := New()
	tp, _ = tp.Update(msgs.TaskPanelSetWidthMsg{Width: 120})
	assert.Equal(t, 120, tp.width)
}

func TestTaskPanelAssigneeDisplay(t *testing.T) {
	tp := New()
	tp, _ = tp.Update(msgs.TasksChangedMsg{
		Tasks: []tasks.Task{
			{ID: "1", Title: "Research", Status: tasks.StatusInProgress, Assignee: "agent-1"},
		},
	})

	view := tp.View()
	assert.Contains(t, view, "(agent-1)")
}
