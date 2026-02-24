package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/shellydir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSkillFiles(t *testing.T) {
	tmp := t.TempDir()
	d := shellydir.New(filepath.Join(tmp, ".shelly"))
	require.NoError(t, os.MkdirAll(d.SkillsDir(), 0o750))

	files := []skillFile{
		{Name: "test-skill", Content: "---\ndescription: test\n---\n# Test\n"},
	}

	require.NoError(t, writeSkillFiles(d, files))

	// Verify the file was created.
	data, err := os.ReadFile(filepath.Join(d.SkillsDir(), "test-skill", "SKILL.md")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, files[0].Content, string(data))
}

func TestWriteSkillFiles_DoesNotOverwrite(t *testing.T) {
	tmp := t.TempDir()
	d := shellydir.New(filepath.Join(tmp, ".shelly"))
	skillDir := filepath.Join(d.SkillsDir(), "existing")
	require.NoError(t, os.MkdirAll(skillDir, 0o750))

	existing := "original content"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(existing), 0o600))

	files := []skillFile{
		{Name: "existing", Content: "new content"},
	}

	require.NoError(t, writeSkillFiles(d, files))

	// Verify original content is preserved.
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, existing, string(data))
}

func TestWriteSkillFiles_Empty(t *testing.T) {
	tmp := t.TempDir()
	d := shellydir.New(filepath.Join(tmp, ".shelly"))
	require.NoError(t, os.MkdirAll(d.SkillsDir(), 0o750))

	// No skill files to write — should be a no-op.
	assert.NoError(t, writeSkillFiles(d, nil))
}
