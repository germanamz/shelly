package shellydir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDir_PathAccessors(t *testing.T) {
	d := New("/project/.shelly")

	assert.Equal(t, "/project/.shelly", d.Root())
	assert.Equal(t, "/project/.shelly/config.yaml", d.ConfigPath())
	assert.Equal(t, "/project/.shelly/context.md", d.ContextPath())
	assert.Equal(t, "/project/.shelly/skills", d.SkillsDir())
	assert.Equal(t, "/project/.shelly/local", d.LocalDir())
	assert.Equal(t, "/project/.shelly/local/permissions.json", d.PermissionsPath())
	assert.Equal(t, "/project/.shelly/local/context-cache.json", d.ContextCachePath())
	assert.Equal(t, "/project/.shelly/.gitignore", d.GitignorePath())
}

func TestDir_Exists(t *testing.T) {
	tmp := t.TempDir()

	d := New(filepath.Join(tmp, "missing"))
	assert.False(t, d.Exists())

	d = New(tmp)
	assert.True(t, d.Exists())
}

func TestDir_ContextFiles(t *testing.T) {
	tmp := t.TempDir()

	// Create some .md files and a non-md file.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "context.md"), []byte("ctx"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "notes.md"), []byte("notes"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("yaml"), 0o600))

	d := New(tmp)
	files := d.ContextFiles()

	assert.Len(t, files, 2)
	assert.Equal(t, filepath.Join(tmp, "context.md"), files[0])
	assert.Equal(t, filepath.Join(tmp, "notes.md"), files[1])
}

func TestDir_ContextFiles_Empty(t *testing.T) {
	tmp := t.TempDir()
	d := New(tmp)

	assert.Nil(t, d.ContextFiles())
}

func TestDir_ContextFiles_NonExistent(t *testing.T) {
	d := New("/nonexistent/path/.shelly")

	assert.Nil(t, d.ContextFiles())
}

func TestEnsureStructure(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.Mkdir(root, 0o750))

	d := New(root)
	require.NoError(t, EnsureStructure(d))

	// local/ should exist.
	info, err := os.Stat(d.LocalDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// .gitignore should exist with correct content.
	data, err := os.ReadFile(d.GitignorePath())
	require.NoError(t, err)
	assert.Equal(t, "local/\n", string(data))
}

func TestEnsureStructure_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.Mkdir(root, 0o750))

	d := New(root)
	require.NoError(t, EnsureStructure(d))

	// Write custom content to .gitignore.
	custom := "local/\ncustom-entry\n"
	require.NoError(t, os.WriteFile(d.GitignorePath(), []byte(custom), 0o600))

	// Second call should NOT overwrite the custom .gitignore.
	require.NoError(t, EnsureStructure(d))

	data, err := os.ReadFile(d.GitignorePath())
	require.NoError(t, err)
	assert.Equal(t, custom, string(data))
}

func TestBootstrap(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".shelly")

	d := New(root)
	require.NoError(t, Bootstrap(d))

	// Root should exist.
	assert.True(t, d.Exists())

	// skills/ should exist.
	info, err := os.Stat(d.SkillsDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// local/ should exist.
	info, err = os.Stat(d.LocalDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// .gitignore should exist.
	_, err = os.Stat(d.GitignorePath())
	require.NoError(t, err)

	// config.yaml should exist with skeleton content.
	data, err := os.ReadFile(d.ConfigPath())
	require.NoError(t, err)
	assert.Contains(t, string(data), "entry_agent:")
}

func TestBootstrap_DoesNotOverwrite(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".shelly")

	d := New(root)
	require.NoError(t, Bootstrap(d))

	// Write custom config.
	custom := "custom: true\n"
	require.NoError(t, os.WriteFile(d.ConfigPath(), []byte(custom), 0o600))

	// Second bootstrap should not overwrite.
	require.NoError(t, Bootstrap(d))

	data, err := os.ReadFile(d.ConfigPath())
	require.NoError(t, err)
	assert.Equal(t, custom, string(data))
}

func TestMigratePermissions(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.Mkdir(root, 0o750))

	d := New(root)
	require.NoError(t, EnsureStructure(d))

	// Create old-style permissions file.
	oldPath := filepath.Join(root, "permissions.json")
	permsData := `{"fs_directories":["/tmp"]}`
	require.NoError(t, os.WriteFile(oldPath, []byte(permsData), 0o600))

	require.NoError(t, MigratePermissions(d))

	// Old file should be gone.
	_, err := os.Stat(oldPath)
	assert.True(t, os.IsNotExist(err))

	// New file should exist with same content.
	data, err := os.ReadFile(d.PermissionsPath())
	require.NoError(t, err)
	assert.Equal(t, permsData, string(data))
}

func TestMigratePermissions_NoOldFile(t *testing.T) {
	tmp := t.TempDir()
	d := New(filepath.Join(tmp, ".shelly"))

	// Should be a no-op.
	assert.NoError(t, MigratePermissions(d))
}

func TestMigratePermissions_NewFileExists(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.Mkdir(root, 0o750))

	d := New(root)
	require.NoError(t, EnsureStructure(d))

	// Create both old and new files.
	oldData := `{"old":true}`
	newData := `{"new":true}`
	require.NoError(t, os.WriteFile(filepath.Join(root, "permissions.json"), []byte(oldData), 0o600))
	require.NoError(t, os.WriteFile(d.PermissionsPath(), []byte(newData), 0o600))

	require.NoError(t, MigratePermissions(d))

	// New file should be unchanged (not overwritten).
	data, err := os.ReadFile(d.PermissionsPath())
	require.NoError(t, err)
	assert.Equal(t, newData, string(data))

	// Old file should still exist (not deleted since we didn't move).
	_, err = os.Stat(filepath.Join(root, "permissions.json"))
	assert.NoError(t, err)
}
