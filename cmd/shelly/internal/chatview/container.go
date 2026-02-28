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
// falling back to AnswerPrefixStyle if the color is empty.
func colorStyle(hexColor string) lipgloss.Style {
	if hexColor == "" {
		return styles.AnswerPrefixStyle
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor))
}

// AgentContainer accumulates display items for one agent while it's processing.
type AgentContainer struct {
	Agent       string
	Prefix      string // configurable emoji prefix (e.g. "🤖", "📝", "🦾")
	Items       []DisplayItem
	CallIndex   map[string]*ToolCallItem // callID → ToolCallItem for O(1) lookup
	StartTime   time.Time
	EndTime     time.Time // frozen when Done is set
	SpinMsg     string    // random message picked once at creation, used for initial spinner
	MaxShow     int       // 0 = show all (root), >0 = windowed (sub-agent)
	Done        bool
	FrameIdx    int
	Color       string // hex color string, e.g. "#0969da"; empty means top-level default
	FinalAnswer string
}

// NewAgentContainer creates a new container for the given agent.
func NewAgentContainer(agentName, prefix string, maxShow int, color string) *AgentContainer {
	if prefix == "" {
		prefix = "🤖"
	}
	return &AgentContainer{
		Agent:     agentName,
		Prefix:    prefix,
		CallIndex: make(map[string]*ToolCallItem),
		StartTime: time.Now(),
		SpinMsg:   format.RandomThinkingMessage(),
		MaxShow:   maxShow,
		Color:     color,
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

// View renders visible items with windowing.
func (ac *AgentContainer) View(width int) string {
	if len(ac.Items) == 0 && !ac.Done {
		// Show "thinking..." when no items yet.
		prefix := ac.Prefix
		if prefix == "" {
			prefix = "🤖"
		}
		frame := format.SpinnerFrames[ac.FrameIdx%len(format.SpinnerFrames)]
		return fmt.Sprintf("%s %s is thinking... %s\n",
			prefix, ac.Agent, styles.SpinnerStyle.Render(frame))
	}

	items := ac.Items
	var sb strings.Builder

	// Apply windowing for sub-agents.
	if ac.MaxShow > 0 && len(items) > ac.MaxShow {
		skipped := len(items) - ac.MaxShow
		fmt.Fprintf(&sb, "  %s\n", styles.DimStyle.Render(fmt.Sprintf("... %d more items", skipped)))
		items = items[skipped:]
	}

	for _, item := range items {
		sb.WriteString(item.View(width))
		sb.WriteString("\n")
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
	prefix := ac.Prefix
	if prefix == "" {
		prefix = "🤖"
	}

	headerStyle := colorStyle(ac.Color)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", headerStyle.Render(fmt.Sprintf("%s %s", prefix, ac.Agent)))
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
		case *SubAgentItem:
			it.Container.AdvanceSpinners()
		}
	}
}
