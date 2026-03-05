package input

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/germanamz/shelly/pkg/chats/content"
)

const (
	maxFileSize = 20 << 20 // 20 MB per file
	sniffBytes  = 512
)

// Attachment represents a pending file attachment.
type Attachment struct {
	Path      string
	Data      []byte
	MediaType string
	Kind      string // "image", "document", "text"
}

// Label returns a short display label for the attachment.
func (a Attachment) Label() string {
	name := filepath.Base(a.Path)
	return fmt.Sprintf("[%s]", name)
}

// ToPart converts an Attachment to a content.Part.
func (a Attachment) ToPart() content.Part {
	switch a.Kind {
	case "image":
		return content.Image{
			Data:      a.Data,
			MediaType: a.MediaType,
		}
	case "document":
		return content.Document{
			Path:      a.Path,
			Data:      a.Data,
			MediaType: a.MediaType,
		}
	default:
		// text — inline with filename header
		return content.Text{
			Text: fmt.Sprintf("--- file: %s ---\n%s\n--- end file ---", a.Path, string(a.Data)),
		}
	}
}

// ReadAttachment reads a file and creates an Attachment with detected MIME type.
func ReadAttachment(path string) (Attachment, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot access %s: %w", path, err)
	}
	if info.IsDir() {
		return Attachment{}, fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxFileSize {
		return Attachment{}, fmt.Errorf("%s exceeds %d MB limit", filepath.Base(path), maxFileSize>>20)
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is user-selected via file picker or paste
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	mediaType := detectMediaType(path, data)
	kind := classifyKind(mediaType)

	return Attachment{
		Path:      path,
		Data:      data,
		MediaType: mediaType,
		Kind:      kind,
	}, nil
}

// detectMediaType returns a MIME type using extension mapping first, then content sniffing.
func detectMediaType(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))

	// Extension-based mapping for common types.
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".go", ".rs", ".py", ".js", ".ts", ".tsx", ".jsx", ".c", ".cpp", ".h", ".rb", ".java", ".kt", ".swift", ".sh", ".bash", ".zsh":
		return "text/plain"
	case ".md", ".txt", ".csv", ".log", ".ini", ".cfg", ".toml":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".xml", ".html", ".htm", ".css", ".sql":
		return "text/plain"
	}

	// Content sniffing fallback.
	sniff := data
	if len(sniff) > sniffBytes {
		sniff = sniff[:sniffBytes]
	}
	return http.DetectContentType(sniff)
}

// classifyKind maps a MIME type to "image", "document", or "text".
func classifyKind(mediaType string) string {
	if strings.HasPrefix(mediaType, "image/") {
		return "image"
	}
	if mediaType == "application/pdf" {
		return "document"
	}
	if strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" || mediaType == "text/yaml" {
		return "text"
	}
	// Unknown binary — treat as document.
	return "document"
}

// DetectFilePaths scans text for file paths and returns those that exist on disk.
func DetectFilePaths(text string) []string {
	candidates := extractPathCandidates(text)
	var valid []string
	seen := make(map[string]bool)
	for _, c := range candidates {
		c = expandHome(c)
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			seen[abs] = true
			valid = append(valid, abs)
		}
	}
	return valid
}

// extractPathCandidates extracts potential file paths from text.
func extractPathCandidates(text string) []string {
	var candidates []string

	// Split by newlines, then process each line.
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try quoted paths first.
		for _, q := range []byte{'"', '\''} {
			for {
				start := strings.IndexByte(line, q)
				if start == -1 {
					break
				}
				end := strings.IndexByte(line[start+1:], q)
				if end == -1 {
					break
				}
				path := line[start+1 : start+1+end]
				if looksLikePath(path) {
					candidates = append(candidates, path)
				}
				line = line[start+1+end+1:]
			}
		}

		// Try space-separated tokens for unquoted paths.
		for token := range strings.FieldsSeq(line) {
			if looksLikePath(token) {
				candidates = append(candidates, token)
			}
		}
	}

	return candidates
}

// looksLikePath returns true if s looks like a file path.
func looksLikePath(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "./")
}

// expandHome expands ~ prefix to the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
