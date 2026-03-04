package basetextarea

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// Model wraps textarea.Model with shared setup and auto-grow behavior.
type Model struct {
	TA        textarea.Model
	MinHeight int
	MaxHeight int
}

// New creates a new auto-growing textarea with shared defaults:
// no prompt, no line numbers, no char limit, cleared styles.
func New(placeholder string, minHeight, maxHeight int) Model {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.ShowLineNumbers = false
	ta.SetHeight(minHeight)
	ta.Prompt = ""
	ta.CharLimit = 0
	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Blurred.CursorLine = lipgloss.NewStyle()
	s.Focused.Prompt = lipgloss.NewStyle()
	s.Blurred.Prompt = lipgloss.NewStyle()
	ta.SetStyles(s)

	return Model{
		TA:        ta,
		MinHeight: minHeight,
		MaxHeight: maxHeight,
	}
}

// Update forwards the message to the underlying textarea and auto-grows the
// height based on visual line count. This implements the pre-set-max-height,
// update, then shrink-to-content pattern.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	m.TA.SetHeight(m.MaxHeight)

	var cmd tea.Cmd
	m.TA, cmd = m.TA.Update(msg)

	lines := m.VisualLineCount()
	h := min(max(lines, m.MinHeight), m.MaxHeight)
	m.TA.SetHeight(h)

	return m, cmd
}

// Value returns the current text.
func (m Model) Value() string {
	return m.TA.Value()
}

// SetValue sets the textarea text.
func (m *Model) SetValue(s string) {
	m.TA.SetValue(s)
}

// Reset clears the textarea and resets height to MinHeight.
func (m *Model) Reset() {
	m.TA.Reset()
	m.TA.SetHeight(m.MinHeight)
}

// Focus gives the textarea focus.
func (m *Model) Focus() tea.Cmd {
	return m.TA.Focus()
}

// Blur removes focus from the textarea.
func (m *Model) Blur() {
	m.TA.Blur()
}

// View renders the textarea.
func (m Model) View() string {
	return m.TA.View()
}

// SetWidth sets the textarea width.
func (m *Model) SetWidth(w int) {
	m.TA.SetWidth(w)
}

// SetHeight sets the textarea height directly.
func (m *Model) SetHeight(h int) {
	m.TA.SetHeight(h)
}

// VisualLineCount returns the number of visual lines the current text occupies,
// accounting for both hard newlines and soft wraps at the textarea width.
func (m Model) VisualLineCount() int {
	text := m.TA.Value()
	if text == "" {
		return 1
	}

	// Single hard-line (most common case): use the textarea's own wrap count
	// via LineInfo().Height to stay perfectly in sync with its rendering.
	if m.TA.LineCount() == 1 {
		return m.TA.LineInfo().Height
	}

	// Multi-line fallback: sum wrap counts per hard line.
	width := max(m.TA.Width(), 1)

	total := 0
	for line := range strings.SplitSeq(text, "\n") {
		total += WordWrapLineCount(line, width)
	}

	return total
}

// WordWrapLineCount returns the number of visual lines a single hard line
// occupies when word-wrapped at the given width. The algorithm mirrors the
// textarea's internal wrap() function from charm.land/bubbles/v2, using the
// same width-measurement libraries (uniseg for string widths, runewidth for
// single-rune widths).
func WordWrapLineCount(text string, width int) int {
	runes := []rune(text)
	if len(runes) == 0 {
		return 1
	}

	lines := 1
	lineWidth := 0 // visual width of the current line so far
	var wordRunes []rune
	spaces := 0

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			wordRunes = append(wordRunes, r)
		}

		if spaces > 0 {
			wordWidth := uniseg.StringWidth(string(wordRunes))
			if lineWidth+wordWidth+spaces > width {
				// Word doesn't fit on current line — wrap.
				lines++
				lineWidth = wordWidth + spaces
			} else {
				lineWidth += wordWidth + spaces
			}
			spaces = 0
			wordRunes = wordRunes[:0]
		} else {
			// Check if a single word exceeds the line width and must be broken.
			lastCharLen := runewidth.RuneWidth(wordRunes[len(wordRunes)-1])
			wordWidth := uniseg.StringWidth(string(wordRunes))
			if wordWidth+lastCharLen > width {
				if lineWidth > 0 {
					lines++
				}
				lineWidth = wordWidth
				wordRunes = wordRunes[:0]
			}
		}
	}

	// Handle remaining text after the loop — mirrors the textarea's >= boundary.
	wordWidth := uniseg.StringWidth(string(wordRunes))
	if lineWidth+wordWidth+spaces >= width {
		lines++
	}

	return lines
}
