package chatview

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// DisplayItem is the interface for all renderable message types.
type DisplayItem interface {
	View(width int) string
	IsLive() bool
	Kind() string
}

// --- ThinkingItem ---

// ThinkingItem is rendered as agent reasoning.
type ThinkingItem struct {
	Agent  string
	Prefix string
	Text   string
	Color  string // hex color string; empty means use default ColorFg
}

func (m *ThinkingItem) View(width int) string {
	prefix := m.Prefix
	if prefix == "" {
		prefix = "🤖"
	}

	header := colorStyle(m.Color).Render(fmt.Sprintf("%s %s", prefix, m.Agent))

	rendered := format.RenderMarkdown(m.Text)
	var sb strings.Builder
	sb.WriteString(header)
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(&sb, "\n %s%s", styles.TreeCorner, line)
		} else {
			fmt.Fprintf(&sb, "\n   %s", line)
		}
	}
	return sb.String()
}

func (m *ThinkingItem) IsLive() bool { return false }
func (m *ThinkingItem) Kind() string { return "thinking" }

// --- ToolCallItem ---

// ToolCallItem represents a single tool invocation.
type ToolCallItem struct {
	CallID    string // tool call ID for matching results
	ToolName  string
	Args      string
	Result    string
	IsError   bool
	Completed bool
	StartTime time.Time
	EndTime   time.Time // frozen when Completed is set
	SpinMsg   string
	FrameIdx  int
}

func (m *ToolCallItem) View(width int) string {
	label := format.FormatToolCall(m.ToolName, m.Args)
	var sb strings.Builder

	contentWidth := max(width-2, 20)

	// Split label into title (first line) and detail lines (rest).
	title, detail, _ := strings.Cut(label, "\n")

	if m.Completed {
		elapsed := ""
		if !m.StartTime.IsZero() {
			end := m.EndTime
			if end.IsZero() {
				end = time.Now()
			}
			elapsed = format.FmtDuration(end.Sub(m.StartTime))
		}

		fmt.Fprintf(&sb, "🔧 %s", styles.ToolNameStyle.Render(title))
		if detail != "" {
			detailWidth := max(contentWidth-4, 20)
			wrapped := format.WordWrap(detail, detailWidth)
			for i, line := range strings.Split(wrapped, "\n") {
				if i == 0 {
					fmt.Fprintf(&sb, "\n %s%s", styles.TreeTee, styles.DimStyle.Render(line))
				} else {
					fmt.Fprintf(&sb, "\n %s%s", styles.TreePipe, styles.DimStyle.Render(line))
				}
			}
		}
		if m.Result != "" {
			resultTxt := format.Truncate(m.Result, 200)
			resultWidth := max(contentWidth-len(styles.TreeCorner)-1, 20)
			suffix := ""
			if elapsed != "" {
				suffix = fmt.Sprintf(" in %s", elapsed)
			}
			resultLine := format.Truncate(resultTxt+suffix, resultWidth)
			if m.IsError {
				fmt.Fprintf(&sb, "\n %s", styles.ToolErrorStyle.Render(styles.TreeCorner+resultLine))
			} else {
				fmt.Fprintf(&sb, "\n %s", styles.ToolResultStyle.Render(styles.TreeCorner+resultLine))
			}
		}
	} else {
		elapsed := ""
		if !m.StartTime.IsZero() {
			elapsed = format.FmtDuration(time.Since(m.StartTime))
		}
		frame := format.SpinnerFrames[m.FrameIdx%len(format.SpinnerFrames)]
		fmt.Fprintf(&sb, "🔧 %s %s %s",
			styles.ToolNameStyle.Render(title),
			styles.DimStyle.Render(elapsed),
			styles.SpinnerStyle.Render(frame),
		)
		if detail != "" {
			detailWidth := max(contentWidth-4, 20)
			wrapped := format.WordWrap(detail, detailWidth)
			for i, line := range strings.Split(wrapped, "\n") {
				if i == 0 {
					fmt.Fprintf(&sb, "\n %s%s", styles.TreeTee, styles.DimStyle.Render(line))
				} else {
					fmt.Fprintf(&sb, "\n %s%s", styles.TreePipe, styles.DimStyle.Render(line))
				}
			}
		}
	}

	return sb.String()
}

