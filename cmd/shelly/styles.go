package main

import "github.com/charmbracelet/lipgloss"

// Centralized style definitions for the TUI.
var (
	// User message styles.
	userPrefixStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")) // blue
	userBlockStyle  = lipgloss.NewStyle().PaddingLeft(1)

	// Thinking / reasoning styles.
	thinkingTextStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // gray
	thinkingFooterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true) // dim

	// Tool call styles.
	toolNameStyle   = lipgloss.NewStyle().Bold(true)                      // bold
	toolResultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // dim gray
	toolErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red

	// Sub-agent styles.
	subAgentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta

	// Agent answer styles.
	answerPrefixStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // cyan
	answerBlockStyle  = lipgloss.NewStyle().PaddingLeft(1)

	// Spinner / animation styles.
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta

	// General utility styles.
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray/dim
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray

	// Error block style.
	errorBlockStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("1"))
)

// Tree-drawing characters for hierarchical display.
const (
	treeCorner = "└ "
	treePipe   = "│ "
)
