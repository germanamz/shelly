package main

import (
	"fmt"
	"time"

	"github.com/germanamz/shelly/pkg/modeladapter"
)

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
			return statusStyle.Render(fmt.Sprintf(" %s total time", fmtDuration(m.duration)))
		}
		return ""
	}

	total := ur.UsageTracker().Total()
	last, hasLast := ur.UsageTracker().Last()
	totalTok := total.InputTokens + total.OutputTokens

	var line string
	switch {
	case hasLast && m.duration > 0:
		lastTok := last.InputTokens + last.OutputTokens
		line = fmt.Sprintf(" %s total, %s tokens last message, %s total time",
			fmtTokens(totalTok),
			fmtTokens(lastTok),
			fmtDuration(m.duration),
		)
	case totalTok > 0:
		line = fmt.Sprintf(" %s total tokens", fmtTokens(totalTok))
	default:
		return ""
	}

	return statusStyle.Render(line)
}
