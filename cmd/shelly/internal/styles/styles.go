package styles

import lipgloss "charm.land/lipgloss/v2"

// GitHub terminal light theme palette.
var (
	ColorFg      = lipgloss.Color("#24292f") // primary foreground
	ColorMuted   = lipgloss.Color("#656d76") // muted/dim text
	ColorAccent  = lipgloss.Color("#0969da") // accent blue
	ColorError   = lipgloss.Color("#cf222e") // error red
	ColorSuccess = lipgloss.Color("#1a7f37") // success green
	ColorWarning = lipgloss.Color("#9a6700") // warning amber
	ColorMagenta = lipgloss.Color("#8250df") // purple/magenta
)

// Centralized style definitions for the TUI.
var (
	// User message styles.
	UserPrefixStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)

	// Thinking / reasoning styles.
	ThinkingTextStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	// Tool call styles.
	ToolNameStyle   = lipgloss.NewStyle().Bold(true)
	ToolResultStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	ToolErrorStyle  = lipgloss.NewStyle().Foreground(ColorError)

	// Sub-agent styles.
	SubAgentStyle = lipgloss.NewStyle().Foreground(ColorMagenta)

	// Agent answer styles.
	AnswerPrefixStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorFg)

	// Spinner / animation styles.
	SpinnerStyle = lipgloss.NewStyle().Foreground(ColorMagenta)

	// General utility styles.
	DimStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
	StatusStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	// Error block style.
	ErrorBlockStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(ColorError)

	// Input styles.
	FocusedBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorAccent)
	DisabledBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorMuted)

	// Picker styles.
	PickerBorder    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorAccent)
	PickerCurStyle  = lipgloss.NewStyle().Bold(true).Underline(true)
	PickerDimStyle  = lipgloss.NewStyle().Foreground(ColorMuted)
	PickerHintStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	// Ask prompt styles.
	AskBorder     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorWarning)
	AskTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorFg)
	AskOptStyle   = lipgloss.NewStyle().Foreground(ColorMuted)
	AskSelStyle   = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	AskTabActive  = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	AskTabDone    = lipgloss.NewStyle().Foreground(ColorSuccess)
	AskTabInact   = lipgloss.NewStyle().Foreground(ColorMuted)
	AskHintStyle  = lipgloss.NewStyle().Foreground(ColorMuted)
)

// Tree-drawing characters for hierarchical display.
const (
	TreeCorner = "└ "
	TreePipe   = "│ "
	TreeTee    = "├─ "
)
