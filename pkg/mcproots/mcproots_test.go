package mcproots

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithRoots_FromContext(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, FromContext(ctx), "no roots set should return nil")

	roots := []string{"/home/user/projects", "/tmp"}
	ctx = WithRoots(ctx, roots)
	assert.Equal(t, roots, FromContext(ctx))
}

func TestFromContext_EmptySlice(t *testing.T) {
	ctx := WithRoots(context.Background(), []string{})
	got := FromContext(ctx)
	assert.NotNil(t, got, "empty slice should not be nil")
	assert.Empty(t, got)
}

func TestIsPathAllowed_NilRoots(t *testing.T) {
	assert.True(t, IsPathAllowed("/any/path", nil), "nil roots = unconstrained")
}

func TestIsPathAllowed_EmptyRoots(t *testing.T) {
	assert.False(t, IsPathAllowed("/any/path", []string{}), "empty roots = nothing allowed")
}

func TestIsPathAllowed_ExactMatch(t *testing.T) {
	roots := []string{"/home/user/projects"}
	assert.True(t, IsPathAllowed("/home/user/projects", roots))
}

func TestIsPathAllowed_Subdirectory(t *testing.T) {
	roots := []string{"/home/user/projects"}
	assert.True(t, IsPathAllowed("/home/user/projects/foo/bar.txt", roots))
}

func TestIsPathAllowed_OutsideRoot(t *testing.T) {
	roots := []string{"/home/user/projects"}
	assert.False(t, IsPathAllowed("/etc/passwd", roots))
}

func TestIsPathAllowed_NoPartialMatch(t *testing.T) {
	roots := []string{"/tmp"}
	assert.False(t, IsPathAllowed("/tmpfoo/bar", roots), "should not match partial prefix")
}

func TestIsPathAllowed_MultipleRoots(t *testing.T) {
	roots := []string{"/home/user/projects", "/tmp/scratch"}
	assert.True(t, IsPathAllowed("/tmp/scratch/file.txt", roots))
	assert.True(t, IsPathAllowed("/home/user/projects/main.go", roots))
	assert.False(t, IsPathAllowed("/var/log/syslog", roots))
}

func TestIsPathAllowed_TrailingSlash(t *testing.T) {
	roots := []string{"/home/user/projects/"}
	assert.True(t, IsPathAllowed("/home/user/projects/foo", roots))
}
