package chatview

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// agentColorStyle returns a lipgloss style for an agent name header.
// If hexColor is empty, it falls back to styles.ColorFg foreground (no bold).
func agentColorStyle(hexColor string) lipgloss.Style {
	if hexColor == "" {
		return lipgloss.NewStyle().Foreground(styles.ColorFg)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor))
}

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

	header := agentColorStyle(m.Color).Render(fmt.Sprintf("%s %s", prefix, m.Agent))

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

// --- SubAgentItem ---

// SubAgentItem wraps a nested agent container.
type SubAgentItem struct {
	Container *AgentContainer
}

func (m *SubAgentItem) View(width int) string {
	prefix := m.Container.Prefix
	if prefix == "" {
		prefix = "🤖"
	}

	if m.Container.Done {
		end := m.Container.EndTime
		if end.IsZero() {
			end = time.Now()
		}
		elapsed := format.FmtDuration(end.Sub(m.Container.StartTime))
		headerStyle := agentColorStyle(m.Container.Color)
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s\n", headerStyle.Render(fmt.Sprintf("%s %s", prefix, m.Container.Agent)))
		if m.Container.FinalAnswer != "" {
			rendered := format.RenderMarkdown(m.Container.FinalAnswer)
			lines := strings.Split(rendered, "\n")
			for len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			for i, line := range lines {
				if i == 0 {
					fmt.Fprintf(&sb, " %s%s\n", styles.TreeCorner, line)
				} else {
					fmt.Fprintf(&sb, "   %s\n", line)
				}
			}
			fmt.Fprintf(&sb, "   %s", styles.DimStyle.Render(fmt.Sprintf("Finished in %s", elapsed)))
		} else {
			fmt.Fprintf(&sb, " %s%s", styles.TreeCorner, styles.DimStyle.Render(fmt.Sprintf("Finished in %s", elapsed)))
		}
		return sb.String()
	}

	var sb strings.Builder
	frame := format.SpinnerFrames[m.Container.FrameIdx%len(format.SpinnerFrames)]

	items := m.Container.Items

	// Show "is thinking..." when the container has no items yet.
	if len(items) == 0 {
		fmt.Fprintf(&sb, "%s %s is thinking... %s",
			prefix, m.Container.Agent, styles.SpinnerStyle.Render(frame))
		return sb.String()
	}

	fmt.Fprintf(&sb, "%s %s\n",
		agentColorStyle(m.Container.Color).Render(fmt.Sprintf("%s %s", prefix, m.Container.Agent)),
		styles.SpinnerStyle.Render(frame),
	)
	if m.Container.MaxShow > 0 && len(items) > m.Container.MaxShow {
		skipped := len(items) - m.Container.MaxShow
		fmt.Fprintf(&sb, "  %s\n", styles.DimStyle.Render(fmt.Sprintf("... %d more items", skipped)))
		items = items[skipped:]
	}

	for _, item := range items {
		for line := range strings.SplitSeq(item.View(width-4), "\n") {
			fmt.Fprintf(&sb, "  %s%s\n", styles.TreePipe, line)
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (m *SubAgentItem) IsLive() bool { return !m.Container.Done }
func (m *SubAgentItem) Kind() string { return "sub_agent" }

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
