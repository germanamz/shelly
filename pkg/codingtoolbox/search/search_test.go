package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autoApprove(_ context.Context, _ string, _ []string) (string, error) {
	return "yes", nil
}

func autoDeny(_ context.Context, _ string, _ []string) (string, error) {
	return "no", nil
}

func newTestSearch(t *testing.T, askFn AskFunc) (*Search, string) {
	t.Helper()

	dir := t.TempDir()
	permFile := filepath.Join(dir, ".shelly", "permissions.json")
	store, err := permissions.New(permFile)
	require.NoError(t, err)

	return New(store, askFn), dir
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return string(data)
}

func TestSearchContent(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\nfunc hello() {}\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: mustJSON(t, contentInput{Pattern: "func.*hello", Directory: dir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var matches []contentMatch
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &matches))
	assert.Len(t, matches, 1)
	assert.Equal(t, 2, matches[0].Line)
	assert.Contains(t, matches[0].Content, "func hello")
}

func TestSearchContent_InvalidRegex(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: mustJSON(t, contentInput{Pattern: "[invalid", Directory: dir}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "invalid pattern")
}

func TestSearchContent_MaxResults(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	// Create a file with many matching lines.
	var b strings.Builder
	for i := range 50 {
		fmt.Fprintf(&b, "match line %d\n", i)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "many.txt"), []byte(b.String()), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: mustJSON(t, contentInput{Pattern: "match", Directory: dir, MaxResults: 5}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var matches []contentMatch
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &matches))
	assert.Len(t, matches, 5)
}

func TestSearchContent_Denied(t *testing.T) {
	s, dir := newTestSearch(t, autoDeny)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: mustJSON(t, contentInput{Pattern: "x", Directory: dir}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestSearchContent_EmptyPattern(t *testing.T) {
	s, _ := newTestSearch(t, autoApprove)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: `{"pattern":"","directory":"/tmp"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "pattern is required")
}

func TestSearchFiles(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "helper.go"), []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_files",
		Arguments: mustJSON(t, filesInput{Pattern: "*.go", Directory: dir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var results []string
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &results))
	// Without **, only matches in top-level.
	assert.Contains(t, results, "main.go")
}

func TestSearchFiles_DoublestarGlob(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.go"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "deep", "c.go"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "deep", "d.txt"), []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_files",
		Arguments: mustJSON(t, filesInput{Pattern: "**/*.go", Directory: dir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var results []string
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &results))
	assert.Len(t, results, 3)
}

func TestSearchFiles_MaxResults(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	for i := range 20 {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.txt", i)), []byte("x"), 0o600))
	}

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_files",
		Arguments: mustJSON(t, filesInput{Pattern: "**/*.txt", Directory: dir, MaxResults: 3}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var results []string
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &results))
	assert.Len(t, results, 3)
}

func TestSearchFiles_Denied(t *testing.T) {
	s, dir := newTestSearch(t, autoDeny)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_files",
		Arguments: mustJSON(t, filesInput{Pattern: "*.go", Directory: dir}),
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "access denied")
}

func TestSearchFiles_PatternWithDirSeparator(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "main.go"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_files",
		Arguments: mustJSON(t, filesInput{Pattern: "sub/*.go", Directory: dir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var results []string
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &results))
	assert.Equal(t, []string{filepath.Join("sub", "main.go")}, results)
}

func TestSearchContent_WithContext(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	fileContent := "line1\nline2\nfunc target() {}\nline4\nline5\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ctx.go"), []byte(fileContent), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: mustJSON(t, contentInput{Pattern: "func target", Directory: dir, ContextLines: 1}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var matches []contentMatch
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &matches))
	require.Len(t, matches, 1)
	assert.Equal(t, 3, matches[0].Line)
	assert.Contains(t, matches[0].Content, "func target")
	assert.NotEmpty(t, matches[0].Context)
	assert.Contains(t, matches[0].Context, "line2")
	assert.Contains(t, matches[0].Context, "line4")
	assert.Contains(t, matches[0].Context, ">")
}

func TestSearchContent_ContextZero(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.go"), []byte("package main\nfunc foo() {}\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: mustJSON(t, contentInput{Pattern: "func foo", Directory: dir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var matches []contentMatch
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &matches))
	require.Len(t, matches, 1)
	assert.Empty(t, matches[0].Context) // no context when context_lines==0
}

func TestSearchContent_LongLines(t *testing.T) {
	s, dir := newTestSearch(t, autoApprove)
	tb := s.Tools()

	// Create a file with a line longer than the default 64KB scanner limit.
	longLine := strings.Repeat("a", 100_000) + "NEEDLE" + strings.Repeat("b", 100_000)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "long.txt"), []byte(longLine+"\n"), 0o600))

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_content",
		Arguments: mustJSON(t, contentInput{Pattern: "NEEDLE", Directory: dir}),
	})

	assert.False(t, tr.IsError, tr.Content)

	var matches []contentMatch
	require.NoError(t, json.Unmarshal([]byte(tr.Content), &matches))
	assert.Len(t, matches, 1)
	assert.Contains(t, matches[0].Content, "NEEDLE")
}

func TestSearchFiles_EmptyPattern(t *testing.T) {
	s, _ := newTestSearch(t, autoApprove)
	tb := s.Tools()

	tr := tb.Call(context.Background(), content.ToolCall{
		ID:        "tc1",
		Name:      "search_files",
		Arguments: `{"pattern":"","directory":"/tmp"}`,
	})

	assert.True(t, tr.IsError)
	assert.Contains(t, tr.Content, "pattern is required")
}
