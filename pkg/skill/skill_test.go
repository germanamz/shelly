package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSkillDir creates a skill folder with a SKILL.md file and returns the folder path.
func writeSkillDir(t *testing.T, parent, name, content string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600))
	return dir
}

func TestLoad(t *testing.T) {
	parent := t.TempDir()
	dir := writeSkillDir(t, parent, "code-review", "# Code Review\n1. Check tests\n")

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "code-review", s.Name)
	assert.Equal(t, "# Code Review\n1. Check tests\n", s.Content)
}

func TestLoadMissingFolder(t *testing.T) {
	_, err := Load("/nonexistent/folder")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill: load")
}

func TestLoadMissingSKILLmd(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "no-skill")
	require.NoError(t, os.MkdirAll(folder, 0o750))

	_, err := Load(folder)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill: load")
}

func TestLoadSetsAbsoluteDir(t *testing.T) {
	parent := t.TempDir()
	writeSkillDir(t, parent, "my-skill", "Content")

	// Use a relative-looking path by changing to the parent and using a relative ref.
	// Since TempDir returns an absolute path, we construct a path with ".." to test resolution.
	absParent, err := filepath.Abs(parent)
	require.NoError(t, err)

	relPath := filepath.Join(absParent, ".", "my-skill")
	s, loadErr := Load(relPath)

	require.NoError(t, loadErr)
	assert.True(t, filepath.IsAbs(s.Dir))
	assert.Equal(t, filepath.Join(absParent, "my-skill"), s.Dir)
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	writeSkillDir(t, dir, "alpha", "Alpha content")
	writeSkillDir(t, dir, "beta", "Beta content")
	// Non-directory file should be skipped.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0o600))

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

func TestLoadDirSkipsNonSkillDirs(t *testing.T) {
	dir := t.TempDir()
	writeSkillDir(t, dir, "valid", "Valid content")
	// Directory without SKILL.md should be silently skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "no-skill-md"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "no-skill-md", "README.md"), []byte("not a skill"), 0o600))

	skills, err := LoadDir(dir)

	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "valid", skills[0].Name)
}

func TestLoadDirWithSupplementaryFiles(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeSkillDir(t, dir, "deploy", "Deploy steps")
	// Add supplementary files and subdirectories.
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "checklist.md"), []byte("checklist"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "deploy.sh"), []byte("#!/bin/bash"), 0o600))

	skills, err := LoadDir(dir)

	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "deploy", skills[0].Name)
	assert.Equal(t, "Deploy steps", skills[0].Content)
}

// --- Frontmatter tests ---

func TestLoadFrontmatterNameAndDescription(t *testing.T) {
	parent := t.TempDir()
	content := "---\nname: code-review\ndescription: Teaches code review best practices\n---\nFull body here.\n"
	dir := writeSkillDir(t, parent, "review", content)

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "code-review", s.Name)
	assert.Equal(t, "Teaches code review best practices", s.Description)
	assert.Equal(t, "Full body here.\n", s.Content)
	assert.True(t, s.HasDescription())
}

func TestLoadFrontmatterNameOnly(t *testing.T) {
	parent := t.TempDir()
	content := "---\nname: my-skill\n---\nBody text.\n"
	dir := writeSkillDir(t, parent, "other", content)

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "my-skill", s.Name)
	assert.Empty(t, s.Description)
	assert.False(t, s.HasDescription())
	assert.Equal(t, "Body text.\n", s.Content)
}

func TestLoadFrontmatterDescriptionOnly(t *testing.T) {
	parent := t.TempDir()
	content := "---\ndescription: A useful skill\n---\nBody.\n"
	dir := writeSkillDir(t, parent, "useful", content)

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "useful", s.Name) // name from folder
	assert.Equal(t, "A useful skill", s.Description)
	assert.Equal(t, "Body.\n", s.Content)
}

func TestLoadNoFrontmatter(t *testing.T) {
	parent := t.TempDir()
	content := "# Just Markdown\nNo frontmatter here.\n"
	dir := writeSkillDir(t, parent, "plain", content)

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "plain", s.Name)
	assert.Empty(t, s.Description)
	assert.False(t, s.HasDescription())
	assert.Equal(t, content, s.Content)
}

func TestLoadFrontmatterNoClosingDelimiter(t *testing.T) {
	parent := t.TempDir()
	content := "---\nname: broken\nSome text without closing.\n"
	dir := writeSkillDir(t, parent, "broken", content)

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "broken", s.Name) // from folder, not frontmatter
	assert.Empty(t, s.Description)
	assert.Equal(t, content, s.Content) // full content preserved
}

func TestLoadFrontmatterEmptyBody(t *testing.T) {
	parent := t.TempDir()
	content := "---\nname: empty-body\ndescription: Has no body\n---\n"
	dir := writeSkillDir(t, parent, "empty", content)

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "empty-body", s.Name)
	assert.Equal(t, "Has no body", s.Description)
	assert.Empty(t, s.Content)
}

func TestLoadFrontmatterInvalidYAML(t *testing.T) {
	parent := t.TempDir()
	content := "---\n: invalid: yaml: [broken\n---\nBody.\n"
	writeSkillDir(t, parent, "invalid", content)

	_, err := Load(filepath.Join(parent, "invalid"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid frontmatter")
}

func TestLoadDirMixedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillDir(t, dir, "alpha", "---\ndescription: Alpha desc\n---\nAlpha body")
	writeSkillDir(t, dir, "beta", "Beta plain content")

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
	parent := t.TempDir()
	content := "---\r\nname: win-skill\r\ndescription: Windows style\r\n---\r\nBody with CRLF.\r\n"
	dir := writeSkillDir(t, parent, "win", content)

	s, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, "win-skill", s.Name)
	assert.Equal(t, "Windows style", s.Description)
	assert.Equal(t, "Body with CRLF.\n", s.Content)
}
