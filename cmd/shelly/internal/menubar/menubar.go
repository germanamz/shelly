package menubar

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
)

// Item represents a single entry in the menu bar.
type Item struct {
	ID    string
	Label string // display text, e.g. "Subagents"
	Badge int    // optional count badge (0 = hidden)
}

// MenuItemSelectedMsg is emitted when the user activates an item.
type MenuItemSelectedMsg struct {
	ID string
}

// MenuDeactivatedMsg is emitted when the user presses Esc in the menu bar.
type MenuDeactivatedMsg struct{}

// MenuSetItemsMsg replaces the item list.
type MenuSetItemsMsg struct {
	Items []Item
}

// Model is a horizontal menu bar component following bubbletea conventions.
// It renders as a single line of items separated by dividers.
type Model struct {
	items   []Item
	cursor  int  // which item is focused (-1 = none/inactive)
	active  bool // whether the menu bar has keyboard focus
	visible bool // whether the menu bar is rendered at all
	width   int
}

// New creates a new menu bar Model.
func New() Model {
	return Model{
		cursor: -1,
	}
}

// Active returns whether the menu bar currently has keyboard focus.
func (m Model) Active() bool { return m.active }

// Visible returns whether the menu bar is rendered.
func (m Model) Visible() bool { return m.visible }

// Items returns the current item list.
func (m Model) Items() []Item { return m.items }

// Cursor returns the current cursor position (-1 if inactive).
func (m Model) Cursor() int { return m.cursor }

// SetVisible sets whether the menu bar is rendered.
func (m *Model) SetVisible(v bool) { m.visible = v }

// SetActive activates or deactivates keyboard focus.
// When activating, the cursor moves to the first item if not already set.
func (m *Model) SetActive(active bool) {
	m.active = active
	if active && m.cursor < 0 && len(m.items) > 0 {
		m.cursor = 0
	}
	if !active {
		m.cursor = -1
	}
}

// SetWidth updates the available width for rendering.
func (m *Model) SetWidth(w int) { m.width = w }

// SetItems replaces the item list. Cursor is clamped if the list shrinks.
func (m *Model) SetItems(items []Item) {
	m.items = items
	if m.cursor >= len(m.items) {
		if len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		} else {
			m.cursor = -1
		}
	}
}

// AddOrUpdateItem adds a new item or updates an existing one by ID.
func (m *Model) AddOrUpdateItem(item Item) {
	for i, existing := range m.items {
		if existing.ID == item.ID {
			m.items[i] = item
			return
		}
	}
	m.items = append(m.items, item)
}

// MoveLeft moves the cursor left.
func (m *Model) MoveLeft() {
	if !m.active || len(m.items) == 0 {
		return
	}
	if m.cursor > 0 {
		m.cursor--
	}
}

// MoveRight moves the cursor right.
func (m *Model) MoveRight() {
	if !m.active || len(m.items) == 0 {
		return
	}
	if m.cursor < len(m.items)-1 {
		m.cursor++
	}
}

// Select returns a MenuItemSelectedMsg for the currently focused item.
// Returns nil if the menu is inactive or empty.
func (m Model) Select() *MenuItemSelectedMsg {
	if !m.active || m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	return &MenuItemSelectedMsg{ID: m.items[m.cursor].ID}
}

// Height returns the number of lines the menu bar occupies (0 when hidden, 1 when visible).
func (m Model) Height() int {
	if !m.visible {
		return 0
	}
	return 1
}

// View renders the menu bar as a single horizontal line.
// Returns an empty string when not visible.
func (m Model) View() string {
	if !m.visible || len(m.items) == 0 {
		return ""
	}

	divider := styles.DimStyle.Render("  │  ")

	var parts []string
	for i, item := range m.items {
		parts = append(parts, m.renderItem(i, item))
	}

	line := strings.Join(parts, divider)

	return lipgloss.NewStyle().Width(m.width).Render(line)
}

func (m Model) renderItem(index int, item Item) string {
	label := item.Label
	if item.Badge > 0 {
		label += badgeText(item.Badge)
	}

	if m.active && index == m.cursor {
		// Focused item: accent color, bold.
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.ColorAccent).
			Render(label)
	}

	if item.Badge == 0 {
		// Dimmed when badge is 0 (no active items in this category).
		return styles.DimStyle.Render(label)
	}

	// Normal (visible, not focused).
	return lipgloss.NewStyle().
		Foreground(styles.ColorFg).
		Render(label)
}

func badgeText(count int) string {
	return lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Render(" (" + itoa(count) + ")")
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
