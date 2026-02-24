package main

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/glamour"
	"github.com/joho/godotenv"
)

// thinkingMessages are displayed while the agent is processing.
var thinkingMessages = []string{
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

// spinnerFrames are braille characters for smooth animation.
var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// mdRenderer renders markdown to terminal-formatted output.
var mdRenderer *glamour.TermRenderer

func initMarkdownRenderer(width int) {
	if width <= 0 {
		width = 100
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return
	}
	mdRenderer = r
}

// renderMarkdown converts markdown text to terminal-formatted output.
func renderMarkdown(text string) string {
	if mdRenderer == nil {
		return text
	}
	out, err := mdRenderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}

// truncate returns s shortened to at most n runes, with "..." appended if
// truncated. Newlines are replaced with spaces for single-line display.
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// fmtTokens formats a token count for display, using k/M suffixes.
func fmtTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// fmtDuration formats a duration for display.
func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	min := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", min, sec)
}

// moveCursorWordLeft moves the cursor to the start of the previous word.
func moveCursorWordLeft(line []rune, cursor int) int {
	for cursor > 0 && unicode.IsSpace(line[cursor-1]) {
		cursor--
	}
	for cursor > 0 && !unicode.IsSpace(line[cursor-1]) {
		cursor--
	}
	return cursor
}

// moveCursorWordRight moves the cursor to the start of the next word.
func moveCursorWordRight(line []rune, cursor int) int {
	lineLen := len(line)
	if cursor < lineLen && !unicode.IsSpace(line[cursor]) {
		for cursor < lineLen && !unicode.IsSpace(line[cursor]) {
			cursor++
		}
	}
	for cursor < lineLen && unicode.IsSpace(line[cursor]) {
		cursor++
	}
	return cursor
}

// deleteWordBackward deletes the word backward from the cursor.
func deleteWordBackward(line []rune, cursor int) ([]rune, int) {
	newCursor := moveCursorWordLeft(line, cursor)
	return append(line[:newCursor], line[cursor:]...), newCursor
}

// loadDotEnv loads environment variables from path. Missing files are ignored.
func loadDotEnv(path string) error {
	err := godotenv.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// renderUserMessage formats a user message for the terminal scrollback,
// properly indenting continuation lines to align with the first line.
func renderUserMessage(text string) string {
	prefix := userPrefixStyle.Render("🧑 You > ")
	lines := strings.Split(text, "\n")
	if len(lines) <= 1 {
		return userBlockStyle.Render(prefix + text)
	}
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(lines[0])
	for _, line := range lines[1:] {
		sb.WriteString("\n  ")
		sb.WriteString(line)
	}
	return userBlockStyle.Render(sb.String())
}

// randomThinkingMessage returns a random thinking message.
func randomThinkingMessage() string {
	return thinkingMessages[rand.IntN(len(thinkingMessages))] //nolint:gosec // cosmetic randomness
}

// resolveConfigPath returns the config file to use. Priority:
// 1. Explicit --config flag (non-empty)
// 2. .shelly/config.yaml (if it exists)
// 3. shelly.yaml (legacy fallback)
func resolveConfigPath(explicit, shellyDirPath string) string {
	if explicit != "" {
		return explicit
	}

	shellyConfig := filepath.Join(shellyDirPath, "config.yaml")
	if _, err := os.Stat(shellyConfig); err == nil {
		return shellyConfig
	}

	return "shelly.yaml"
}
