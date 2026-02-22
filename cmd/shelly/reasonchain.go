package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// reasonStep represents a single tool call or thinking step within an agent's chain.
type reasonStep struct {
	kind      string // "call", "result", "thinking"
	toolName  string
	args      string
	text      string
	isError   bool
	completed bool
	spinMsg   string // random message assigned once at creation
}

// reasonChain accumulates reasoning steps for one agent while it's processing.
type reasonChain struct {
	agent    string
	steps    []reasonStep
	frameIdx int
}

var (
	stepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // dim/gray
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta
)

// newReasonChain creates a new chain for the given agent.
func newReasonChain(agent string) *reasonChain {
	return &reasonChain{agent: agent}
}

// addCall adds a tool call step with a random thinking message.
func (rc *reasonChain) addCall(toolName, args string) {
	rc.steps = append(rc.steps, reasonStep{
		kind:     "call",
		toolName: toolName,
		args:     args,
		spinMsg:  randomThinkingMessage(),
	})
}

// addResult marks the last matching call as completed and adds a result step.
func (rc *reasonChain) addResult(text string, isError bool) {
	// Mark last incomplete call as completed.
	for i := len(rc.steps) - 1; i >= 0; i-- {
		if rc.steps[i].kind == "call" && !rc.steps[i].completed {
			rc.steps[i].completed = true
			break
		}
	}
	rc.steps = append(rc.steps, reasonStep{
		kind:      "result",
		text:      text,
		isError:   isError,
		completed: true,
	})
}

// addThinking adds a thinking/text step.
func (rc *reasonChain) addThinking(text string) {
	rc.steps = append(rc.steps, reasonStep{
		kind:      "thinking",
		text:      text,
		completed: true,
		spinMsg:   randomThinkingMessage(),
	})
}

// renderLive renders the chain as it appears while the agent is working.
func (rc *reasonChain) renderLive(verbose bool) string {
	if len(rc.steps) == 0 {
		return ""
	}

	var sb strings.Builder
	frame := spinnerFrames[rc.frameIdx%len(spinnerFrames)]

	for _, step := range rc.steps {
		switch step.kind {
		case "call":
			args := ""
			if step.args != "" {
				args = " " + dimStyle.Render(truncate(step.args, 120))
			}
			if step.completed {
				fmt.Fprintf(&sb, "  %s%s\n",
					stepStyle.Render(fmt.Sprintf("[%s] [calling %s]", rc.agent, step.toolName)),
					args,
				)
			} else {
				fmt.Fprintf(&sb, "  %s%s %s\n",
					stepStyle.Render(fmt.Sprintf("[%s] [calling %s]", rc.agent, step.toolName)),
					args,
					spinnerStyle.Render(fmt.Sprintf("%s %s", frame, step.spinMsg)),
				)
			}
		case "result":
			if verbose {
				if step.isError {
					fmt.Fprintf(&sb, "  %s\n", errorStyle.Render(
						fmt.Sprintf("[%s] [error] %s", rc.agent, truncate(step.text, 200)),
					))
				} else {
					fmt.Fprintf(&sb, "  %s\n", dimStyle.Render(
						fmt.Sprintf("[%s] [result] %s", rc.agent, truncate(step.text, 200)),
					))
				}
			}
		case "thinking":
			maxLen := 80
			if verbose {
				maxLen = 200
			}
			fmt.Fprintf(&sb, "  %s\n", dimStyle.Render(
				fmt.Sprintf("[%s] [thinking] %s", rc.agent, truncate(step.text, maxLen)),
			))
		}
	}

	return sb.String()
}

// collapsedSummary renders the chain as a dim one-line summary after agent completion.
func (rc *reasonChain) collapsedSummary() string {
	toolCounts := make(map[string]int)
	for _, step := range rc.steps {
		if step.kind == "call" {
			toolCounts[step.toolName]++
		}
	}
	if len(toolCounts) == 0 {
		return ""
	}

	var parts []string
	for name, count := range toolCounts {
		if count > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", name, count))
		} else {
			parts = append(parts, name)
		}
	}

	totalSteps := 0
	for _, step := range rc.steps {
		if step.kind == "call" {
			totalSteps++
		}
	}

	return dimStyle.Render(fmt.Sprintf("  [%s: %d steps: %s]", rc.agent, totalSteps, strings.Join(parts, ", ")))
}
