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
	"sync"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// pendingResult holds the outcome of a single in-flight permission prompt so
// that concurrent callers waiting on the same directory can share the result.
type pendingResult struct {
	done chan struct{}
	err  error
}

// Search provides search tools with permission gating.
type Search struct {
	store      *permissions.Store
	ask        AskFunc
	pendingMu  sync.Mutex
	pendingDir map[string]*pendingResult
}

// New creates a Search backed by the given shared permissions store.
func New(store *permissions.Store, askFn AskFunc) *Search {
	return &Search{store: store, ask: askFn, pendingDir: make(map[string]*pendingResult)}
}

// Tools returns a ToolBox containing the search tools.
func (s *Search) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(s.contentTool(), s.filesTool())

	return tb
}

// checkPermission ensures the directory is approved. Concurrent calls for the
// same directory coalesce into a single prompt so the user is never asked the
// same question multiple times.
func (s *Search) checkPermission(ctx context.Context, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("search: resolve path: %w", err)
	}

	// Resolve symlinks so a symlink pointing outside the intended directory
	// requires independent approval (matching filesystem.checkPermission).
	realAbs, evalErr := filepath.EvalSymlinks(abs)
	if evalErr != nil && !os.IsNotExist(evalErr) {
		return fmt.Errorf("search: resolve symlink: %w", evalErr)
	}

	if realAbs != "" && realAbs != abs {
		if err := s.askAndApproveDir(ctx, realAbs); err != nil {
			return err
		}
	}

	// Fast path: already approved (no lock contention).
	if s.store.IsDirApproved(abs) {
		return nil
	}

	s.pendingMu.Lock()
	// Re-check after acquiring lock — another goroutine may have approved.
	if s.store.IsDirApproved(abs) {
		s.pendingMu.Unlock()
		return nil
	}

	// If a prompt is already in-flight for this dir, wait for its result.
	if pr, ok := s.pendingDir[abs]; ok {
		s.pendingMu.Unlock()
		<-pr.done

		if pr.err != nil {
			return pr.err
		}

		return nil // Permission was granted (either "yes" or "trust")
	}

	// We are the first — create a pending entry and release the lock.
	pr := &pendingResult{done: make(chan struct{})}
	s.pendingDir[abs] = pr
	s.pendingMu.Unlock()

	// Ask the user (blocking).
	pr.err = s.askAndApproveDir(ctx, abs)

	// Signal waiters and clean up.
	close(pr.done)
	s.pendingMu.Lock()
	delete(s.pendingDir, abs)
	s.pendingMu.Unlock()

	return pr.err
}

// askAndApproveDir prompts the user and approves the directory on "yes".
func (s *Search) askAndApproveDir(ctx context.Context, dir string) error {
	resp, err := s.ask(ctx, fmt.Sprintf("Allow search access to %s?", dir), []string{"yes", "no"})
	if err != nil {
		return fmt.Errorf("search: ask permission: %w", err)
	}

	if !strings.EqualFold(resp, "yes") {
		return fmt.Errorf("search: access denied to %s", dir)
	}

	return s.store.ApproveDir(dir)
}

// --- search_content ---

type contentInput struct {
	Pattern      string `json:"pattern"`
	Directory    string `json:"directory"`
	MaxResults   int    `json:"max_results"`
	ContextLines int    `json:"context_lines"` // number of surrounding lines to include per match
}

type contentMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
	Context string `json:"context,omitempty"` // surrounding lines when context_lines > 0
}

func (s *Search) contentTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "search_content",
		Description: "Search file contents using a regular expression. Returns matching lines with file path and line number. Set context_lines to include N surrounding lines per match (avoids a follow-up fs_read_lines in many cases). Use to find code patterns, definitions, or references across a directory.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Regular expression pattern to search for"},"directory":{"type":"string","description":"Directory to search in"},"max_results":{"type":"integer","description":"Maximum number of results (default 100)"},"context_lines":{"type":"integer","description":"Number of lines before and after each match to include in the context field (default 0)"}},"required":["pattern","directory"]}`),
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

	absReal, err := filepath.EvalSymlinks(abs)
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

		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil
		}
		if realPath != absReal && !strings.HasPrefix(realPath, absReal+string(filepath.Separator)) {
			return nil
		}

		if len(matches) >= maxResults || totalBytes >= maxTotalBytes {
			return filepath.SkipAll
		}

		if !isTextFile(realPath) {
			return nil
		}

		rel, _ := filepath.Rel(abs, path)

		if in.ContextLines > 0 {
			return s.searchFileWithContext(realPath, rel, re, in.ContextLines, maxResults, maxTotalBytes, &matches, &totalBytes)
		}

		file, err := os.Open(realPath) //nolint:gosec // path is approved by user
		if err != nil {
			return nil // skip unreadable files
		}
		defer file.Close() //nolint:errcheck // best-effort close on read

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

// searchFileWithContext reads all lines from a file and appends matches that
// include context_lines surrounding lines. Each match's Context field contains
// the window formatted as " N→content" lines, with ">N→content" on the match.
func (s *Search) searchFileWithContext(
	realPath, rel string,
	re *regexp.Regexp,
	contextLines, maxResults, maxTotalBytes int,
	matches *[]contentMatch,
	totalBytes *int,
) error {
	data, err := os.ReadFile(realPath) //nolint:gosec // path is approved by user
	if err != nil {
		return nil // skip unreadable files
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	// Trim trailing empty element from a trailing newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	for i, line := range lines {
		if !re.MatchString(line) {
			continue
		}

		start := max(0, i-contextLines)
		end := min(len(lines)-1, i+contextLines)

		var sb strings.Builder
		for j := start; j <= end; j++ {
			if j == i {
				fmt.Fprintf(&sb, ">%6d→%s\n", j+1, lines[j])
			} else {
				fmt.Fprintf(&sb, " %6d→%s\n", j+1, lines[j])
			}
		}
		ctx := strings.TrimSuffix(sb.String(), "\n")

		*matches = append(*matches, contentMatch{
			Path:    rel,
			Line:    i + 1,
			Content: line,
			Context: ctx,
		})

		*totalBytes += len(ctx)

		if len(*matches) >= maxResults || *totalBytes >= maxTotalBytes {
			return filepath.SkipAll
		}
	}

	return nil
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

	absReal, err := filepath.EvalSymlinks(abs)
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

		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil
		}
		if realPath != absReal && !strings.HasPrefix(realPath, absReal+string(filepath.Separator)) {
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
