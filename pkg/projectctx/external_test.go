package projectctx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadExternal_ClaudeMD(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("# Claude\nProject rules."), 0o600))

	result := LoadExternal(tmp)

	assert.Equal(t, "# Claude\nProject rules.", result)
}

func TestLoadExternal_CursorRules(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".cursorrules"), []byte("Always use tabs."), 0o600))

	result := LoadExternal(tmp)

	assert.Equal(t, "Always use tabs.", result)
}

func TestLoadExternal_CursorMDC(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, ".cursor", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "style.mdc"), []byte("Use gofumpt."), 0o600))

	result := LoadExternal(tmp)

	assert.Equal(t, "Use gofumpt.", result)
}

func TestLoadExternal_CursorMDC_WithFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, ".cursor", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o750))

	content := "---\ndescription: Style guide\nglobs: \"*.go\"\n---\nUse gofumpt for formatting."
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "style.mdc"), []byte(content), 0o600))

	result := LoadExternal(tmp)

	assert.Equal(t, "Use gofumpt for formatting.", result)
}

func TestLoadExternal_CursorMDC_SortedAlphabetically(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, ".cursor", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "b-testing.mdc"), []byte("Use testify."), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "a-style.mdc"), []byte("Use gofumpt."), 0o600))

	result := LoadExternal(tmp)

	assert.Equal(t, "Use gofumpt.\n\nUse testify.", result)
}

func TestLoadExternal_AllSources(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, ".cursor", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("Claude rules"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".cursorrules"), []byte("Cursor rules"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "extra.mdc"), []byte("MDC rules"), 0o600))

	result := LoadExternal(tmp)

	assert.Equal(t, "Claude rules\n\nCursor rules\n\nMDC rules", result)
}

func TestLoadExternal_EmptyFiles(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("  \n\n  "), 0o600))

	result := LoadExternal(tmp)

	assert.Empty(t, result)
}

func TestLoadExternal_NonexistentPath(t *testing.T) {
	result := LoadExternal("/nonexistent/path")

	assert.Empty(t, result)
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "with frontmatter",
			raw:  "---\ntitle: Test\n---\nBody content.",
			want: "Body content.",
		},
		{
			name: "no frontmatter",
			raw:  "Just plain content.",
			want: "Just plain content.",
		},
		{
			name: "unclosed frontmatter",
			raw:  "---\ntitle: Test\nNo closing delimiter.",
			want: "---\ntitle: Test\nNo closing delimiter.",
		},
		{
			name: "empty body after frontmatter",
			raw:  "---\ntitle: Test\n---",
			want: "",
		},
		{
			name: "frontmatter with blank lines in body",
			raw:  "---\nkey: val\n---\n\nParagraph one.\n\nParagraph two.",
			want: "Paragraph one.\n\nParagraph two.",
		},
		{
			name: "triple dashes in body",
			raw:  "---\nkey: val\n---\nContent with --- in it.",
			want: "Content with --- in it.",
		},
		{
			name: "windows line endings",
			raw:  "---\r\ntitle: Test\r\n---\r\nBody content.",
			want: "Body content.",
		},
		{
			name: "windows line endings no frontmatter",
			raw:  "Just plain\r\ncontent.",
			want: "Just plain\ncontent.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripFrontmatter(tt.raw))
		})
	}
}
