package main

import (
	"fmt"
	"strings"
	"time"
)

// displayItem is the interface for all renderable message types.
type displayItem interface {
	View(width int) string
	IsLive() bool
	Kind() string
}

// --- thinkingItem ---
// Rendered as agent reasoning:
//
//	🤖 <agent name>
//	 └ <text>
type thinkingItem struct {
	agent  string
	prefix string
	text   string
}

func (m *thinkingItem) View(width int) string {
	prefix := m.prefix
	if prefix == "" {
		prefix = "🤖"
	}
	header := fmt.Sprintf("%s %s", prefix, m.agent)

	rendered := renderMarkdown(m.text)
	var sb strings.Builder
	sb.WriteString(header)
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(&sb, "\n %s%s", treeCorner, line)
		} else {
			fmt.Fprintf(&sb, "\n   %s", line)
		}
	}
	return sb.String()
}

func (m *thinkingItem) IsLive() bool { return false }
func (m *thinkingItem) Kind() string { return "thinking" }

// --- toolCallItem ---
// While running:
//
//	🔧 <tool name>(args) 2s, 900 tokens
//
// After finished:
//
//	🔧 <tool name>(args)
//	 └ <result> in 2s, 900 tokens
type toolCallItem struct {
	callID    string // tool call ID for matching results
	toolName  string
	args      string
	result    string
	isError   bool
	completed bool
	startTime time.Time
	spinMsg   string
	frameIdx  int
}

func (m *toolCallItem) View(width int) string {
	label := formatToolCall(m.toolName, m.args)
	var sb strings.Builder

	contentWidth := max(width-2, 20)

	if m.completed {
		elapsed := ""
		if !m.startTime.IsZero() {
			elapsed = fmtDuration(time.Since(m.startTime))
		}

		fmt.Fprintf(&sb, "🔧 %s", toolNameStyle.Render(label))
		if m.result != "" {
			resultTxt := truncate(m.result, 200)
			resultWidth := max(contentWidth-len(treeCorner)-1, 20)
			suffix := ""
			if elapsed != "" {
				suffix = fmt.Sprintf(" in %s", elapsed)
			}
			resultLine := truncate(resultTxt+suffix, resultWidth)
			if m.isError {
				fmt.Fprintf(&sb, "\n %s", toolErrorStyle.Render(treeCorner+resultLine))
			} else {
				fmt.Fprintf(&sb, "\n %s", toolResultStyle.Render(treeCorner+resultLine))
			}
		}
	} else {
		elapsed := ""
		if !m.startTime.IsZero() {
			elapsed = fmtDuration(time.Since(m.startTime))
		}
		frame := spinnerFrames[m.frameIdx%len(spinnerFrames)]
		fmt.Fprintf(&sb, "🔧 %s %s %s",
			toolNameStyle.Render(label),
			dimStyle.Render(elapsed),
			spinnerStyle.Render(frame),
		)
	}

	return sb.String()
}

func (m *toolCallItem) IsLive() bool { return !m.completed }
func (m *toolCallItem) Kind() string { return "tool_call" }

// --- toolGroupItem ---
// While running:
//
//	🔧 Using tools
//	├─ <tool>(args)
//	│  └ <result> in 2s
//	├─ <tool>(args) 0.1s
//	└ 1050 tokens
//
// After finished:
//
//	🔧 Used tools
//	└ Finished with N tools in 2.2s
type toolGroupItem struct {
	toolName  string
	calls     []*toolCallItem
	maxShow   int // 0 = show all, >0 = window
	startTime time.Time
}

