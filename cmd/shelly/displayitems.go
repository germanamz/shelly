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

// --- thinkingMessage ---

type thinkingMessage struct {
	agent   string
	prefix  string
	text    string
	elapsed time.Duration
}

func (m *thinkingMessage) View(width int) string {
	prefix := m.prefix
	if prefix == "" {
		prefix = "🤖"
	}
	header := thinkingTextStyle.Render(fmt.Sprintf("%s %s >", prefix, m.agent))

	var sb strings.Builder
	first := true
	for line := range strings.SplitSeq(strings.TrimRight(m.text, "\n"), "\n") {
		if first {
			fmt.Fprintf(&sb, "  %s %s", header, thinkingTextStyle.Render(line))
			first = false
		} else {
			fmt.Fprintf(&sb, "\n  %s", thinkingTextStyle.Render("  "+line))
		}
	}
	if m.elapsed > 0 {
		footer := thinkingFooterStyle.Render(fmt.Sprintf("── Thought for %s", fmtDuration(m.elapsed)))
		sb.WriteString("\n  " + footer)
	}
	return sb.String()
}

func (m *thinkingMessage) IsLive() bool { return false }
func (m *thinkingMessage) Kind() string { return "thinking" }

// --- spinnerMessage ---

type spinnerMessage struct {
	agent    string
	prefix   string
	text     string
	frameIdx int
}

func (m *spinnerMessage) View(width int) string {
	prefix := m.prefix
	if prefix == "" {
		prefix = "🤖"
	}
	frame := spinnerFrames[m.frameIdx%len(spinnerFrames)]
	return fmt.Sprintf("  %s %s",
		spinnerStyle.Render(frame),
		spinnerStyle.Render(fmt.Sprintf("%s %s > %s", prefix, m.agent, m.text)),
	)
}

func (m *spinnerMessage) IsLive() bool { return true }
func (m *spinnerMessage) Kind() string { return "spinner" }

// --- toolCallMessage ---

type toolCallMessage struct {
	toolName  string
	args      string
	result    string
	isError   bool
	completed bool
	spinMsg   string
	frameIdx  int
}

func (m *toolCallMessage) View(width int) string {
	label := formatToolCall(m.toolName, m.args)
	var sb strings.Builder

	if m.completed {
		fmt.Fprintf(&sb, "  %s", toolNameStyle.Render("🔧 "+label))
		if m.result != "" {
			resultTxt := truncate(m.result, 200)
			if m.isError {
				fmt.Fprintf(&sb, "\n  %s", toolErrorStyle.Render(treeCorner+resultTxt))
			} else {
				fmt.Fprintf(&sb, "\n  %s", toolResultStyle.Render(treeCorner+resultTxt))
			}
		}
	} else {
		frame := spinnerFrames[m.frameIdx%len(spinnerFrames)]
		fmt.Fprintf(&sb, "  %s %s",
			toolNameStyle.Render("🔧 "+label),
			spinnerStyle.Render(fmt.Sprintf("%s %s", frame, m.spinMsg)),
		)
	}

	return sb.String()
}

func (m *toolCallMessage) IsLive() bool { return !m.completed }
func (m *toolCallMessage) Kind() string { return "tool_call" }

// --- toolGroupMessage ---

type toolGroupMessage struct {
	toolName string
	calls    []*toolCallMessage
	maxShow  int // 0 = show all, >0 = window
}

func (m *toolGroupMessage) View(width int) string {
	var sb strings.Builder

	items := m.calls
	if m.maxShow > 0 && len(items) > m.maxShow {
		skipped := len(items) - m.maxShow
		fmt.Fprintf(&sb, "  %s\n", dimStyle.Render(fmt.Sprintf("... %d more %s calls", skipped, m.toolName)))
		items = items[skipped:]
	}

	for i, call := range items {
		sb.WriteString(call.View(width))
		if i < len(items)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (m *toolGroupMessage) IsLive() bool {
	for _, c := range m.calls {
		if c.IsLive() {
			return true
		}
	}
	return false
}

func (m *toolGroupMessage) Kind() string { return "tool_group" }

func (m *toolGroupMessage) addCall(args string) *toolCallMessage {
	tc := &toolCallMessage{
		toolName: m.toolName,
		args:     args,
		spinMsg:  randomThinkingMessage(),
	}
	m.calls = append(m.calls, tc)
	return tc
}

func (m *toolGroupMessage) findPending() *toolCallMessage {
	for _, c := range m.calls {
		if !c.completed {
			return c
		}
	}
	return nil
}

// --- subAgentMessage ---

type subAgentMessage struct {
	agent    string
	prefix   string
	items    []displayItem
	maxShow  int
	done     bool
	frameIdx int
}

func (m *subAgentMessage) View(width int) string {
	var sb strings.Builder

	prefix := m.prefix
	if prefix == "" {
		prefix = "🦾"
	}

	if !m.done {
		frame := spinnerFrames[m.frameIdx%len(spinnerFrames)]
		fmt.Fprintf(&sb, "  %s %s\n",
			subAgentStyle.Render(fmt.Sprintf("%s %s", prefix, m.agent)),
			spinnerStyle.Render(frame),
		)
	} else {
		fmt.Fprintf(&sb, "  %s\n",
			subAgentStyle.Render(fmt.Sprintf("%s %s (done)", prefix, m.agent)),
		)
	}

	items := m.items
	if m.maxShow > 0 && len(items) > m.maxShow {
		skipped := len(items) - m.maxShow
		fmt.Fprintf(&sb, "    %s\n", dimStyle.Render(fmt.Sprintf("... %d more items", skipped)))
		items = items[skipped:]
	}

	for _, item := range items {
		// Indent sub-agent items with a tree pipe.
		for line := range strings.SplitSeq(item.View(width-4), "\n") {
			fmt.Fprintf(&sb, "    %s%s\n", treePipe, line)
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (m *subAgentMessage) IsLive() bool { return !m.done }
func (m *subAgentMessage) Kind() string { return "sub_agent" }
