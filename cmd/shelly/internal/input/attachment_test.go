package input

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMediaType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"file.png", "image/png"},
		{"file.jpg", "image/jpeg"},
		{"file.jpeg", "image/jpeg"},
		{"file.gif", "image/gif"},
		{"file.webp", "image/webp"},
		{"file.pdf", "application/pdf"},
		{"file.go", "text/plain"},
		{"file.md", "text/plain"},
		{"file.json", "application/json"},
		{"file.yaml", "text/yaml"},
		{"file.yml", "text/yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectMediaType(tt.path, []byte("hello"))
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestClassifyKind(t *testing.T) {
	assert.Equal(t, "image", classifyKind("image/png"))
	assert.Equal(t, "image", classifyKind("image/jpeg"))
	assert.Equal(t, "document", classifyKind("application/pdf"))
	assert.Equal(t, "text", classifyKind("text/plain"))
	assert.Equal(t, "text", classifyKind("application/json"))
	assert.Equal(t, "document", classifyKind("application/octet-stream"))
}

func TestReadAttachment(t *testing.T) {
	dir := t.TempDir()

	t.Run("text file", func(t *testing.T) {
		path := filepath.Join(dir, "hello.go")
		require.NoError(t, os.WriteFile(path, []byte("package main"), 0o600))

		att, err := ReadAttachment(path)
		require.NoError(t, err)
		assert.Equal(t, "text", att.Kind)
		assert.Equal(t, "text/plain", att.MediaType)
		assert.Equal(t, []byte("package main"), att.Data)
	})

	t.Run("directory returns error", func(t *testing.T) {
		_, err := ReadAttachment(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is a directory")
	})

	t.Run("nonexistent returns error", func(t *testing.T) {
		_, err := ReadAttachment(filepath.Join(dir, "nope"))
		assert.Error(t, err)
	})

	t.Run("oversized file returns error", func(t *testing.T) {
		path := filepath.Join(dir, "big.txt")
		f, err := os.Create(path) //nolint:gosec // test file
		require.NoError(t, err)
		require.NoError(t, f.Truncate(maxFileSize+1))
		require.NoError(t, f.Close())

		_, err = ReadAttachment(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})
}

func TestDetectFilePaths(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello"), 0o600))

	t.Run("detects absolute path", func(t *testing.T) {
		paths := DetectFilePaths(testFile)
		assert.Equal(t, []string{testFile}, paths)
	})

	t.Run("detects quoted path", func(t *testing.T) {
		paths := DetectFilePaths("'" + testFile + "'")
		assert.Equal(t, []string{testFile}, paths)
	})

	t.Run("ignores nonexistent", func(t *testing.T) {
		paths := DetectFilePaths("/nonexistent/file.txt")
		assert.Empty(t, paths)
	})

	t.Run("deduplicates", func(t *testing.T) {
		paths := DetectFilePaths(testFile + "\n" + testFile)
		assert.Equal(t, []string{testFile}, paths)
	})
}

func TestAttachmentToPart(t *testing.T) {
	t.Run("image part", func(t *testing.T) {
		att := Attachment{Path: "photo.png", Data: []byte("png"), MediaType: "image/png", Kind: "image"}
		part := att.ToPart()
		assert.Equal(t, "image", part.PartKind())
	})

	t.Run("document part", func(t *testing.T) {
		att := Attachment{Path: "doc.pdf", Data: []byte("pdf"), MediaType: "application/pdf", Kind: "document"}
		part := att.ToPart()
		assert.Equal(t, "document", part.PartKind())
	})

	t.Run("text part", func(t *testing.T) {
		att := Attachment{Path: "code.go", Data: []byte("package main"), MediaType: "text/plain", Kind: "text"}
		part := att.ToPart()
		assert.Equal(t, "text", part.PartKind())
	})
}

func TestAttachmentLabel(t *testing.T) {
	att := Attachment{Path: "/some/path/file.pdf"}
	assert.Equal(t, "[file.pdf]", att.Label())
}
