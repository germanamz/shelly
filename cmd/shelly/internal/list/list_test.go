package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	m := New("panel1", true)
	assert.Equal(t, "panel1", m.panelID)
	assert.True(t, m.selectable)
	assert.Empty(t, m.Items())
}

func TestSetItems_Basic(t *testing.T) {
	m := New("p1", false)
	items := []Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	}
	m.SetItems(items)
	assert.Len(t, m.Items(), 2)
}

func TestSetItems_SelectablePreservesCursor(t *testing.T) {
	m := New("p1", true)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	m.MoveDown() // cursor on "b"
	m.MoveDown() // cursor on "c"
	assert.Equal(t, 2, m.Cursor())

	// Update items — "c" still exists at index 1.
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "c", Label: "Charlie"},
	})
	assert.Equal(t, 1, m.Cursor()) // cursor followed "c" to index 1
}

func TestSetItems_SelectableCursorClampsOnShrink(t *testing.T) {
	m := New("p1", true)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	m.MoveDown()
	m.MoveDown() // cursor on "c"

	// Shrink to 1 item — cursor clamps.
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
	})
	assert.Equal(t, 0, m.Cursor())
}

func TestMoveUp_Selectable(t *testing.T) {
	m := New("p1", true)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	m.MoveDown()
	assert.Equal(t, 1, m.Cursor())
	m.MoveUp()
	assert.Equal(t, 0, m.Cursor())
	m.MoveUp() // should not go below 0
	assert.Equal(t, 0, m.Cursor())
}

func TestMoveDown_Selectable(t *testing.T) {
	m := New("p1", true)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	m.MoveDown()
	assert.Equal(t, 1, m.Cursor())
	m.MoveDown() // should not exceed last item
	assert.Equal(t, 1, m.Cursor())
}

func TestMoveDown_ReadOnly_Scrolls(t *testing.T) {
	m := New("p1", false)
	m.SetSize(40, 2)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	assert.Equal(t, 0, m.scrollTop)
	m.MoveDown()
	assert.Equal(t, 1, m.scrollTop)
	m.MoveDown() // can't scroll past max
	assert.Equal(t, 1, m.scrollTop)
}

func TestMoveUp_ReadOnly_Scrolls(t *testing.T) {
	m := New("p1", false)
	m.SetSize(40, 2)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	m.MoveDown() // scrollTop = 1
	m.MoveUp()
	assert.Equal(t, 0, m.scrollTop)
	m.MoveUp() // can't go below 0
	assert.Equal(t, 0, m.scrollTop)
}

func TestSelect_Selectable(t *testing.T) {
	m := New("p1", true)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	m.MoveDown()
	msg := m.Select()
	assert.NotNil(t, msg)
	assert.Equal(t, "p1", msg.PanelID)
	assert.Equal(t, "b", msg.ItemID)
}

func TestSelect_ReadOnly_ReturnsNil(t *testing.T) {
	m := New("p1", false)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
	})
	assert.Nil(t, m.Select())
}

func TestSelect_EmptyList_ReturnsNil(t *testing.T) {
	m := New("p1", true)
	assert.Nil(t, m.Select())
}

func TestRenderLine_Basic(t *testing.T) {
	m := New("p1", false)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha", Status: StatusPending},
	})
	line := m.RenderLine(0)
	assert.Contains(t, line, "Alpha")
	assert.Contains(t, line, "○") // pending icon
}

func TestRenderLine_WithDetail(t *testing.T) {
	m := New("p1", false)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha", Detail: "extra info"},
	})
	line := m.RenderLine(0)
	assert.Contains(t, line, "Alpha")
	assert.Contains(t, line, "extra info")
}

func TestRenderLine_WithIndent(t *testing.T) {
	m := New("p1", false)
	m.SetItems([]Item{
		{ID: "a", Label: "Root", Indent: 0},
		{ID: "b", Label: "Child", Indent: 1},
		{ID: "c", Label: "Grandchild", Indent: 2},
	})
	root := m.RenderLine(0)
	child := m.RenderLine(1)
	grandchild := m.RenderLine(2)

	// Each level has 2 more spaces of indent.
	assert.Less(t, leadingSpaces(root), leadingSpaces(child))
	assert.Less(t, leadingSpaces(child), leadingSpaces(grandchild))
}

func TestRenderLine_SelectableCursorHighlight(t *testing.T) {
	m := New("p1", true)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	// Cursor is at 0 — first line gets highlight prefix.
	focused := m.RenderLine(0)
	unfocused := m.RenderLine(1)
	assert.Contains(t, focused, ">")
	assert.NotContains(t, unfocused, ">")
}

func TestRenderLine_OutOfBounds(t *testing.T) {
	m := New("p1", false)
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	assert.Empty(t, m.RenderLine(-1))
	assert.Empty(t, m.RenderLine(5))
}

func TestRenderLine_StatusIcons(t *testing.T) {
	m := New("p1", false)
	m.SetItems([]Item{
		{ID: "a", Label: "Pending", Status: StatusPending},
		{ID: "b", Label: "Running", Status: StatusRunning},
		{ID: "c", Label: "Done", Status: StatusDone},
		{ID: "d", Label: "Failed", Status: StatusFailed},
	})
	assert.Contains(t, m.RenderLine(0), "○")
	// Running uses spinner frame — just check it renders.
	assert.Contains(t, m.RenderLine(1), "Running")
	assert.Contains(t, m.RenderLine(2), "✓")
	assert.Contains(t, m.RenderLine(3), "✗")
}

func TestView_EmptyShowsNoItems(t *testing.T) {
	m := New("p1", false)
	m.SetSize(40, 5)
	view := m.View()
	assert.Contains(t, view, "No items")
}

func TestView_RendersVisibleItems(t *testing.T) {
	m := New("p1", false)
	m.SetSize(40, 2)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	view := m.View()
	assert.Contains(t, view, "Alpha")
	assert.Contains(t, view, "Beta")
	assert.NotContains(t, view, "Charlie")
}

func TestView_ScrollShowsLaterItems(t *testing.T) {
	m := New("p1", false)
	m.SetSize(40, 2)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	m.MoveDown() // scroll
	view := m.View()
	assert.NotContains(t, view, "Alpha")
	assert.Contains(t, view, "Beta")
	assert.Contains(t, view, "Charlie")
}

func TestView_NoHeightShowsAll(t *testing.T) {
	m := New("p1", false)
	m.SetSize(40, 0) // height 0 means show all
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	view := m.View()
	assert.Contains(t, view, "Alpha")
	assert.Contains(t, view, "Beta")
	assert.Contains(t, view, "Charlie")
}

func TestScrollToCursor(t *testing.T) {
	m := New("p1", true)
	m.SetSize(40, 2)
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
		{ID: "d", Label: "Delta"},
	})
	// Move cursor down past visible window.
	m.MoveDown()
	m.MoveDown()
	m.MoveDown() // cursor at 3
	assert.Equal(t, 3, m.Cursor())
	// scrollTop should have adjusted to show cursor.
	assert.Equal(t, 2, m.scrollTop)
}

func TestAdvanceSpinner(t *testing.T) {
	m := New("p1", false)
	assert.Equal(t, 0, m.spinnerIdx)
	m.AdvanceSpinner()
	assert.Equal(t, 1, m.spinnerIdx)
	m.AdvanceSpinner()
	assert.Equal(t, 2, m.spinnerIdx)
}

// leadingSpaces counts leading space characters in a string (ignoring ANSI codes).
func leadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r == ' ' {
			count++
		} else {
			break
		}
	}
	return count
}
