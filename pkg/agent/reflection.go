package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/germanamz/shelly/pkg/agentctx"
)

// taskSlug extracts a short keyword from a task description for use in instance
// names. It picks the first word that is >= 3 characters long and not a common
// stop word, lowercases it, and truncates to 12 characters. Falls back to "task".
func taskSlug(task string) string {
	stopWords := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "from": {},
		"that": {}, "this": {}, "into": {}, "onto": {}, "over": {},
		"some": {}, "then": {}, "than": {}, "also": {}, "just": {},
	}

	for w := range strings.FieldsSeq(task) {
		w = strings.ToLower(w)
		// Strip non-alphanumeric characters from edges.
		w = strings.TrimFunc(w, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		})
		if len(w) < 3 {
			continue
		}
		if _, stop := stopWords[w]; stop {
			continue
		}
		if len(w) > 12 {
			w = w[:12]
		}
		return w
	}

	return "task"
}

// --- reflection helpers ---

const (
	maxReflectionFiles = 5
	maxReflectionBytes = 32 * 1024
)

// writeReflection writes a reflection note when a sub-agent fails.
// It is best-effort: errors are silently ignored.
func writeReflection(dir string, agentName string, task string, cr *CompletionResult) {
	if dir == "" || cr == nil || cr.Status != "failed" {
		return
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return // best-effort
	}

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339)
	// Use a sanitized filename based on agent name and timestamp.
	safeName := agentctx.SanitizeFilename(agentName + "-" + now.Format("20060102-150405"))
	path := filepath.Join(dir, safeName+".md")

	var b strings.Builder
	fmt.Fprintf(&b, "# Reflection: %s\n\n", agentName)
	fmt.Fprintf(&b, "**Timestamp**: %s\n\n", timestamp)
	fmt.Fprintf(&b, "## Task\n%s\n\n", task)
	fmt.Fprintf(&b, "## Summary\n%s\n\n", cr.Summary)
	if cr.Caveats != "" {
		fmt.Fprintf(&b, "## Caveats\n%s\n\n", cr.Caveats)
	}
	if len(cr.FilesModified) > 0 {
		b.WriteString("## Files Modified\n")
		for _, f := range cr.FilesModified {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}

	os.WriteFile(path, []byte(b.String()), 0o600) //nolint:errcheck,gosec // best-effort reflection
}


// searchReflections searches for relevant reflections before delegating.
// Returns an empty string if no relevant reflections are found.
func searchReflections(dir string, task string) string {
	if dir == "" {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	sort.Slice(entries, func(i, j int) bool {
		fi, errI := entries[i].Info()
		fj, errJ := entries[j].Info()
		if errI != nil && errJ != nil {
			return false
		}
		if errI != nil {
			return false // errored entries sort to the end
		}
		if errJ != nil {
			return true
		}
		return fi.ModTime().After(fj.ModTime())
	})

	var reflections []string
	var totalBytes int
	taskLower := strings.ToLower(task)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // dir is controlled by config, entry names from ReadDir
		if err != nil {
			continue
		}

		content := string(data)
		// Simple relevance: check if any words from the task appear in the reflection.
		if containsRelevantKeywords(taskLower, strings.ToLower(content)) {
			reflections = append(reflections, content)
			totalBytes += len(data)
			if len(reflections) >= maxReflectionFiles || totalBytes >= maxReflectionBytes {
				break
			}
		}
	}

	if len(reflections) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n<prior_reflections>\nPrevious attempts at similar tasks failed. Learn from these reflections:\n\n")
	for _, r := range reflections {
		b.WriteString(r)
		b.WriteString("\n---\n")
	}
	b.WriteString("</prior_reflections>")
	return b.String()
}

// containsRelevantKeywords checks if the content shares significant keywords with the task.
func containsRelevantKeywords(task, content string) bool {
	words := strings.Fields(task)
	matches := 0
	for _, w := range words {
		if len(w) < 4 { // skip short words like "the", "for", etc.
			continue
		}
		if strings.Contains(content, w) {
			matches++
		}
	}
	return matches >= 2 // at least 2 significant word matches
}
