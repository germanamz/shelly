package notes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteNote_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "write_note",
		Arguments: `{"name":"my-note","content":"# Hello\nThis is a note."}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, `"my-note" saved`)

	data, err := os.ReadFile(filepath.Join(dir, "my-note.md")) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "# Hello\nThis is a note.", string(data))
}

func TestReadNote_ReturnsContent(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	// Write first.
	tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "write_note",
		Arguments: `{"name":"todo","content":"Buy milk"}`,
	})

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "read_note",
		Arguments: `{"name":"todo"}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "Buy milk", tr.Content)
}

func TestListNotes_ListsAllNotes(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "write_note",
		Arguments: `{"name":"alpha","content":"First note"}`,
	})
	tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "write_note",
		Arguments: `{"name":"beta","content":"Second note"}`,
	})

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc3",
		Name:      "list_notes",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "alpha")
	assert.Contains(t, tr.Content, "beta")
	assert.Contains(t, tr.Content, "First note")
	assert.Contains(t, tr.Content, "Second note")
}

func TestWriteNote_RejectsInvalidName(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tests := []struct {
		name string
		args string
	}{
		{"path traversal", `{"name":"../etc/passwd","content":"bad"}`},
		{"spaces", `{"name":"my note","content":"bad"}`},
		{"dots", `{"name":"note.secret","content":"bad"}`},
		{"slashes", `{"name":"sub/note","content":"bad"}`},
		{"empty name", `{"name":"","content":"bad"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := tb.Call(context.Background(), content.ToolCall{
				ID:        "tc1",
				Name:      "write_note",
				Arguments: tt.args,
			})
			assert.True(t, tr.IsError, "expected error for %s, got: %s", tt.name, tr.Content)
		})
	}
}

func TestWriteNote_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "write_note",
		Arguments: `{"name":"overwrite-me","content":"original"}`,
	})

	tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "write_note",
		Arguments: `{"name":"overwrite-me","content":"updated"}`,
	})

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc3",
		Name:      "read_note",
		Arguments: `{"name":"overwrite-me"}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "updated", tr.Content)
}

func TestReadNote_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "read_note",
		Arguments: `{"name":"nonexistent"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "not found")
}

func TestWriteNote_AutoCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "notes")
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "write_note",
		Arguments: `{"name":"deep","content":"created in nested dir"}`,
	})

	assert.False(t, tr.IsError, tr.Content)

	data, err := os.ReadFile(filepath.Join(dir, "deep.md")) //nolint:gosec // test reads from temp dir
	require.NoError(t, err)
	assert.Equal(t, "created in nested dir", string(data))
}

func TestListNotes_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "list_notes",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "No notes found.", tr.Content)
}

func TestListNotes_NonexistentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "list_notes",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Equal(t, "No notes found.", tr.Content)
}

func TestReadNote_RejectsInvalidName(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "read_note",
		Arguments: `{"name":"../etc/passwd"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "invalid name")
}

func TestWriteNote_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "write_note",
		Arguments: `not json`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "invalid input")
}

func TestReadNote_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "read_note",
		Arguments: `not json`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "invalid input")
}

func TestReadNote_EmptyName(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "read_note",
		Arguments: `{"name":""}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "name is required")
}

func TestListNotes_IgnoresNonMdFiles(t *testing.T) {
	dir := t.TempDir()

	// Write a non-md file directly.
	err := os.WriteFile(filepath.Join(dir, "not-a-note.txt"), []byte("ignored"), 0o600)
	require.NoError(t, err)

	s := New(dir)
	tb := s.Tools()

	// Write one real note.
	tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "write_note",
		Arguments: `{"name":"real","content":"I am real"}`,
	})

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc2",
		Name:      "list_notes",
		Arguments: `{}`,
	})

	assert.False(t, tr.IsError, tr.Content)
	assert.Contains(t, tr.Content, "real")
	assert.NotContains(t, tr.Content, "not-a-note")
}

func TestToolsRegistered(t *testing.T) {
	s := New(t.TempDir())
	tb := s.Tools()
	tools := tb.Tools()

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	assert.True(t, names["write_note"])
	assert.True(t, names["read_note"])
	assert.True(t, names["list_notes"])
	assert.Len(t, tools, 3)
}
