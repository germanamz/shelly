package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AttachmentWriter stores binary data and returns a reference key.
type AttachmentWriter interface {
	WriteAttachment(data []byte, mediaType string) (ref string, err error)
}

// AttachmentReader loads binary data by reference key.
type AttachmentReader interface {
	ReadAttachment(ref string) (data []byte, mediaType string, err error)
}

// FileAttachmentStore implements AttachmentWriter and AttachmentReader using
// content-addressable files in a directory.
type FileAttachmentStore struct {
	dir string
}

// NewFileAttachmentStore creates a store that reads/writes attachment files under dir.
func NewFileAttachmentStore(dir string) *FileAttachmentStore {
	return &FileAttachmentStore{dir: dir}
}

// WriteAttachment stores data as a file named by its SHA-256 hash. Deduplicates
// by skipping the write if the file already exists.
func (s *FileAttachmentStore) WriteAttachment(data []byte, mediaType string) (string, error) {
	hash := attachmentHash(data)
	filename := hash + attachmentExt(mediaType)
	target := filepath.Join(s.dir, filename)

	if _, err := os.Stat(target); err == nil {
		return filename, nil // already exists
	}

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return "", fmt.Errorf("attachments: mkdir: %w", err)
	}

	if err := atomicWrite(s.dir, target, data); err != nil {
		return "", fmt.Errorf("attachments: write: %w", err)
	}

	return filename, nil
}

// ReadAttachment reads an attachment file and infers the media type from its extension.
func (s *FileAttachmentStore) ReadAttachment(ref string) ([]byte, string, error) {
	path := filepath.Join(s.dir, ref)
	data, err := os.ReadFile(path) //nolint:gosec // path from trusted dir + ref
	if err != nil {
		return nil, "", fmt.Errorf("attachments: read %s: %w", ref, err)
	}
	mediaType := extToMediaType(filepath.Ext(ref))
	return data, mediaType, nil
}

// attachmentHash returns the SHA-256 hex digest of data.
func attachmentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// attachmentExt returns a file extension for the given media type.
func attachmentExt(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

// extToMediaType infers a media type from a file extension.
func extToMediaType(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
