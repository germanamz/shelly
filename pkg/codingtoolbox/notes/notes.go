// Package notes provides persistent note-taking tools for agents. Notes are
// stored as Markdown files on disk and survive context compaction, allowing
// agents to re-read important information after a context reset.
package notes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// validName matches names that contain only alphanumeric characters, hyphens,
// and underscores.
var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Store manages persistent notes stored as Markdown files in a directory.
type Store struct {
	dir string
}

// New creates a Store that persists notes in the given directory. The directory
// is created (along with any necessary parents) on the first write operation.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Tools returns a ToolBox with write_note, read_note, and list_notes tools.
func (s *Store) Tools() *toolbox.ToolBox {
	tb := toolbox.New()

	tb.Register(
		toolbox.Tool{
			Name:        "write_note",
			Description: "Create or overwrite a persistent note. Notes survive context compaction and can be re-read later to restore important context. Use descriptive names like 'architecture-decisions' or 'user_preferences'.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Note name (alphanumeric, hyphens, underscores only)"},"content":{"type":"string","description":"Markdown content of the note"}},"required":["name","content"]}`),
			Handler:     s.handleWrite,
		},
		toolbox.Tool{
			Name:        "read_note",
			Description: "Read the content of a previously saved note by name.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Name of the note to read"}},"required":["name"]}`),
			Handler:     s.handleRead,
		},
		toolbox.Tool{
			Name:        "list_notes",
			Description: "List all available notes with a first-line preview of each.",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler:     s.handleList,
		},
	)

	return tb
}

// --- input types ---

type writeInput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type readInput struct {
	Name string `json:"name"`
}

// --- handlers ---

func (s *Store) handleWrite(_ context.Context, input json.RawMessage) (string, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("write_note: invalid input: %w", err)
	}

	if in.Name == "" {
		return "", fmt.Errorf("write_note: name is required")
	}

	if !validName.MatchString(in.Name) {
		return "", fmt.Errorf("write_note: invalid name %q: only alphanumeric characters, hyphens, and underscores are allowed", in.Name)
	}

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return "", fmt.Errorf("write_note: failed to create directory: %w", err)
	}

	path := filepath.Join(s.dir, in.Name+".md")
	path = filepath.Clean(path)

	if err := os.WriteFile(path, []byte(in.Content), 0o600); err != nil {
		return "", fmt.Errorf("write_note: failed to write file: %w", err)
	}

	return fmt.Sprintf("Note %q saved.", in.Name), nil
}

func (s *Store) handleRead(_ context.Context, input json.RawMessage) (string, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("read_note: invalid input: %w", err)
	}

	if in.Name == "" {
		return "", fmt.Errorf("read_note: name is required")
	}

	if !validName.MatchString(in.Name) {
		return "", fmt.Errorf("read_note: invalid name %q: only alphanumeric characters, hyphens, and underscores are allowed", in.Name)
	}

	path := filepath.Join(s.dir, in.Name+".md")
	path = filepath.Clean(path)

	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from validated name, no user-controlled traversal
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("read_note: note %q not found", in.Name)
		}
		return "", fmt.Errorf("read_note: failed to read file: %w", err)
	}

	return string(data), nil
}

func (s *Store) handleList(_ context.Context, _ json.RawMessage) (string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No notes found.", nil
		}
		return "", fmt.Errorf("list_notes: failed to read directory: %w", err)
	}

	var lines []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		name := strings.TrimSuffix(e.Name(), ".md")
		preview := firstLine(filepath.Join(s.dir, e.Name()))
		lines = append(lines, fmt.Sprintf("- %s: %s", name, preview))
	}

	if len(lines) == 0 {
		return "No notes found.", nil
	}

	return strings.Join(lines, "\n"), nil
}

// firstLine reads the first non-empty line of a file for preview purposes.
func firstLine(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from directory listing, not user input
	if err != nil {
		return "(unable to read)"
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if len(trimmed) > 80 {
				return trimmed[:80] + "..."
			}
			return trimmed
		}
	}

	return "(empty)"
}
