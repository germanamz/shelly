package chatview

import (
	"fmt"
	"slices"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// colorStyle returns a lipgloss style with the given hex color as foreground,
// falling back to ColorFg if the color is empty.
func colorStyle(hexColor string) lipgloss.Style {
	if hexColor == "" {
		return lipgloss.NewStyle().Foreground(styles.ColorFg)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor))
}

// AgentContainer accumulates display items for one agent while it's processing.
type AgentContainer struct {
	Agent         string
	Prefix        string // configurable emoji prefix (e.g. "🤖", "📝", "🦾")
	ProviderLabel string // provider display label (e.g. "anthropic/claude-sonnet-4")
	Items         []DisplayItem
	Committed     []string                 // per-agent committed history (user messages, etc.)
	CallIndex     map[string]*ToolCallItem // callID → ToolCallItem for O(1) lookup
	StartTime     time.Time
	EndTime       time.Time // frozen when Done is set
	SpinMsg       string    // random message picked once at creation, used for initial spinner
	MaxShow       int       // 0 = show all (root), >0 = windowed (sub-agent)
	Done          bool
	FrameIdx      int
	Color         string // hex color string, e.g. "#0969da"; empty means top-level default
	FinalAnswer   string
}

// NewAgentContainer creates a new container for the given agent.
func NewAgentContainer(agentName, prefix string, maxShow int, color, providerLabel string) *AgentContainer {
	if prefix == "" {
		prefix = "🤖"
	}
	return &AgentContainer{
		Agent:         agentName,
		Prefix:        prefix,
		ProviderLabel: providerLabel,
		CallIndex:     make(map[string]*ToolCallItem),
		StartTime:     time.Now(),
		SpinMsg:       format.RandomThinkingMessage(),
		MaxShow:       maxShow,
		Color:         color,
	}
}

// AddThinking adds a thinking message.
func (ac *AgentContainer) AddThinking(text string) {
	ac.Items = append(ac.Items, &ThinkingItem{
		Agent:  ac.Agent,
		Prefix: ac.Prefix,
		Text:   text,
		Color:  ac.Color,
	})
}

// AddPlan adds a plan display item.
func (ac *AgentContainer) AddPlan(text string) {
	ac.Items = append(ac.Items, &PlanItem{
		Agent:  ac.Agent,
		Prefix: ac.Prefix,
		Text:   text,
		Color:  ac.Color,
	})
}

// AddToolCall adds a single tool call item.
func (ac *AgentContainer) AddToolCall(callID, toolName, args string) *ToolCallItem {
	tc := &ToolCallItem{
		CallID:    callID,
		ToolName:  toolName,
		Args:      args,
		StartTime: time.Now(),
		SpinMsg:   format.RandomThinkingMessage(),
	}
	ac.Items = append(ac.Items, tc)
	if callID != "" {
		ac.CallIndex[callID] = tc
	}
	return tc
}

// AddGroupCall adds a call to an existing tool group and indexes it.
func (ac *AgentContainer) AddGroupCall(tg *ToolGroupItem, callID, args string) {
	tc := tg.AddCall(callID, args)
	if callID != "" {
		ac.CallIndex[callID] = tc
	}
}

// AddToolGroup adds a tool group for parallel calls of the same tool.
func (ac *AgentContainer) AddToolGroup(toolName string, maxShow int) *ToolGroupItem {
	tg := &ToolGroupItem{
		ToolName:  toolName,
		MaxShow:   maxShow,
		StartTime: time.Now(),
	}
	ac.Items = append(ac.Items, tg)
	return tg
}

// FindPendingCall returns the last incomplete ToolCallItem (not in a group).
func (ac *AgentContainer) FindPendingCall() *ToolCallItem {
	for i := len(ac.Items) - 1; i >= 0; i-- {
		switch item := ac.Items[i].(type) {
		case *ToolCallItem:
			if !item.Completed {
				return item
			}
		case *ToolGroupItem:
			if p := item.FindPending(); p != nil {
				return p
			}
		}
	}
	return nil
}

// FindLastToolGroup returns the last ToolGroupItem for the given tool name.
func (ac *AgentContainer) FindLastToolGroup(toolName string) *ToolGroupItem {
	for i := len(ac.Items) - 1; i >= 0; i-- {
		if tg, ok := ac.Items[i].(*ToolGroupItem); ok && tg.ToolName == toolName {
			return tg
		}
	}
	return nil
}

// CompleteToolCall marks the pending call with the given ID as done.
func (ac *AgentContainer) CompleteToolCall(callID, result string, isError bool) {
	tc := ac.findCallByID(callID)
	if tc == nil {
		tc = ac.FindPendingCall()
	}
	if tc == nil {
		return
	}
	now := time.Now()
	tc.Completed = true
	tc.EndTime = now
	tc.Result = result
	tc.IsError = isError

	// If this call belongs to a group, freeze the group's EndTime when all calls are done.
	for _, item := range ac.Items {
		if tg, ok := item.(*ToolGroupItem); ok && tg.EndTime.IsZero() && slices.Contains(tg.Calls, tc) {
			// tc belongs to this group — check if all calls are done.
			if !tg.IsLive() {
				tg.EndTime = now
			}
			return
		}
	}
}

// findCallByID returns the pending ToolCallItem with the given ID.
func (ac *AgentContainer) findCallByID(callID string) *ToolCallItem {
	if callID == "" {
		return nil
	}
	tc, ok := ac.CallIndex[callID]
	if ok && !tc.Completed {
		return tc
	}
	return nil
}

// IsLive returns true if the agent is still processing.
func (ac *AgentContainer) IsLive() bool { return !ac.Done }