func (m *ToolCallItem) IsLive() bool { return !m.Completed }
func (m *ToolCallItem) Kind() string { return "tool_call" }

// --- ToolGroupItem ---

// ToolGroupItem groups parallel calls of the same tool.
type ToolGroupItem struct {
	ToolName  string
	Calls     []*ToolCallItem
	MaxShow   int // 0 = show all, >0 = window
	StartTime time.Time
	EndTime   time.Time // frozen when all calls complete
}

func (m *ToolGroupItem) View(width int) string {
	allDone := !m.IsLive()

	var sb strings.Builder

	if allDone {
		elapsed := ""
		if !m.StartTime.IsZero() {
			end := m.EndTime
			if end.IsZero() {
				end = time.Now()
			}
			elapsed = format.FmtDuration(end.Sub(m.StartTime))
		}
		sb.WriteString("🔧 Used tools\n")
		summary := fmt.Sprintf("Finished with %d tools", len(m.Calls))
		if elapsed != "" {
			summary += fmt.Sprintf(" in %s", elapsed)
		}
		fmt.Fprintf(&sb, " %s%s", styles.TreeCorner, styles.DimStyle.Render(summary))
		return sb.String()
	}

	// Running state — show tree.
	sb.WriteString("🔧 Using tools\n")
	for i, call := range m.Calls {
		isLast := i == len(m.Calls)-1
		connector := styles.TreeTee
		childPrefix := styles.TreePipe
		if isLast {
			connector = styles.TreeCorner
			childPrefix = "  "
		}

		label := format.FormatToolCall(call.ToolName, call.Args)
		callTitle, callDetail, _ := strings.Cut(label, "\n")

		if call.Completed {
			fmt.Fprintf(&sb, "%s%s", connector, styles.ToolNameStyle.Render(callTitle))
			if callDetail != "" {
				detailWidth := max(width-6, 20)
				wrapped := format.WordWrap(callDetail, detailWidth)
				for j, line := range strings.Split(wrapped, "\n") {
					if j == 0 {
						fmt.Fprintf(&sb, "\n%s%s%s", childPrefix, styles.TreeTee, styles.DimStyle.Render(line))
					} else {
						fmt.Fprintf(&sb, "\n%s%s%s", childPrefix, styles.TreePipe, styles.DimStyle.Render(line))
					}
				}
			}
			if call.Result != "" {
				resultTxt := format.Truncate(call.Result, 200)
				elapsed := ""
				if !call.StartTime.IsZero() {
					end := call.EndTime
					if end.IsZero() {
						end = time.Now()
					}
					elapsed = fmt.Sprintf(" in %s", format.FmtDuration(end.Sub(call.StartTime)))
				}
				if call.IsError {
					fmt.Fprintf(&sb, "\n%s%s", childPrefix, styles.ToolErrorStyle.Render(styles.TreeCorner+resultTxt+elapsed))
				} else {
					fmt.Fprintf(&sb, "\n%s%s", childPrefix, styles.ToolResultStyle.Render(styles.TreeCorner+resultTxt+elapsed))
				}
			}
		} else {
			elapsed := ""
			if !call.StartTime.IsZero() {
				elapsed = format.FmtDuration(time.Since(call.StartTime))
			}
			frame := format.SpinnerFrames[call.FrameIdx%len(format.SpinnerFrames)]
			fmt.Fprintf(&sb, "%s%s %s %s",
				connector,
				styles.ToolNameStyle.Render(callTitle),
				styles.DimStyle.Render(elapsed),
				styles.SpinnerStyle.Render(frame),
			)
			if callDetail != "" {
				detailWidth := max(width-6, 20)
				wrapped := format.WordWrap(callDetail, detailWidth)
				for j, line := range strings.Split(wrapped, "\n") {
					if j == 0 {
						fmt.Fprintf(&sb, "\n%s%s%s", childPrefix, styles.TreeTee, styles.DimStyle.Render(line))
					} else {
						fmt.Fprintf(&sb, "\n%s%s%s", childPrefix, styles.TreePipe, styles.DimStyle.Render(line))
					}
				}
			}
		}
		if !isLast {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (m *ToolGroupItem) IsLive() bool {
	for _, c := range m.Calls {
		if c.IsLive() {
			return true
		}
	}
	return false
}

func (m *ToolGroupItem) Kind() string { return "tool_group" }

func (m *ToolGroupItem) AddCall(callID, args string) *ToolCallItem {
	tc := &ToolCallItem{
		CallID:    callID,
		ToolName:  m.ToolName,
		Args:      args,
		StartTime: time.Now(),
		SpinMsg:   format.RandomThinkingMessage(),
	}
	m.Calls = append(m.Calls, tc)
	return tc
}

func (m *ToolGroupItem) FindPending() *ToolCallItem {
	for _, c := range m.Calls {
		if !c.Completed {
			return c
		}
	}
	return nil
}

// --- TaskMessageItem ---

// TaskMessageItem displays the delegation task sent from parent to sub-agent.
type TaskMessageItem struct {
	Text  string
	Color string // hex color string; empty means use default
}

func (m *TaskMessageItem) View(width int) string {
	// Use plain word-wrap instead of markdown rendering — the task message is
	// a brief delegation label that doesn't need glamour formatting, and the
	// width must account for the "📋 " prefix (3 columns).
	text := strings.TrimRight(m.Text, "\n ")
	if w := width - 3; w > 0 {
		text = format.WordWrap(text, w)
	}
	return styles.DimStyle.Render("📋 " + text)
}

func (m *TaskMessageItem) IsLive() bool { return false }
func (m *TaskMessageItem) Kind() string { return "task_message" }

// --- PlanItem ---

// PlanItem displays agent plan text.
type PlanItem struct {
	Agent  string
	Prefix string
	Text   string
	Color  string // hex color string; empty means use ThinkingTextStyle
}

func (m *PlanItem) View(width int) string {
	prefix := m.Prefix
	if prefix == "" {
		prefix = "📝"
	}

	var headerStyle lipgloss.Style
	if m.Color != "" {
		headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(m.Color))
	} else {
		headerStyle = styles.ThinkingTextStyle
	}
	header := headerStyle.Render(fmt.Sprintf("%s %s plan:", prefix, m.Agent))

	rendered := format.RenderMarkdown(m.Text)
	var sb strings.Builder
	sb.WriteString(header)
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(&sb, "\n %s%s", styles.TreeCorner, styles.ThinkingTextStyle.Render(line))
		} else {
			fmt.Fprintf(&sb, "\n   %s", styles.ThinkingTextStyle.Render(line))
		}
	}
	return sb.String()
}

