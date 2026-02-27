package format

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// IsDarkBG is set once before bubbletea starts (in main.go) so that glamour
// never issues its own OSC 11 query while the program is running.
var IsDarkBG bool

// ThinkingMessages are displayed while the agent is processing.
var ThinkingMessages = []string{
	"Thinking...",
	"Pondering the cosmos...",
	"Consulting ancient scrolls...",
	"Brewing a response...",
	"Connecting synapses...",
	"Mining for wisdom...",
	"Summoning knowledge...",
	"Assembling words...",
	"Crunching tokens...",
	"Weaving thoughts...",
	"Channeling creativity...",
	"Exploring possibilities...",
	"Decoding the matrix...",
	"Warming up neurons...",
	"Traversing the knowledge graph...",
}

// SpinnerFrames are braille characters for smooth animation.
var SpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// mdRenderer renders markdown to terminal-formatted output.
var (
	mdRenderer      *glamour.TermRenderer
	mdRendererMu    sync.Mutex
	mdRendererWidth int
)

// InitMarkdownRenderer initializes the glamour renderer at the given width.
func InitMarkdownRenderer(width int) {
	if width <= 0 {
		width = 100
	}
	mdRendererMu.Lock()
	defer mdRendererMu.Unlock()
	if width == mdRendererWidth && mdRenderer != nil {
		return
	}
	// Use a fixed style based on the pre-detected background color.
	// glamour.WithAutoStyle() must NOT be used here — it queries the terminal
	// (OSC 11) which races with bubbletea's input handling and leaks escape
	// sequences into the textarea.
	style := glamourstyles.LightStyleConfig
	if IsDarkBG {
		style = glamourstyles.DarkStyleConfig
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return
	}
	mdRenderer = r
	mdRendererWidth = width
}

// RenderMarkdown converts markdown text to terminal-formatted output.
func RenderMarkdown(text string) string {
	mdRendererMu.Lock()
	r := mdRenderer
	mdRendererMu.Unlock()
	if r == nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}

// Truncate returns s shortened to at most n runes, with "..." appended if
// truncated. Newlines are replaced with spaces for single-line display.
func Truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// FmtTokens formats a token count for display, using k/M suffixes.
func FmtTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// FmtDuration formats a duration for display.
func FmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	min := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", min, sec)
}

// RenderUserMessage formats a user message for display.
func RenderUserMessage(text string) string {
	header := styles.UserPrefixStyle.Render("🧑‍💻 User")
	lines := strings.Split(text, "\n")
	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(" ")
	sb.WriteString(styles.TreeCorner)
	sb.WriteString(lines[0])
	for _, line := range lines[1:] {
		sb.WriteString("\n   ")
		sb.WriteString(line)
	}
	return sb.String()
}

// RandomThinkingMessage returns a random thinking message.
func RandomThinkingMessage() string {
	return ThinkingMessages[rand.IntN(len(ThinkingMessages))] //nolint:gosec // cosmetic randomness
}
