package menubar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	m := New()
	assert.False(t, m.Active())
	assert.False(t, m.Visible())
	assert.Empty(t, m.Items())
	assert.Equal(t, -1, m.Cursor())
}

func TestSetActive_MovesCursorToFirst(t *testing.T) {
	m := New()
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	m.SetActive(true)
	assert.True(t, m.Active())
	assert.Equal(t, 0, m.Cursor())
}

func TestSetActive_Deactivate_ResetsCursor(t *testing.T) {
	m := New()
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	m.SetActive(true)
	assert.Equal(t, 0, m.Cursor())
	m.SetActive(false)
	assert.Equal(t, -1, m.Cursor())
	assert.False(t, m.Active())
}

func TestSetActive_EmptyItems(t *testing.T) {
	m := New()
	m.SetActive(true)
	assert.True(t, m.Active())
	assert.Equal(t, -1, m.Cursor()) // no items to focus
}

func TestMoveLeft(t *testing.T) {
	m := New()
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	m.SetActive(true)
	m.MoveRight()
	m.MoveRight() // cursor at 2
	assert.Equal(t, 2, m.Cursor())
	m.MoveLeft()
	assert.Equal(t, 1, m.Cursor())
	m.MoveLeft()
	assert.Equal(t, 0, m.Cursor())
	m.MoveLeft() // can't go below 0
	assert.Equal(t, 0, m.Cursor())
}

func TestMoveRight(t *testing.T) {
	m := New()
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	m.SetActive(true)
	m.MoveRight()
	assert.Equal(t, 1, m.Cursor())
	m.MoveRight() // can't exceed last item
	assert.Equal(t, 1, m.Cursor())
}

func TestMoveLeft_Inactive_Noop(t *testing.T) {
	m := New()
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	m.MoveLeft()
	assert.Equal(t, -1, m.Cursor())
}

func TestMoveRight_Inactive_Noop(t *testing.T) {
	m := New()
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	m.MoveRight()
	assert.Equal(t, -1, m.Cursor())
}

func TestSelect_Active(t *testing.T) {
	m := New()
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	m.SetActive(true)
	m.MoveRight()
	msg := m.Select()
	assert.NotNil(t, msg)
	assert.Equal(t, "b", msg.ID)
}

func TestSelect_Inactive_ReturnsNil(t *testing.T) {
	m := New()
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	assert.Nil(t, m.Select())
}

func TestSelect_Empty_ReturnsNil(t *testing.T) {
	m := New()
	m.SetActive(true)
	assert.Nil(t, m.Select())
}

func TestSetItems_ClampsCursor(t *testing.T) {
	m := New()
	m.SetItems([]Item{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
		{ID: "c", Label: "Charlie"},
	})
	m.SetActive(true)
	m.MoveRight()
	m.MoveRight() // cursor at 2
	// Shrink to 1 item.
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	assert.Equal(t, 0, m.Cursor())
}

func TestSetItems_EmptyResetsCursor(t *testing.T) {
	m := New()
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	m.SetActive(true)
	assert.Equal(t, 0, m.Cursor())
	m.SetItems(nil)
	assert.Equal(t, -1, m.Cursor())
}

func TestAddOrUpdateItem_NewItem(t *testing.T) {
	m := New()
	m.AddOrUpdateItem(Item{ID: "a", Label: "Alpha"})
	assert.Len(t, m.Items(), 1)
	assert.Equal(t, "Alpha", m.Items()[0].Label)
}

func TestAddOrUpdateItem_UpdateExisting(t *testing.T) {
	m := New()
	m.AddOrUpdateItem(Item{ID: "a", Label: "Alpha", Badge: 1})
	m.AddOrUpdateItem(Item{ID: "a", Label: "Alpha", Badge: 5})
	assert.Len(t, m.Items(), 1)
	assert.Equal(t, 5, m.Items()[0].Badge)
}

func TestHeight_Hidden(t *testing.T) {
	m := New()
	assert.Equal(t, 0, m.Height())
}

func TestHeight_Visible(t *testing.T) {
	m := New()
	m.SetVisible(true)
	assert.Equal(t, 1, m.Height())
}

func TestView_NotVisible(t *testing.T) {
	m := New()
	m.SetItems([]Item{{ID: "a", Label: "Alpha"}})
	assert.Empty(t, m.View())
}

func TestView_VisibleNoItems(t *testing.T) {
	m := New()
	m.SetVisible(true)
	assert.Empty(t, m.View())
}

func TestView_RendersItems(t *testing.T) {
	m := New()
	m.SetVisible(true)
	m.SetWidth(80)
	m.SetItems([]Item{
		{ID: "a", Label: "Subagents", Badge: 3},
		{ID: "b", Label: "Tasks", Badge: 2},
	})
	view := m.View()
	assert.Contains(t, view, "Subagents")
	assert.Contains(t, view, "Tasks")
	assert.Contains(t, view, "(3)")
	assert.Contains(t, view, "(2)")
	assert.Contains(t, view, "│") // divider
}

func TestView_FocusedItemHighlighted(t *testing.T) {
	m := New()
	m.SetVisible(true)
	m.SetWidth(80)
	m.SetItems([]Item{
		{ID: "a", Label: "Subagents", Badge: 3},
		{ID: "b", Label: "Tasks", Badge: 2},
	})
	m.SetActive(true)
	// Cursor is at 0 — "Subagents" should be highlighted.
	view := m.View()
	assert.Contains(t, view, "Subagents")
	assert.Contains(t, view, "Tasks")
}

func TestView_ZeroBadge_Dimmed(t *testing.T) {
	m := New()
	m.SetVisible(true)
	m.SetWidth(80)
	m.SetItems([]Item{
		{ID: "a", Label: "Subagents", Badge: 0},
	})
	view := m.View()
	// Badge 0 means no count shown but item is dimmed.
	assert.Contains(t, view, "Subagents")
	assert.NotContains(t, view, "(0)")
}

func TestView_PositiveBadge_ShowsCount(t *testing.T) {
	m := New()
	m.SetVisible(true)
	m.SetWidth(80)
	m.SetItems([]Item{
		{ID: "a", Label: "Tasks", Badge: 7},
	})
	view := m.View()
	assert.Contains(t, view, "(7)")
}

func TestItoa(t *testing.T) {
	assert.Equal(t, "0", itoa(0))
	assert.Equal(t, "1", itoa(1))
	assert.Equal(t, "42", itoa(42))
	assert.Equal(t, "100", itoa(100))
}
