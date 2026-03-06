package list

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// Status determines the icon rendered for a list item.
type Status int

const (
	StatusNone    Status = iota
	StatusPending        // renders: circle
	StatusRunning        // renders: spinner (animated)
	StatusDone           // renders: checkmark (dimmed)
	StatusFailed         // renders: X (error color)
)

// Item represents a single entry in the list.
type Item struct {
	ID     string
	Label  string // primary text
	Detail string // secondary text (right-aligned or after label)
	Status Status // determines icon
	Color  string // optional hex color for the label
	Indent int    // nesting depth (0 = root), rendered as indentation
}

// ListItemSelectedMsg is emitted when the user selects an item (selectable mode only).
type ListItemSelectedMsg struct {
	PanelID string
	ItemID  string
}

// ListDeactivatedMsg is emitted when the user presses Esc (selectable mode only).
type ListDeactivatedMsg struct{}

// ListSetItemsMsg replaces the item list.
type ListSetItemsMsg struct {
	Items []Item
}

// ListSetSizeMsg resizes the list viewport.
type ListSetSizeMsg struct {
	Width  int
	Height int
}

// Model is a generic vertical list that renders items with status icons,
// scrolling, and optional indentation. Supports read-only and selectable modes.
type Model struct {
	items      []Item
	width      int
	height     int // max visible rows before scrolling
	scrollTop  int
	spinnerIdx int
	selectable bool
	cursor     int    // focused item index (selectable mode only)
	panelID    string // for ListItemSelectedMsg
}

// New creates a new list Model.
func New(panelID string, selectable bool) Model {
	return Model{
		panelID:    panelID,
		selectable: selectable,
	}
}

// Items returns the current item list.
func (m Model) Items() []Item { return m.items }

// Cursor returns the current cursor position (only meaningful in selectable mode).
func (m Model) Cursor() int { return m.cursor }

// SetItems replaces the item list. In selectable mode, cursor position is
// preserved by matching the current item's ID; clamped if the list shrinks.
func (m *Model) SetItems(items []Item) {
	if m.selectable && len(m.items) > 0 && m.cursor < len(m.items) {
		currentID := m.items[m.cursor].ID
		m.items = items
		m.cursor = m.findByID(currentID)
	} else {
		m.items = items
	}
	m.clampScroll()
}

// SetSize updates the visible dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.clampScroll()
}

// AdvanceSpinner increments the spinner frame counter.
func (m *Model) AdvanceSpinner() {
	m.spinnerIdx++
}

// MoveUp moves the cursor up one item (selectable mode only).
// In read-only mode, scrolls up.
func (m *Model) MoveUp() {
	if m.selectable {
		if m.cursor > 0 {
			m.cursor--
		}
		m.scrollToCursor()
	} else if m.scrollTop > 0 {
		m.scrollTop--
	}
}

// MoveDown moves the cursor down one item (selectable mode only).
// In read-only mode, scrolls down.
func (m *Model) MoveDown() {
	if m.selectable {
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		m.scrollToCursor()
	} else {
		maxScroll := max(len(m.items)-m.height, 0)
		if m.scrollTop < maxScroll {
			m.scrollTop++
		}
	}
}

// Select returns a ListItemSelectedMsg for the currently focused item.
// Returns nil if the list is empty or not selectable.
func (m Model) Select() *ListItemSelectedMsg {
	if !m.selectable || len(m.items) == 0 {
		return nil
	}
	return &ListItemSelectedMsg{
		PanelID: m.panelID,
		ItemID:  m.items[m.cursor].ID,
	}
}

// RenderLine renders a single item line at the given index.
// In selectable mode, the focused line gets cursor highlight styling.
func (m Model) RenderLine(index int) string {
	if index < 0 || index >= len(m.items) {
		return ""
	}
	item := m.items[index]

	// Build indent.
	indent := strings.Repeat("  ", item.Indent)

	// Build status icon.
	icon := m.statusIcon(item.Status)

	// Build label.
	label := item.Label
	if item.Color != "" {
		label = lipgloss.NewStyle().Foreground(lipgloss.Color(item.Color)).Render(label)
	}

	// Build detail.
	detail := ""
	if item.Detail != "" {
		detail = " " + styles.DimStyle.Render(item.Detail)
	}

	line := fmt.Sprintf("%s%s %s%s", indent, icon, label, detail)

	// Cursor highlight in selectable mode.
	if m.selectable && index == m.cursor {
		line = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.ColorAccent).
			Render("> ") + line
	} else if m.selectable {
		line = "  " + line
	}

	return line
}

// View renders the visible portion of the list.
// Returns empty "No items" message when the list is empty.
func (m Model) View() string {
	if len(m.items) == 0 {
		return lipgloss.NewStyle().
			Foreground(styles.ColorMuted).
			Width(m.width).
			Align(lipgloss.Center).
			Render("No items")
	}

	visibleCount := min(m.height, len(m.items))
	if visibleCount <= 0 {
		visibleCount = len(m.items)
	}

	end := min(m.scrollTop+visibleCount, len(m.items))

	var lines []string
	for i := m.scrollTop; i < end; i++ {
		lines = append(lines, m.RenderLine(i))
	}

	return strings.Join(lines, "\n")
}

func (m Model) statusIcon(s Status) string {
	frame := format.SpinnerFrames[m.spinnerIdx%len(format.SpinnerFrames)]
	switch s {
	case StatusPending:
		return "○"
	case StatusRunning:
		return styles.SpinnerStyle.Render(frame)
	case StatusDone:
		return styles.DimStyle.Render("✓")
	case StatusFailed:
		return lipgloss.NewStyle().Foreground(styles.ColorError).Render("✗")
	default:
		return " "
	}
}

func (m *Model) scrollToCursor() {
	if m.height <= 0 {
		return
	}
	if m.cursor < m.scrollTop {
		m.scrollTop = m.cursor
	}
	if m.cursor >= m.scrollTop+m.height {
		m.scrollTop = m.cursor - m.height + 1
	}
}

func (m *Model) clampScroll() {
	if len(m.items) == 0 {
		m.scrollTop = 0
		m.cursor = 0
		return
	}
	if m.selectable && m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	maxScroll := max(len(m.items)-m.height, 0)
	if m.scrollTop > maxScroll {
		m.scrollTop = maxScroll
	}
}

func (m Model) findByID(id string) int {
	for i, item := range m.items {
		if item.ID == id {
			return i
		}
	}
	// ID not found — clamp to end.
	if len(m.items) == 0 {
		return 0
	}
	return len(m.items) - 1
}
