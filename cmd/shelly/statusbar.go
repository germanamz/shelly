package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

var statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

// statusBarModel shows token usage and timing information.
type statusBarModel struct {
	completer modeladapter.Completer
	duration  time.Duration
}

func newStatusBar(completer modeladapter.Completer) statusBarModel {
	return statusBarModel{completer: completer}
}

func (m statusBarModel) View() string {
	ur, ok := m.completer.(modeladapter.UsageReporter)
	if !ok {
		if m.duration > 0 {
			return statusStyle.Render(fmt.Sprintf(" [%s]", fmtDuration(m.duration)))
		}
		return ""
	}

	total := ur.UsageTracker().Total()
	last, hasLast := ur.UsageTracker().Last()
	maxTok := ur.ModelMaxTokens()

	var line string
	switch {
	case hasLast && m.duration > 0:
		line = fmt.Sprintf(" last: ↑%s ↓%s · total: ↑%s ↓%s · limit: %s · %s",
			fmtTokens(last.InputTokens),
			fmtTokens(last.OutputTokens),
			fmtTokens(total.InputTokens),
			fmtTokens(total.OutputTokens),
			fmtTokens(maxTok),
			fmtDuration(m.duration),
		)
	case total.InputTokens+total.OutputTokens > 0:
		line = fmt.Sprintf(" tokens: ↑%s ↓%s · limit: %s",
			fmtTokens(total.InputTokens),
			fmtTokens(total.OutputTokens),
			fmtTokens(maxTok),
		)
	default:
		return ""
	}

	return statusStyle.Render(line)
}
