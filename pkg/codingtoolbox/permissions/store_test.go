package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_NoFile(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, ".shelly", "perms.json"))
	require.NoError(t, err)

	assert.False(t, s.IsDirApproved("/tmp"))
	assert.False(t, s.IsCommandTrusted("git"))
}

func TestApproveDir(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	require.NoError(t, s.ApproveDir("/home/user/projects"))
	assert.True(t, s.IsDirApproved("/home/user/projects"))
	assert.False(t, s.IsDirApproved("/home/user"))
}

func TestIsDirApproved_Ancestry(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	require.NoError(t, s.ApproveDir("/home/user"))
	assert.True(t, s.IsDirApproved("/home/user/projects/foo"))
}

func TestTrustCommand(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	require.NoError(t, s.TrustCommand("git"))
	assert.True(t, s.IsCommandTrusted("git"))
	assert.False(t, s.IsCommandTrusted("rm"))
}

func TestPersistence_NewFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.json")

	s1, err := New(path)
	require.NoError(t, err)
	require.NoError(t, s1.ApproveDir("/tmp/test"))
	require.NoError(t, s1.TrustCommand("npm"))

	// Second store loads from the same file.
	s2, err := New(path)
	require.NoError(t, err)
	assert.True(t, s2.IsDirApproved("/tmp/test"))
	assert.True(t, s2.IsCommandTrusted("npm"))
}

func TestLoad_LegacyFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.json")

	// Write legacy format (flat array of directories).
	legacy, err := json.Marshal([]string{"/home/user", "/tmp"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, legacy, 0o600))

	s, err := New(path)
	require.NoError(t, err)
	assert.True(t, s.IsDirApproved("/home/user"))
	assert.True(t, s.IsDirApproved("/tmp"))
	assert.False(t, s.IsCommandTrusted("anything"))
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.json")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o600))

	s, err := New(path)
	require.NoError(t, err)
	assert.False(t, s.IsDirApproved("/anything"))
}

func TestPersist_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "perms.json")

	s, err := New(path)
	require.NoError(t, err)
	require.NoError(t, s.TrustCommand("echo"))

	// File should exist.
	_, err = os.Stat(path)
	assert.NoError(t, err)
}