func (m *PlanItem) IsLive() bool { return false }
func (m *PlanItem) Kind() string { return "plan" }

// --- SummaryLineItem ---

// SummaryLineItem is a single-line summary for a disposed sub-agent,
// replacing the full AgentContainer in the parent's Items after the
// sub-agent finishes.
type SummaryLineItem struct {
	Agent         string
	Prefix        string // emoji prefix (e.g. "🤖")
	ProviderLabel string
	FinalAnswer   string
	Color         string // hex color string
	Elapsed       string // pre-formatted duration
	Failed        bool   // true if the agent failed
}

func (m *SummaryLineItem) View(width int) string {
	statusIcon := styles.DimStyle.Render("✓")
	if m.Failed {
		statusIcon = lipgloss.NewStyle().Foreground(styles.ColorError).Render("✗")
	}

	label := m.Agent
	if m.ProviderLabel != "" {
		label += " (" + m.ProviderLabel + ")"
	}

	header := colorStyle(m.Color).Render(fmt.Sprintf("%s %s", m.Prefix, label))

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s", statusIcon, header)

	suffix := ""
	if m.Elapsed != "" {
		suffix = " " + styles.DimStyle.Render(m.Elapsed)
	}

	if m.FinalAnswer != "" {
		// Compute available width for the answer excerpt.
		// Reserve space for the prefix, header, separator, and elapsed.
		excerpt := format.Truncate(m.FinalAnswer, 60)
		fmt.Fprintf(&sb, " %s %s", styles.DimStyle.Render("—"), styles.DimStyle.Render(excerpt))
	}

	sb.WriteString(suffix)
	return sb.String()
}