// Kind returns the display item kind.
func (ac *AgentContainer) Kind() string { return "agent" }

// View renders the agent container as a DisplayItem.
// When done, it delegates to CollapsedSummary.
// When live, it renders items with optional header and indentation for sub-agents.
func (ac *AgentContainer) View(width int) string {
	if ac.Done {
		return ac.CollapsedSummary()
	}

	items := ac.Items
	frame := format.SpinnerFrames[ac.FrameIdx%len(format.SpinnerFrames)]

	// No items yet — show "is thinking..." spinner.
	if len(items) == 0 {
		label := ac.Agent
		if ac.ProviderLabel != "" {
			label += " (" + ac.ProviderLabel + ")"
		}
		line := fmt.Sprintf("%s %s is thinking... %s",
			ac.Prefix, label, styles.SpinnerStyle.Render(frame))
		if ac.Color == "" {
			// Top-level: add trailing newline to match original behavior.
			line += "\n"
		}
		return line
	}

	var sb strings.Builder
	isSubAgent := ac.Color != ""

	// Sub-agents get a colored header line.
	if isSubAgent {
		fmt.Fprintf(&sb, "%s %s\n",
			colorStyle(ac.Color).Render(fmt.Sprintf("%s %s", ac.Prefix, ac.Agent)),
			styles.SpinnerStyle.Render(frame),
		)
	}

	// Apply windowing.
	if ac.MaxShow > 0 && len(items) > ac.MaxShow {
		skipped := len(items) - ac.MaxShow
		fmt.Fprintf(&sb, "  %s\n", styles.DimStyle.Render(fmt.Sprintf("... %d more items", skipped)))
		items = items[skipped:]
	}

	if isSubAgent {
		// Sub-agent: indent items with tree-pipe.
		for _, item := range items {
			for line := range strings.SplitSeq(item.View(width-4), "\n") {
				fmt.Fprintf(&sb, "  %s%s\n", styles.TreePipe, line)
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	// Top-level: render items flat.
	for _, item := range items {
		sb.WriteString(item.View(width))
		sb.WriteString("\n")
	}

	// Show a spinner when all items are completed but the agent is still
	// processing (e.g., after delegation results return and the agent makes
	// another LLM call to summarize).
	if !ac.hasLiveItems() {
		fmt.Fprintf(&sb, "%s %s is thinking... %s\n",
			ac.Prefix, ac.Agent, styles.SpinnerStyle.Render(frame))
	}

	return sb.String()
}

// ViewFlat renders the agent container's items flat (like a top-level agent),
// ignoring MaxShow windowing and sub-agent styling (no tree-pipe indentation,
// no colored header). Used when the agent is focused in the single chat view.
func (ac *AgentContainer) ViewFlat(width int) string {
	if ac.Done {
		return ac.CollapsedSummary()
	}

	items := ac.Items
	frame := format.SpinnerFrames[ac.FrameIdx%len(format.SpinnerFrames)]

	if len(items) == 0 {
		label := ac.Agent
		if ac.ProviderLabel != "" {
			label += " (" + ac.ProviderLabel + ")"
		}
		return fmt.Sprintf("%s %s is thinking... %s\n",
			ac.Prefix, label, styles.SpinnerStyle.Render(frame))
	}

	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(item.View(width))
		sb.WriteString("\n")
	}

	if !ac.hasLiveItems() {
		fmt.Fprintf(&sb, "%s %s is thinking... %s\n",
			ac.Prefix, ac.Agent, styles.SpinnerStyle.Render(frame))
	}

	return sb.String()
}

// CollapsedSummary returns a summary after agent completion, including the final answer if present.
func (ac *AgentContainer) CollapsedSummary() string {
	end := ac.EndTime
	if end.IsZero() {
		end = time.Now()
	}
	elapsed := format.FmtDuration(end.Sub(ac.StartTime))
	headerStyle := colorStyle(ac.Color)

	label := ac.Agent
	if ac.ProviderLabel != "" {
		label += " (" + ac.ProviderLabel + ")"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", headerStyle.Render(fmt.Sprintf("%s %s", ac.Prefix, label)))

	// Include collapsed subagent summaries so the last message from each
	// subagent remains visible after the parent collapses.
	for _, item := range ac.Items {
		switch sa := item.(type) {
		case *SubAgentRefItem:
			fmt.Fprintf(&sb, " %s%s\n", styles.TreePipe, sa.View(0))
		case *SummaryLineItem:
			fmt.Fprintf(&sb, " %s%s\n", styles.TreePipe, sa.View(0))
		}
	}

	if ac.FinalAnswer != "" {
		rendered := format.RenderMarkdown(ac.FinalAnswer)
		lines := strings.Split(rendered, "\n")
		// Trim trailing empty lines from rendered output.
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

// hasLiveItems returns true if any item in the container is still in progress.
func (ac *AgentContainer) hasLiveItems() bool {
	for _, item := range ac.Items {
		if item.IsLive() {
			return true
		}
	}
	return false
}

// AdvanceSpinners increments spinner frames for all live items.
func (ac *AgentContainer) AdvanceSpinners() {
	ac.FrameIdx++
	for _, item := range ac.Items {
		switch it := item.(type) {
		case *ToolCallItem:
			if !it.Completed {
				it.FrameIdx++
			}
		case *ToolGroupItem:
			for _, c := range it.Calls {
				if !c.Completed {
					c.FrameIdx++
				}
			}
		case *SubAgentRefItem:
			if it.Status == "running" {
				it.FrameIdx++
			}
		}
	}
}
