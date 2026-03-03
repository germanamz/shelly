package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

func TestTrustDomain(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	require.NoError(t, s.TrustDomain("example.com"))
	assert.True(t, s.IsDomainTrusted("example.com"))
	assert.False(t, s.IsDomainTrusted("other.com"))
}

func TestPersistence_Domains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.json")

	s1, err := New(path)
	require.NoError(t, err)
	require.NoError(t, s1.TrustDomain("api.example.com"))

	s2, err := New(path)
	require.NoError(t, err)
	assert.True(t, s2.IsDomainTrusted("api.example.com"))
	assert.False(t, s2.IsDomainTrusted("other.com"))
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

func TestApprovedDirs(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	assert.Empty(t, s.ApprovedDirs())

	require.NoError(t, s.ApproveDir("/a"))
	require.NoError(t, s.ApproveDir("/b"))

	dirs := s.ApprovedDirs()
	assert.Len(t, dirs, 2)
	assert.ElementsMatch(t, []string{"/a", "/b"}, dirs)
}

func TestOnDirApproved_CallbackFires(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	var got []string
	s.OnDirApproved(func(d string) {
		got = append(got, d)
	})

	require.NoError(t, s.ApproveDir("/first"))
	require.NoError(t, s.ApproveDir("/second"))

	assert.Equal(t, []string{"/first", "/second"}, got)
}

func TestOnDirApproved_NoDuplicateFire(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	callCount := 0
	s.OnDirApproved(func(string) {
		callCount++
	})

	require.NoError(t, s.ApproveDir("/dup"))
	require.NoError(t, s.ApproveDir("/dup")) // re-approve same dir

	assert.Equal(t, 1, callCount, "callback should not fire on re-approval")
}

func TestOnDirApproved_Unsubscribe(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	callCount := 0
	unsub := s.OnDirApproved(func(string) {
		callCount++
	})

	require.NoError(t, s.ApproveDir("/before"))
	assert.Equal(t, 1, callCount)

	unsub()

	require.NoError(t, s.ApproveDir("/after"))
	assert.Equal(t, 1, callCount, "callback should not fire after unsubscribe")
}

func TestOnDirApproved_ConcurrentSafety(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "perms.json"))
	require.NoError(t, err)

	var mu sync.Mutex
	var got []string
	s.OnDirApproved(func(d string) {
		mu.Lock()
		got = append(got, d)
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = s.ApproveDir(filepath.Join("/concurrent", fmt.Sprintf("%d", n)))
		}(i)
	}
	wg.Wait()

	mu.Lock()
	assert.Len(t, got, 10)
	mu.Unlock()
}
