package panel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	p := New("test-panel", "Test Title")
	assert.Equal(t, "test-panel", p.PanelID())
	assert.Equal(t, "Test Title", p.title)
	assert.False(t, p.Active())
}

func TestView_InactiveReturnsEmpty(t *testing.T) {
	p := New("p1", "Title")
	assert.Empty(t, p.View("some content"))
}

func TestView_ActiveRendersContent(t *testing.T) {
	p := New("p1", "Title")
	p.SetActive(true)
	p.SetSize(40, 10)

	view := p.View("hello world")
	assert.Contains(t, view, "hello world")
	assert.Contains(t, view, "Title")
}

func TestView_EmptyContentShowsNoItems(t *testing.T) {
	p := New("p1", "Title")
	p.SetActive(true)
	p.SetSize(40, 10)

	view := p.View("")
	assert.Contains(t, view, "No items")
}

func TestView_WhitespaceContentShowsNoItems(t *testing.T) {
	p := New("p1", "Title")
	p.SetActive(true)
	p.SetSize(40, 10)

	view := p.View("   \n  ")
	assert.Contains(t, view, "No items")
}

func TestViewError_RendersErrorStyled(t *testing.T) {
	p := New("p1", "Title")
	p.SetActive(true)
	p.SetSize(40, 10)

	view := p.ViewError("something broke")
	assert.Contains(t, view, "something broke")
	assert.Contains(t, view, "Title")
}

func TestViewError_InactiveReturnsEmpty(t *testing.T) {
	p := New("p1", "Title")
	assert.Empty(t, p.ViewError("error"))
}

func TestSetSize(t *testing.T) {
	p := New("p1", "Title")
	p.SetSize(80, 12)
	assert.Equal(t, 80, p.Width())
	assert.Equal(t, 12, p.Height())
	assert.Equal(t, 10, p.ContentHeight())
}

func TestContentHeight_SmallHeight(t *testing.T) {
	p := New("p1", "Title")
	p.SetSize(40, 1)
	assert.Equal(t, 0, p.ContentHeight())

	p.SetSize(40, 0)
	assert.Equal(t, 0, p.ContentHeight())
}

func TestView_TruncatesContentToHeight(t *testing.T) {
	p := New("p1", "Title")
	p.SetActive(true)
	p.SetSize(40, 5) // content height = 3

	content := "line1\nline2\nline3\nline4\nline5"
	view := p.View(content)
	// Should contain first 3 lines but not line4/line5
	assert.Contains(t, view, "line1")
	assert.Contains(t, view, "line2")
	assert.Contains(t, view, "line3")
	assert.NotContains(t, view, "line4")
	assert.NotContains(t, view, "line5")
}

func TestView_HasBorder(t *testing.T) {
	p := New("p1", "Title")
	p.SetActive(true)
	p.SetSize(30, 5)

	view := p.View("content")
	lines := strings.Split(view, "\n")
	// Should have at least 3 lines (top border, content, bottom border)
	assert.GreaterOrEqual(t, len(lines), 3)
}
