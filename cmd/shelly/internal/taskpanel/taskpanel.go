package taskpanel

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/tasks"
)

// TaskPanelModel displays the task board above the input.
type TaskPanelModel struct {
	tasks      []tasks.Task
	spinnerIdx int
	width      int
}

// New creates a new TaskPanelModel.
func New() TaskPanelModel {
	return TaskPanelModel{}
}

// Update processes messages for the task panel.
func (m TaskPanelModel) Update(msg tea.Msg) (TaskPanelModel, tea.Cmd) {
	switch msg := msg.(type) {
	case msgs.TasksChangedMsg:
		m.tasks = msg.Tasks
		return m, nil
	case msgs.TickMsg:
		m.spinnerIdx++
		return m, nil
	case msgs.TaskPanelSetWidthMsg:
		m.width = msg.Width
		return m, nil
	}
	return m, nil
}

// View renders the task panel. Returns empty string when there are no active tasks.
func (m TaskPanelModel) View() string {
	if len(m.tasks) == 0 {
		return ""
	}

	// Check if all tasks are in terminal states (completed/failed).
	allDone := true
	for _, t := range m.tasks {
		if t.Status == tasks.StatusPending || t.Status == tasks.StatusInProgress {
			allDone = false
			break
		}
	}
	if allDone {
		return ""
	}

	// Count by status.
	var pending, inProgress, completed int
	for _, t := range m.tasks {
		switch t.Status {
		case tasks.StatusPending:
			pending++
		case tasks.StatusInProgress:
			inProgress++
		case tasks.StatusCompleted, tasks.StatusFailed:
			completed++
		}
	}

	// Sort: pending first, in-progress second, completed/failed last.
	sorted := make([]tasks.Task, len(m.tasks))
	copy(sorted, m.tasks)
	sort.Slice(sorted, func(i, j int) bool {
		rank := func(s tasks.Status) int {
			switch s {
			case tasks.StatusPending:
				return 0
			case tasks.StatusInProgress:
				return 1
			default:
				return 2
			}
		}
		return rank(sorted[i].Status) < rank(sorted[j].Status)
	})

	// Show max 6 items.
	if len(sorted) > 6 {
		sorted = sorted[:6]
	}

	frame := format.SpinnerFrames[m.spinnerIdx%len(format.SpinnerFrames)]

	var sb strings.Builder
	header := fmt.Sprintf("Tasks  %d pending  %d in progress  %d completed",
		pending, inProgress, completed)
	sb.WriteString(header)

	for _, t := range sorted {
		sb.WriteString("\n")
		title := t.Title
		assignee := ""
		if t.Assignee != "" {
			assignee = fmt.Sprintf(" (%s)", t.Assignee)
		}
		switch t.Status {
		case tasks.StatusPending:
			fmt.Fprintf(&sb, "○ %s%s", title, assignee)
		case tasks.StatusInProgress:
			fmt.Fprintf(&sb, "%s %s%s",
				styles.SpinnerStyle.Render(frame), title, assignee)
		default: // completed or failed
			sb.WriteString(styles.DimStyle.Render(fmt.Sprintf("✓ %s%s", title, assignee)))
		}
	}

	return sb.String()
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
