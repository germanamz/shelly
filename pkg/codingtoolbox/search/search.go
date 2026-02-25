// Package search provides tools for searching file contents and finding files
// by name patterns. Directory access is gated by the shared permissions store.
package search

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// Search provides search tools with permission gating.
type Search struct {
	store *permissions.Store
	ask   AskFunc
}

// New creates a Search backed by the given shared permissions store.
func New(store *permissions.Store, askFn AskFunc) *Search {
	return &Search{store: store, ask: askFn}
}

// Tools returns a ToolBox containing the search tools.
func (s *Search) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(s.contentTool(), s.filesTool())

	return tb
}

// checkPermission ensures the directory is approved.
func (s *Search) checkPermission(ctx context.Context, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("search: resolve path: %w", err)
	}

	if s.store.IsDirApproved(abs) {
		return nil
	}

	resp, err := s.ask(ctx, fmt.Sprintf("Allow search access to %s?", abs), []string{"yes", "no"})
	if err != nil {
		return fmt.Errorf("search: ask permission: %w", err)
	}

	if !strings.EqualFold(resp, "yes") {
		return fmt.Errorf("search: access denied to %s", abs)
	}

	return s.store.ApproveDir(abs)
}

// --- search_content ---

type contentInput struct {
	Pattern    string `json:"pattern"`
	Directory  string `json:"directory"`
	MaxResults int    `json:"max_results"`
}

type contentMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

func (s *Search) contentTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "search_content",
		Description: "Search file contents using a regular expression. Returns matching lines with file path and line number. Use to find code patterns, definitions, or references across a directory. Returns matching lines only — use fs_read to see surrounding context after finding matches.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Regular expression pattern to search for"},"directory":{"type":"string","description":"Directory to search in"},"max_results":{"type":"integer","description":"Maximum number of results (default 100)"}},"required":["pattern","directory"]}`),
		Handler:     s.handleContent,
	}
}

func (s *Search) handleContent(ctx context.Context, input json.RawMessage) (string, error) {
	var in contentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("search_content: invalid input: %w", err)
	}

	if in.Pattern == "" {
		return "", fmt.Errorf("search_content: pattern is required")
	}

	if in.Directory == "" {
		return "", fmt.Errorf("search_content: directory is required")
	}

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", fmt.Errorf("search_content: invalid pattern: %w", err)
	}

	if err := s.checkPermission(ctx, in.Directory); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Directory)
	if err != nil {
		return "", fmt.Errorf("search_content: %w", err)
	}

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	const maxTotalBytes = 1 << 20 // 1MB total content cap

	var (
		matches    []contentMatch
		totalBytes int
	)

	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors
		}

		if d.IsDir() {
			return nil
		}

		if len(matches) >= maxResults || totalBytes >= maxTotalBytes {
			return filepath.SkipAll
		}

		if !isTextFile(path) {
			return nil
		}

		file, err := os.Open(path) //nolint:gosec // path is approved by user
		if err != nil {
			return nil // skip unreadable files
		}
		defer file.Close() //nolint:errcheck // best-effort close on read

		rel, _ := filepath.Rel(abs, path)
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 1<<20) // allow lines up to 1MB
		lineNum := 0

		for scanner.Scan() {
			lineNum++

			line := scanner.Text()
			if re.MatchString(line) {
				matches = append(matches, contentMatch{
					Path:    rel,
					Line:    lineNum,
					Content: line,
				})

				totalBytes += len(line)

				if len(matches) >= maxResults || totalBytes >= maxTotalBytes {
					return filepath.SkipAll
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return nil // skip files with scan errors
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search_content: %w", err)
	}

	data, err := json.Marshal(matches)
	if err != nil {
		return "", fmt.Errorf("search_content: marshal: %w", err)
	}

	return string(data), nil
}

// --- search_files ---

type filesInput struct {
	Pattern    string `json:"pattern"`
	Directory  string `json:"directory"`
	MaxResults int    `json:"max_results"`
}

func (s *Search) filesTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "search_files",
		Description: "Find files by name pattern (supports glob with ** for recursive matching). Returns matching file paths. Use to locate files before reading them. Example patterns: '**/*.go' for all Go files, '**/test_*' for test files.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Glob pattern to match file names (supports **)"},"directory":{"type":"string","description":"Directory to search in"},"max_results":{"type":"integer","description":"Maximum number of results (default 100)"}},"required":["pattern","directory"]}`),
		Handler:     s.handleFiles,
	}
}

func (s *Search) handleFiles(ctx context.Context, input json.RawMessage) (string, error) {
	var in filesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("search_files: invalid input: %w", err)
	}

	if in.Pattern == "" {
		return "", fmt.Errorf("search_files: pattern is required")
	}

	if in.Directory == "" {
		return "", fmt.Errorf("search_files: directory is required")
	}

	if err := s.checkPermission(ctx, in.Directory); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(in.Directory)
	if err != nil {
		return "", fmt.Errorf("search_files: %w", err)
	}

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	var results []string

	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if len(results) >= maxResults {
			return filepath.SkipAll
		}

		rel, _ := filepath.Rel(abs, path)
		if matchGlob(in.Pattern, rel) {
			results = append(results, rel)
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search_files: %w", err)
	}

	data, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("search_files: marshal: %w", err)
	}

	return string(data), nil
}

// --- helpers ---

// matchGlob matches a glob pattern against a path, supporting ** for any
// number of path segments. It falls back to filepath.Match for simple patterns.
func matchGlob(pattern, path string) bool {
	// Handle ** patterns.
	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, path)
	}

	// When the pattern contains a directory separator, match against the
	// full relative path instead of just the base name.
	if strings.ContainsRune(pattern, filepath.Separator) || strings.ContainsRune(pattern, '/') {
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	matched, _ := filepath.Match(pattern, filepath.Base(path))

	return matched
}

// matchDoublestar handles glob patterns containing **.
func matchDoublestar(pattern, path string) bool {
	parts := strings.Split(pattern, "**")
	if len(parts) == 2 {
		prefix := strings.TrimSuffix(parts[0], string(filepath.Separator))
		suffix := strings.TrimPrefix(parts[1], string(filepath.Separator))

		// ** at start: match suffix against end of path.
		if prefix == "" {
			if suffix == "" {
				return true
			}

			// Check if any suffix of the path matches the glob.
			segments := strings.Split(path, string(filepath.Separator))
			for i := range segments {
				candidate := strings.Join(segments[i:], string(filepath.Separator))
				if matched, _ := filepath.Match(suffix, candidate); matched {
					return true
				}
			}

			// Also try matching just the filename.
			matched, _ := filepath.Match(suffix, filepath.Base(path))

			return matched
		}

		// ** in middle or end.
		if !strings.HasPrefix(path, prefix+string(filepath.Separator)) && path != prefix {
			return false
		}

		if suffix == "" {
			return true
		}

		rest := strings.TrimPrefix(path, prefix+string(filepath.Separator))
		segments := strings.Split(rest, string(filepath.Separator))

		for i := range segments {
			candidate := strings.Join(segments[i:], string(filepath.Separator))
			if matched, _ := filepath.Match(suffix, candidate); matched {
				return true
			}
		}

		return false
	}

	// Fallback for complex patterns.
	matched, _ := filepath.Match(pattern, path)

	return matched
}

// isTextFile does a quick check to determine if a file is likely text.
func isTextFile(path string) bool {
	f, err := os.Open(path) //nolint:gosec // path is approved by user
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // best-effort close on read

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	return utf8.Valid(buf[:n])
}
