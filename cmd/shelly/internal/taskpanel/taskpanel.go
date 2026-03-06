package taskpanel

import (
	"sort"

	"github.com/germanamz/shelly/cmd/shelly/internal/list"
	"github.com/germanamz/shelly/cmd/shelly/internal/panel"
	"github.com/germanamz/shelly/pkg/tasks"
)

// PanelID identifies the task panel in menu bar and message routing.
const PanelID = "tasks"

// TaskPanelModel displays the task board as a panel with a read-only list.
type TaskPanelModel struct {
	panel panel.Model
	list  list.Model
	tasks []tasks.Task
}

// New creates a new TaskPanelModel.
func New() TaskPanelModel {
	return TaskPanelModel{
		panel: panel.New(PanelID, "Tasks"),
		list:  list.New(PanelID, false), // read-only: scroll only, no cursor
	}
}

// Active returns whether the panel is open.
func (m TaskPanelModel) Active() bool { return m.panel.Active() }

// SetActive opens or closes the panel.
func (m *TaskPanelModel) SetActive(active bool) { m.panel.SetActive(active) }

// SetSize updates the panel and list dimensions.
func (m *TaskPanelModel) SetSize(width, height int) {
	m.panel.SetSize(width, height)
	m.list.SetSize(width-2, m.panel.ContentHeight()) // -2 for borders
}

// Height returns the panel's total height (including borders).
// Returns 0 when inactive.
func (m TaskPanelModel) Height() int {
	if !m.panel.Active() {
		return 0
	}
	return m.panel.Height()
}

// SetTasks updates the task list and rebuilds list items.
func (m *TaskPanelModel) SetTasks(t []tasks.Task) {
	m.tasks = t
	m.rebuildList()
}

// MoveUp scrolls the list up.
func (m *TaskPanelModel) MoveUp() { m.list.MoveUp() }

// MoveDown scrolls the list down.
func (m *TaskPanelModel) MoveDown() { m.list.MoveDown() }

// AdvanceSpinner increments the spinner frame counter.
func (m *TaskPanelModel) AdvanceSpinner() { m.list.AdvanceSpinner() }

// View renders the panel with the list content.
func (m TaskPanelModel) View() string {
	return m.panel.View(m.list.View())
}

// HasActiveTasks returns true if any task is pending or in progress.
func (m TaskPanelModel) HasActiveTasks() bool {
	for _, t := range m.tasks {
		if t.Status == tasks.StatusPending || t.Status == tasks.StatusInProgress {
			return true
		}
	}
	return false
}

// Tasks returns the current task list.
func (m TaskPanelModel) Tasks() []tasks.Task { return m.tasks }

// ActiveTaskCount returns the number of pending or in-progress tasks.
func (m TaskPanelModel) ActiveTaskCount() int {
	count := 0
	for _, t := range m.tasks {
		if t.Status == tasks.StatusPending || t.Status == tasks.StatusInProgress {
			count++
		}
	}
	return count
}

func (m *TaskPanelModel) rebuildList() {
	// Sort: pending first, in-progress second, completed/failed last.
	sorted := make([]tasks.Task, len(m.tasks))
	copy(sorted, m.tasks)
	sort.Slice(sorted, func(i, j int) bool {
		return taskRank(sorted[i].Status) < taskRank(sorted[j].Status)
	})

	items := make([]list.Item, len(sorted))
	for i, t := range sorted {
		detail := ""
		if t.Assignee != "" {
			detail = t.Assignee
		}
		items[i] = list.Item{
			ID:     t.ID,
			Label:  t.Title,
			Detail: detail,
			Status: taskStatusToListStatus(t.Status),
		}
	}
	m.list.SetItems(items)
}

func taskStatusToListStatus(s tasks.Status) list.Status {
	switch s {
	case tasks.StatusPending:
		return list.StatusPending
	case tasks.StatusInProgress:
		return list.StatusRunning
	case tasks.StatusCompleted:
		return list.StatusDone
	case tasks.StatusFailed:
		return list.StatusFailed
	default:
		return list.StatusNone
	}
}

func taskRank(s tasks.Status) int {
	switch s {
	case tasks.StatusPending:
		return 0
	case tasks.StatusInProgress:
		return 1
	default:
		return 2
	}
}
