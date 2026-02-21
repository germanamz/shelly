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
