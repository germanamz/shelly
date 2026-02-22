package projectctx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/shellydir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateIndex_GoModule(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/example/proj\n\ngo 1.21\n"), 0o600))

	idx := generateIndex(tmp)
	assert.Contains(t, idx, "Go module: github.com/example/proj")
}

func TestGenerateIndex_EntryPoints(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example\n"), 0o600))

	cmdDir := filepath.Join(tmp, "cmd", "myapp")
	require.NoError(t, os.MkdirAll(cmdDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte("package main\n"), 0o600))

	idx := generateIndex(tmp)
	assert.Contains(t, idx, "cmd/myapp/main.go")
}

func TestGenerateIndex_Packages(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example\n"), 0o600))

	pkgDir := filepath.Join(tmp, "pkg", "foo")
	require.NoError(t, os.MkdirAll(pkgDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "foo.go"), []byte("package foo\n"), 0o600))

	barDir := filepath.Join(tmp, "pkg", "bar")
	require.NoError(t, os.MkdirAll(barDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(barDir, "bar.go"), []byte("package bar\n"), 0o600))

	idx := generateIndex(tmp)
	assert.Contains(t, idx, "pkg/bar")
	assert.Contains(t, idx, "pkg/foo")
}

func TestGenerateIndex_Empty(t *testing.T) {
	tmp := t.TempDir()
	idx := generateIndex(tmp)
	assert.Empty(t, idx)
}

func TestGenerate_WritesCache(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example\n"), 0o600))

	shellyRoot := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.MkdirAll(shellyRoot, 0o750))

	d := shellydir.New(shellyRoot)
	require.NoError(t, shellydir.EnsureStructure(d))

	idx, err := Generate(tmp, d)
	require.NoError(t, err)
	assert.Contains(t, idx, "Go module: example")

	// Cache file should exist.
	data, err := os.ReadFile(d.ContextCachePath())
	require.NoError(t, err)
	assert.Equal(t, idx, string(data))
}

func TestReadModule(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/foo/bar\n\ngo 1.22\n"), 0o600))

	assert.Equal(t, "github.com/foo/bar", readModule(tmp))
}

func TestReadModule_NotFound(t *testing.T) {
	assert.Empty(t, readModule("/nonexistent"))
}

func TestFindEntryPoints_None(t *testing.T) {
	tmp := t.TempDir()
	assert.Nil(t, findEntryPoints(tmp))
}

func TestFindPackages_NoPkgDir(t *testing.T) {
	tmp := t.TempDir()
	assert.Nil(t, findPackages(tmp))
}

func TestFindPackages_DepthLimit(t *testing.T) {
	tmp := t.TempDir()

	// Create a deeply nested package.
	deepDir := filepath.Join(tmp, "pkg", "a", "b", "c", "d", "e")
	require.NoError(t, os.MkdirAll(deepDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(deepDir, "deep.go"), []byte("package e\n"), 0o600))

	// Create a shallow package.
	shallowDir := filepath.Join(tmp, "pkg", "top")
	require.NoError(t, os.MkdirAll(shallowDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(shallowDir, "top.go"), []byte("package top\n"), 0o600))

	pkgs := findPackages(tmp)
	assert.Contains(t, pkgs, "pkg/top")
	// Depth 5 (a/b/c/d/e) should be skipped.
	assert.NotContains(t, pkgs, "pkg/a/b/c/d/e")
}
