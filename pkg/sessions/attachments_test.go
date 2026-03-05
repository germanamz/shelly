package sessions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachmentHash(t *testing.T) {
	data := []byte("hello world")
	h1 := attachmentHash(data)
	h2 := attachmentHash(data)
	assert.Equal(t, h1, h2, "hash should be deterministic")
	assert.Len(t, h1, 64, "SHA-256 hex digest should be 64 chars")

	different := attachmentHash([]byte("different"))
	assert.NotEqual(t, h1, different)
}

func TestAttachmentExt(t *testing.T) {
	tests := []struct {
		mediaType string
		want      string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"image/svg+xml", ".svg"},
		{"application/pdf", ".pdf"},
		{"application/octet-stream", ".bin"},
		{"unknown/type", ".bin"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, attachmentExt(tt.mediaType), "mediaType=%s", tt.mediaType)
	}
}

func TestFileAttachmentStore_WriteRead_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	data := []byte{0x89, 0x50, 0x4E, 0x47} // fake PNG header
	ref, err := store.WriteAttachment(data, "image/png")
	require.NoError(t, err)
	assert.Contains(t, ref, ".png")

	gotData, gotMedia, err := store.ReadAttachment(ref)
	require.NoError(t, err)
	assert.Equal(t, data, gotData)
	assert.Equal(t, "image/png", gotMedia)
}

func TestFileAttachmentStore_Dedup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	data := []byte("same content")
	ref1, err := store.WriteAttachment(data, "image/png")
	require.NoError(t, err)

	ref2, err := store.WriteAttachment(data, "image/png")
	require.NoError(t, err)

	assert.Equal(t, ref1, ref2, "same data should produce same ref")

	// Verify only one file on disk.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestFileAttachmentStore_ReadNotFound(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	store := NewFileAttachmentStore(dir)

	_, _, err := store.ReadAttachment("nonexistent.png")
	assert.Error(t, err)
}
