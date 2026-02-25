package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// agentContainer accumulates display items for one agent while it's processing.
// It replaces reasonChain with typed display items and windowing support.
type agentContainer struct {
	agent     string
	prefix    string // configurable emoji prefix (e.g. "🤖", "📝", "🦾")
	items     []displayItem
	startTime time.Time
	maxShow   int // 0 = show all (root), >0 = windowed (sub-agent)
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
		startTime: time.Now(),
		maxShow:   maxShow,
	}
}

// addThinking adds a thinking message.
func (ac *agentContainer) addThinking(text string) {
	ac.items = append(ac.items, &thinkingMessage{
		agent:  ac.agent,
		prefix: ac.prefix,
		text:   text,
	})
}

// addPlan adds a plan display item.
func (ac *agentContainer) addPlan(text string) {
	ac.items = append(ac.items, &planMessage{
		agent:  ac.agent,
		prefix: ac.prefix,
		text:   text,
	})
}

// addToolCall adds a single tool call item.
func (ac *agentContainer) addToolCall(toolName, args string) *toolCallMessage {
	tc := &toolCallMessage{
		toolName: toolName,
		args:     args,
		spinMsg:  randomThinkingMessage(),
	}
	ac.items = append(ac.items, tc)
	return tc
}

// addToolGroup adds a tool group for parallel calls of the same tool.
func (ac *agentContainer) addToolGroup(toolName string, maxShow int) *toolGroupMessage {
	tg := &toolGroupMessage{
		toolName: toolName,
		maxShow:  maxShow,
	}
	ac.items = append(ac.items, tg)
	return tg
}

// findPendingCall returns the last incomplete toolCallMessage (not in a group).
func (ac *agentContainer) findPendingCall() *toolCallMessage {
	for i := len(ac.items) - 1; i >= 0; i-- {
		switch item := ac.items[i].(type) {
		case *toolCallMessage:
			if !item.completed {
				return item
			}
		case *toolGroupMessage:
			if p := item.findPending(); p != nil {
				return p
			}
		}
	}
	return nil
}

// findLastToolGroup returns the last toolGroupMessage for the given tool name.
func (ac *agentContainer) findLastToolGroup(toolName string) *toolGroupMessage {
	for i := len(ac.items) - 1; i >= 0; i-- {
		if tg, ok := ac.items[i].(*toolGroupMessage); ok && tg.toolName == toolName {
			return tg
		}
	}
	return nil
}

// completeToolCall marks the last pending call as done with the given result.
func (ac *agentContainer) completeToolCall(result string, isError bool) {
	tc := ac.findPendingCall()
	if tc == nil {
		return
	}
	tc.completed = true
	tc.result = result
	tc.isError = isError
}

// View renders visible items with windowing.
func (ac *agentContainer) View(width int) string {
	if len(ac.items) == 0 && !ac.done {
		// Show spinner when no items yet.
		frame := spinnerFrames[ac.frameIdx%len(spinnerFrames)]
		return fmt.Sprintf("  %s %s\n",
			spinnerStyle.Render(frame),
			spinnerStyle.Render(fmt.Sprintf("%s %s > %s", ac.prefix, ac.agent, randomThinkingMessage())),
		)
	}

	items := ac.items
	var sb strings.Builder

	// Apply windowing.
	if ac.maxShow > 0 && len(items) > ac.maxShow {
		skipped := len(items) - ac.maxShow
		fmt.Fprintf(&sb, "  %s\n", dimStyle.Render(fmt.Sprintf("... %d more items", skipped)))
		items = items[skipped:]
	}

	for _, item := range items {
		sb.WriteString(item.View(width))
		sb.WriteString("\n")
	}

	// Elapsed time.
	if !ac.done {
		elapsed := time.Since(ac.startTime)
		fmt.Fprintf(&sb, "  %s\n", dimStyle.Render(
			fmt.Sprintf("%s %s > %s", ac.prefix, ac.agent, fmtDuration(elapsed)),
		))
	}

	return sb.String()
}

// collapsedSummary returns a dim one-line summary after agent completion.
func (ac *agentContainer) collapsedSummary() string {
	toolCounts := make(map[string]int)
	totalCalls := 0
	for _, item := range ac.items {
		switch it := item.(type) {
		case *toolCallMessage:
			toolCounts[it.toolName]++
			totalCalls++
		case *toolGroupMessage:
			toolCounts[it.toolName] += len(it.calls)
			totalCalls += len(it.calls)
		}
	}

	if totalCalls == 0 {
		return ""
	}

	toolNames := make([]string, 0, len(toolCounts))
	for name := range toolCounts {
		toolNames = append(toolNames, name)
	}
	slices.Sort(toolNames)

	var parts []string
	for _, name := range toolNames {
		count := toolCounts[name]
		if count > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", name, count))
		} else {
			parts = append(parts, name)
		}
	}

	elapsed := time.Since(ac.startTime)
	return dimStyle.Render(fmt.Sprintf("  %s%s %s: %s. Ran for %s",
		treeCorner, ac.prefix, ac.agent, strings.Join(parts, ", "), fmtDuration(elapsed)))
}

// advanceSpinners increments spinner frames for all live items.
func (ac *agentContainer) advanceSpinners() {
	ac.frameIdx++
	for _, item := range ac.items {
		switch it := item.(type) {
		case *toolCallMessage:
			if !it.completed {
				it.frameIdx++
			}
		case *toolGroupMessage:
			for _, c := range it.calls {
				if !c.completed {
					c.frameIdx++
				}
			}
		case *subAgentMessage:
			it.container.advanceSpinners()
		case *spinnerMessage:
			it.frameIdx++
		}
	}
}