func (m *SummaryLineItem) IsLive() bool { return false }
func (m *SummaryLineItem) Kind() string { return "summary_line" }

// --- SubAgentRefItem ---

// SubAgentRefItem is a compact inline reference to a sub-agent displayed in
// the parent's Items list. While the sub-agent is running it shows a spinner;
// when done it shows a one-line completion summary. This replaces nesting the
// full AgentContainer inside the parent.
type SubAgentRefItem struct {
	Agent         string
	Prefix        string
	ProviderLabel string
	Color         string
	Task          string // delegation task description
	Status        string // "running", "done", "failed"
	FinalAnswer   string // set on completion
	Elapsed       string // set on completion
	FrameIdx      int    // for spinner animation
}

func (m *SubAgentRefItem) View(width int) string {
	label := m.Agent
	if m.ProviderLabel != "" {
		label += " (" + m.ProviderLabel + ")"
	}

	var sb strings.Builder

	switch m.Status {
	case "done":
		statusIcon := styles.DimStyle.Render("✓")
		header := colorStyle(m.Color).Render(fmt.Sprintf("%s %s", m.Prefix, label))
		fmt.Fprintf(&sb, "%s %s", statusIcon, header)
		if m.FinalAnswer != "" {
			excerpt := format.Truncate(m.FinalAnswer, 60)
			fmt.Fprintf(&sb, " %s %s", styles.DimStyle.Render("—"), styles.DimStyle.Render(excerpt))
		}
		if m.Elapsed != "" {
			fmt.Fprintf(&sb, " %s", styles.DimStyle.Render(m.Elapsed))
		}
	case "failed":
		statusIcon := lipgloss.NewStyle().Foreground(styles.ColorError).Render("✗")
		header := colorStyle(m.Color).Render(fmt.Sprintf("%s %s", m.Prefix, label))
		fmt.Fprintf(&sb, "%s %s", statusIcon, header)
		if m.FinalAnswer != "" {
			excerpt := format.Truncate(m.FinalAnswer, 60)
			fmt.Fprintf(&sb, " %s %s", styles.DimStyle.Render("—"), styles.DimStyle.Render(excerpt))
		}
		if m.Elapsed != "" {
			fmt.Fprintf(&sb, " %s", styles.DimStyle.Render(m.Elapsed))
		}
	default: // "running"
		frame := format.SpinnerFrames[m.FrameIdx%len(format.SpinnerFrames)]
		header := colorStyle(m.Color).Render(fmt.Sprintf("⚡ %s", label))
		fmt.Fprintf(&sb, "%s %s", header, styles.SpinnerStyle.Render(frame))
		if m.Task != "" {
			taskText := m.Task
			if w := width - 4; w > 0 {
				taskText = format.WordWrap(taskText, w)
			}
			fmt.Fprintf(&sb, "\n  %s", styles.DimStyle.Render("Task: "+taskText))
		}
	}

	return sb.String()
}

func (m *SubAgentRefItem) IsLive() bool { return m.Status == "running" }
func (m *SubAgentRefItem) Kind() string { return "sub_agent_ref" }
