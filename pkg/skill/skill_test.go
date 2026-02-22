package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "code-review.md")
	require.NoError(t, os.WriteFile(path, []byte("# Code Review\n1. Check tests\n"), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "code-review", s.Name)
	assert.Equal(t, "# Code Review\n1. Check tests\n", s.Content)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/file.md")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill: load")
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("Alpha content"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "beta.md"), []byte("Beta content"), 0o600))
	// Non-.md file should be skipped.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0o600))
	// Subdirectory should be skipped.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o750))

	skills, err := LoadDir(dir)

	require.NoError(t, err)
	require.Len(t, skills, 2)
	assert.Equal(t, "alpha", skills[0].Name)
	assert.Equal(t, "Alpha content", skills[0].Content)
	assert.Equal(t, "beta", skills[1].Name)
	assert.Equal(t, "Beta content", skills[1].Content)
}

func TestLoadDirEmpty(t *testing.T) {
	dir := t.TempDir()

	skills, err := LoadDir(dir)

	require.NoError(t, err)
	assert.Empty(t, skills)
}

func TestLoadDirMissing(t *testing.T) {
	_, err := LoadDir("/nonexistent/dir")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill: load dir")
}

// --- Frontmatter tests ---

func TestLoadFrontmatterNameAndDescription(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: code-review\ndescription: Teaches code review best practices\n---\nFull body here.\n"
	path := filepath.Join(dir, "review.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "code-review", s.Name)
	assert.Equal(t, "Teaches code review best practices", s.Description)
	assert.Equal(t, "Full body here.\n", s.Content)
	assert.True(t, s.HasDescription())
}

func TestLoadFrontmatterNameOnly(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: my-skill\n---\nBody text.\n"
	path := filepath.Join(dir, "other.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "my-skill", s.Name)
	assert.Empty(t, s.Description)
	assert.False(t, s.HasDescription())
	assert.Equal(t, "Body text.\n", s.Content)
}

func TestLoadFrontmatterDescriptionOnly(t *testing.T) {
	dir := t.TempDir()
	content := "---\ndescription: A useful skill\n---\nBody.\n"
	path := filepath.Join(dir, "useful.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "useful", s.Name) // name from filename
	assert.Equal(t, "A useful skill", s.Description)
	assert.Equal(t, "Body.\n", s.Content)
}

func TestLoadNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := "# Just Markdown\nNo frontmatter here.\n"
	path := filepath.Join(dir, "plain.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "plain", s.Name)
	assert.Empty(t, s.Description)
	assert.False(t, s.HasDescription())
	assert.Equal(t, content, s.Content)
}

func TestLoadFrontmatterNoClosingDelimiter(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: broken\nSome text without closing.\n"
	path := filepath.Join(dir, "broken.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "broken", s.Name) // from filename, not frontmatter
	assert.Empty(t, s.Description)
	assert.Equal(t, content, s.Content) // full content preserved
}

func TestLoadFrontmatterEmptyBody(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: empty-body\ndescription: Has no body\n---\n"
	path := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "empty-body", s.Name)
	assert.Equal(t, "Has no body", s.Description)
	assert.Empty(t, s.Content)
}

func TestLoadFrontmatterInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	content := "---\n: invalid: yaml: [broken\n---\nBody.\n"
	path := filepath.Join(dir, "invalid.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	_, err := Load(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid frontmatter")
}

func TestLoadDirMixedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "alpha.md"),
		[]byte("---\ndescription: Alpha desc\n---\nAlpha body"),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "beta.md"),
		[]byte("Beta plain content"),
		0o600,
	))

	skills, err := LoadDir(dir)

	require.NoError(t, err)
	require.Len(t, skills, 2)

	assert.Equal(t, "alpha", skills[0].Name)
	assert.Equal(t, "Alpha desc", skills[0].Description)
	assert.Equal(t, "Alpha body", skills[0].Content)

	assert.Equal(t, "beta", skills[1].Name)
	assert.Empty(t, skills[1].Description)
	assert.Equal(t, "Beta plain content", skills[1].Content)
}

func TestLoadFrontmatterWindowsLineEndings(t *testing.T) {
	dir := t.TempDir()
	content := "---\r\nname: win-skill\r\ndescription: Windows style\r\n---\r\nBody with CRLF.\r\n"
	path := filepath.Join(dir, "win.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "win-skill", s.Name)
	assert.Equal(t, "Windows style", s.Description)
	assert.Equal(t, "Body with CRLF.\n", s.Content)
}
