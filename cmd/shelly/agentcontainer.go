package main

import (
	"fmt"
	"strings"
	"time"
)

// agentContainer accumulates display items for one agent while it's processing.
type agentContainer struct {
	agent     string
	prefix    string // configurable emoji prefix (e.g. "🤖", "📝", "🦾")
	items     []displayItem
	callIndex map[string]*toolCallItem // callID → toolCallItem for O(1) lookup
	startTime time.Time
	spinMsg   string // random message picked once at creation, used for initial spinner
	maxShow   int    // 0 = show all (root), >0 = windowed (sub-agent)
	done      bool
	frameIdx  int
}

// newAgentContainer creates a new container for the given agent.
func newAgentContainer(agentName, prefix string, maxShow int) *agentContainer {
	if prefix == "" {
		prefix = "🤖"
	}
	return &agentContainer{
		agent:     agentName,
		prefix:    prefix,
		callIndex: make(map[string]*toolCallItem),
		startTime: time.Now(),
		spinMsg:   randomThinkingMessage(),
		maxShow:   maxShow,
	}
}

// addThinking adds a thinking message.
func (ac *agentContainer) addThinking(text string) {
	ac.items = append(ac.items, &thinkingItem{
		agent:  ac.agent,
		prefix: ac.prefix,
		text:   text,
	})
}

// addPlan adds a plan display item.
func (ac *agentContainer) addPlan(text string) {
	ac.items = append(ac.items, &planItem{
		agent:  ac.agent,
		prefix: ac.prefix,
		text:   text,
	})
}

// addToolCall adds a single tool call item.
func (ac *agentContainer) addToolCall(callID, toolName, args string) *toolCallItem {
	tc := &toolCallItem{
		callID:    callID,
		toolName:  toolName,
		args:      args,
		startTime: time.Now(),
		spinMsg:   randomThinkingMessage(),
	}
	ac.items = append(ac.items, tc)
	if callID != "" {
		ac.callIndex[callID] = tc
	}
	return tc
}

// addGroupCall adds a call to an existing tool group and indexes it.
func (ac *agentContainer) addGroupCall(tg *toolGroupItem, callID, args string) {
	tc := tg.addCall(callID, args)
	if callID != "" {
		ac.callIndex[callID] = tc
	}
}

// addToolGroup adds a tool group for parallel calls of the same tool.
func (ac *agentContainer) addToolGroup(toolName string, maxShow int) *toolGroupItem {
	tg := &toolGroupItem{
		toolName:  toolName,
		maxShow:   maxShow,
		startTime: time.Now(),
	}
	ac.items = append(ac.items, tg)
	return tg
}

// findPendingCall returns the last incomplete toolCallItem (not in a group).
func (ac *agentContainer) findPendingCall() *toolCallItem {
	for i := len(ac.items) - 1; i >= 0; i-- {
		switch item := ac.items[i].(type) {
		case *toolCallItem:
			if !item.completed {
				return item
			}
		case *toolGroupItem:
			if p := item.findPending(); p != nil {
				return p
			}
		}
	}
	return nil
}

// findLastToolGroup returns the last toolGroupItem for the given tool name.
func (ac *agentContainer) findLastToolGroup(toolName string) *toolGroupItem {
	for i := len(ac.items) - 1; i >= 0; i-- {
		if tg, ok := ac.items[i].(*toolGroupItem); ok && tg.toolName == toolName {
			return tg
		}
	}
	return nil
}

// completeToolCall marks the pending call with the given ID as done.
func (ac *agentContainer) completeToolCall(callID, result string, isError bool) {
	tc := ac.findCallByID(callID)
	if tc == nil {
		tc = ac.findPendingCall()
	}
	if tc == nil {
		return
	}
	tc.completed = true
	tc.result = result
	tc.isError = isError
}

// findCallByID returns the pending toolCallItem with the given ID.
func (ac *agentContainer) findCallByID(callID string) *toolCallItem {
	if callID == "" {
		return nil
	}
	tc, ok := ac.callIndex[callID]
	if ok && !tc.completed {
		return tc
	}
	return nil
}

// View renders visible items with windowing.
func (ac *agentContainer) View(width int) string {
	if len(ac.items) == 0 && !ac.done {
		// Show "thinking..." when no items yet.
		prefix := ac.prefix
		if prefix == "" {
			prefix = "🤖"
		}
		frame := spinnerFrames[ac.frameIdx%len(spinnerFrames)]
		return fmt.Sprintf("%s %s is thinking... %s\n",
			prefix, ac.agent, spinnerStyle.Render(frame))
	}

	items := ac.items
	var sb strings.Builder

	// Apply windowing for sub-agents.
	if ac.maxShow > 0 && len(items) > ac.maxShow {
		skipped := len(items) - ac.maxShow
		fmt.Fprintf(&sb, "  %s\n", dimStyle.Render(fmt.Sprintf("... %d more items", skipped)))
		items = items[skipped:]
	}

	for _, item := range items {
		sb.WriteString(item.View(width))
		sb.WriteString("\n")
	}

	return sb.String()
}

// collapsedSummary returns a one-line summary after agent completion.
func (ac *agentContainer) collapsedSummary() string {
	elapsed := fmtDuration(time.Since(ac.startTime))
	prefix := ac.prefix
	if prefix == "" {
		prefix = "🤖"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", prefix, ac.agent)
	fmt.Fprintf(&sb, "%s%s", treeCorner, dimStyle.Render(fmt.Sprintf("Finished in %s", elapsed)))
	return sb.String()
}

// advanceSpinners increments spinner frames for all live items.
func (ac *agentContainer) advanceSpinners() {
	ac.frameIdx++
	for _, item := range ac.items {
		switch it := item.(type) {
		case *toolCallItem:
			if !it.completed {
				it.frameIdx++
			}
		case *toolGroupItem:
			for _, c := range it.calls {
				if !c.completed {
					c.frameIdx++
				}
			}
		case *subAgentItem:
			it.container.advanceSpinners()
		}
	}
}
