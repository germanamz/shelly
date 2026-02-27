package main

import lipgloss "charm.land/lipgloss/v2"

// GitHub terminal light theme palette.
var (
	colorFg      = lipgloss.Color("#24292f") // primary foreground
	colorMuted   = lipgloss.Color("#656d76") // muted/dim text
	colorAccent  = lipgloss.Color("#0969da") // accent blue
	colorError   = lipgloss.Color("#cf222e") // error red
	colorSuccess = lipgloss.Color("#1a7f37") // success green
	colorWarning = lipgloss.Color("#9a6700") // warning amber
	colorMagenta = lipgloss.Color("#8250df") // purple/magenta
)

// Centralized style definitions for the TUI.
var (
	// User message styles.
	userPrefixStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	// Thinking / reasoning styles.
	thinkingTextStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// Tool call styles.
	toolNameStyle   = lipgloss.NewStyle().Bold(true)
	toolResultStyle = lipgloss.NewStyle().Foreground(colorMuted)
	toolErrorStyle  = lipgloss.NewStyle().Foreground(colorError)

	// Sub-agent styles.
	subAgentStyle = lipgloss.NewStyle().Foreground(colorMagenta)

	// Agent answer styles.
	answerPrefixStyle = lipgloss.NewStyle().Bold(true).Foreground(colorFg)

	// Spinner / animation styles.
	spinnerStyle = lipgloss.NewStyle().Foreground(colorMagenta)

	// General utility styles.
	dimStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	statusStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// Error block style.
	errorBlockStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(colorError)

	// Input styles.
	focusedBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent)
	disabledBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorMuted)

	// Picker styles.
	pickerBorder    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent)
	pickerCurStyle  = lipgloss.NewStyle().Bold(true).Underline(true)
	pickerDimStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	pickerHintStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// Ask prompt styles.
	askBorder     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorWarning)
	askTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	askOptStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	askSelStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	askTabActive  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	askTabDone    = lipgloss.NewStyle().Foreground(colorSuccess)
	askTabInact   = lipgloss.NewStyle().Foreground(colorMuted)
	askHintStyle  = lipgloss.NewStyle().Foreground(colorMuted)
)

// Tree-drawing characters for hierarchical display.
const (
	treeCorner = "└ "
	treePipe   = "│ "
	treeTee    = "├─ "
)
