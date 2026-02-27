package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	metas := List()
	assert.GreaterOrEqual(t, len(metas), 2, "should have at least 2 templates")

	names := make(map[string]bool, len(metas))
	for _, m := range metas {
		assert.NotEmpty(t, m.Name)
		assert.NotEmpty(t, m.Description)
		names[m.Name] = true
	}

	assert.True(t, names["simple-assistant"], "should include simple-assistant")
	assert.True(t, names["coding-team"], "should include coding-team")
}

func TestGet(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		tmpl, err := Get("simple-assistant")
		require.NoError(t, err)
		assert.Equal(t, "simple-assistant", tmpl.Meta.Name)
		assert.NotEmpty(t, tmpl.Config.Providers)
		assert.NotEmpty(t, tmpl.Config.Agents)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := Get("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestApply(t *testing.T) {
	t.Run("simple assistant", func(t *testing.T) {
		dir := t.TempDir()
		shellyDir := filepath.Join(dir, ".shelly")

		tmpl, err := Get("simple-assistant")
		require.NoError(t, err)

		err = Apply(tmpl, shellyDir, false)
		require.NoError(t, err)

		// Config file should exist.
		configPath := filepath.Join(shellyDir, "config.yaml")
		assert.FileExists(t, configPath)

		// Local dir and .gitignore should exist.
		assert.DirExists(t, filepath.Join(shellyDir, "local"))
		assert.FileExists(t, filepath.Join(shellyDir, ".gitignore"))

		// No embedded skills for simple-assistant.
		assert.Empty(t, tmpl.EmbeddedSkills)
	})

	t.Run("coding team with skills", func(t *testing.T) {
		dir := t.TempDir()
		shellyDir := filepath.Join(dir, ".shelly")

		tmpl, err := Get("coding-team")
		require.NoError(t, err)

		err = Apply(tmpl, shellyDir, false)
		require.NoError(t, err)

		assert.FileExists(t, filepath.Join(shellyDir, "config.yaml"))

		// Skills should be written.
		for _, sk := range tmpl.EmbeddedSkills {
			skillPath := filepath.Join(shellyDir, "skills", sk.Name, "SKILL.md")
			assert.FileExists(t, skillPath, "skill %q should exist", sk.Name)

			content, err := os.ReadFile(skillPath) //nolint:gosec // test code with controlled paths
			require.NoError(t, err)
			assert.NotEmpty(t, content)
		}
	})

	t.Run("refuses overwrite without force", func(t *testing.T) {
		dir := t.TempDir()
		shellyDir := filepath.Join(dir, ".shelly")

		tmpl, err := Get("simple-assistant")
		require.NoError(t, err)

		// First apply succeeds.
		err = Apply(tmpl, shellyDir, false)
		require.NoError(t, err)

		// Second apply without force should fail.
		err = Apply(tmpl, shellyDir, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("force overwrites", func(t *testing.T) {
		dir := t.TempDir()
		shellyDir := filepath.Join(dir, ".shelly")

		tmpl, err := Get("simple-assistant")
		require.NoError(t, err)

		err = Apply(tmpl, shellyDir, false)
		require.NoError(t, err)

		// Force apply should succeed.
		err = Apply(tmpl, shellyDir, true)
		assert.NoError(t, err)
	})
}