func (m *toolGroupItem) View(width int) string {
	allDone := !m.IsLive()

	var sb strings.Builder

	if allDone {
		elapsed := ""
		if !m.startTime.IsZero() {
			elapsed = fmtDuration(time.Since(m.startTime))
		}
		sb.WriteString("🔧 Used tools\n")
		summary := fmt.Sprintf("Finished with %d tools", len(m.calls))
		if elapsed != "" {
			summary += fmt.Sprintf(" in %s", elapsed)
		}
		fmt.Fprintf(&sb, " %s%s", treeCorner, dimStyle.Render(summary))
		return sb.String()
	}

	// Running state — show tree.
	sb.WriteString("🔧 Using tools\n")
	for i, call := range m.calls {
		isLast := i == len(m.calls)-1
		connector := treeTee
		childPrefix := treePipe
		if isLast {
			connector = treeCorner
			childPrefix = "  "
		}

		label := formatToolCall(call.toolName, call.args)

		if call.completed {
			fmt.Fprintf(&sb, "%s%s", connector, toolNameStyle.Render(label))
			if call.result != "" {
				resultTxt := truncate(call.result, 200)
				elapsed := ""
				if !call.startTime.IsZero() {
					elapsed = fmt.Sprintf(" in %s", fmtDuration(time.Since(call.startTime)))
				}
				if call.isError {
					fmt.Fprintf(&sb, "\n%s%s", childPrefix, toolErrorStyle.Render(treeCorner+resultTxt+elapsed))
				} else {
					fmt.Fprintf(&sb, "\n%s%s", childPrefix, toolResultStyle.Render(treeCorner+resultTxt+elapsed))
				}
			}
		} else {
			elapsed := ""
			if !call.startTime.IsZero() {
				elapsed = fmtDuration(time.Since(call.startTime))
			}
			frame := spinnerFrames[call.frameIdx%len(spinnerFrames)]
			fmt.Fprintf(&sb, "%s%s %s %s",
				connector,
				toolNameStyle.Render(label),
				dimStyle.Render(elapsed),
				spinnerStyle.Render(frame),
			)
		}
		if !isLast {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (m *toolGroupItem) IsLive() bool {
	for _, c := range m.calls {
		if c.IsLive() {
			return true
		}
	}
	return false
}

func (m *toolGroupItem) Kind() string { return "tool_group" }

func (m *toolGroupItem) addCall(callID, args string) *toolCallItem {
	tc := &toolCallItem{
		callID:    callID,
		toolName:  m.toolName,
		args:      args,
		startTime: time.Now(),
		spinMsg:   randomThinkingMessage(),
	}
	m.calls = append(m.calls, tc)
	return tc
}

func (m *toolGroupItem) findPending() *toolCallItem {
	for _, c := range m.calls {
		if !c.completed {
			return c
		}
	}
	return nil
}

// --- subAgentItem ---
// While running: scrollable container with inner items (max 4 visible lines).
// After finished:
//
//	🤖 <sub agent name>
//	└ Finished in 5.3s
type subAgentItem struct {
	container *agentContainer
}

func (m *subAgentItem) View(width int) string {
	prefix := m.container.prefix
	if prefix == "" {
		prefix = "🤖"
	}

	if m.container.done {
		elapsed := fmtDuration(time.Since(m.container.startTime))
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s %s\n", prefix, m.container.agent)
		fmt.Fprintf(&sb, "%s%s", treeCorner, dimStyle.Render(fmt.Sprintf("Finished in %s", elapsed)))
		return sb.String()
	}

	var sb strings.Builder
	frame := spinnerFrames[m.container.frameIdx%len(spinnerFrames)]
	fmt.Fprintf(&sb, "%s %s\n",
		subAgentStyle.Render(fmt.Sprintf("%s %s", prefix, m.container.agent)),
		spinnerStyle.Render(frame),
	)

	items := m.container.items
	if m.container.maxShow > 0 && len(items) > m.container.maxShow {
		skipped := len(items) - m.container.maxShow
		fmt.Fprintf(&sb, "  %s\n", dimStyle.Render(fmt.Sprintf("... %d more items", skipped)))
		items = items[skipped:]
	}

	for _, item := range items {
		for line := range strings.SplitSeq(item.View(width-4), "\n") {
			fmt.Fprintf(&sb, "  %s%s\n", treePipe, line)
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (m *subAgentItem) IsLive() bool { return !m.container.done }
func (m *subAgentItem) Kind() string { return "sub_agent" }

// --- planItem ---
type planItem struct {
	agent  string
	prefix string
	text   string
}

func (m *planItem) View(width int) string {
	prefix := m.prefix
	if prefix == "" {
		prefix = "📝"
	}
	header := fmt.Sprintf("%s %s plan:", prefix, m.agent)

	rendered := renderMarkdown(m.text)
	var sb strings.Builder
	sb.WriteString(thinkingTextStyle.Render(header))
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(&sb, "\n %s%s", treeCorner, thinkingTextStyle.Render(line))
		} else {
			fmt.Fprintf(&sb, "\n   %s", thinkingTextStyle.Render(line))
		}
	}
	return sb.String()
}

func (m *planItem) IsLive() bool { return false }
func (m *planItem) Kind() string { return "plan" }
