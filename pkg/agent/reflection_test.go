package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchReflectionsCapsFiles(t *testing.T) {
	dir := t.TempDir()

	// Create more than maxReflectionFiles (5) relevant reflection files.
	for i := range 10 {
		name := fmt.Sprintf("agent-%d.md", i)
		body := fmt.Sprintf("# Reflection\n\n## Task\nimplement authentication middleware refactor\n\n## Summary\nfailed %d\n", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}

	result := searchReflections(dir, "implement authentication middleware refactor")
	assert.NotEmpty(t, result)

	// Count how many reflection blocks are in the output (separated by "---").
	count := strings.Count(result, "# Reflection")
	assert.LessOrEqual(t, count, maxReflectionFiles)
}

func TestSearchReflectionsCapsBytes(t *testing.T) {
	dir := t.TempDir()

	// Create a few large reflection files that exceed maxReflectionBytes.
	for i := range 3 {
		name := fmt.Sprintf("agent-%d.md", i)
		// Each file is ~20KB, so 2 files = 40KB which exceeds maxReflectionBytes (32KB).
		body := fmt.Sprintf("# Reflection\n\n## Task\nimplement authentication middleware refactor\n\n## Summary\n%s\n", strings.Repeat("x", 20*1024))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}

	result := searchReflections(dir, "implement authentication middleware refactor")
	assert.NotEmpty(t, result)

	// Should have stopped before reading all 3.
	count := strings.Count(result, "# Reflection")
	assert.LessOrEqual(t, count, 2)
}

func TestTaskSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"refactor the authentication module", "refactor"},
		{"fix bug in parser", "fix"},
		{"the and for with", "task"}, // all stop words
		{"a b", "task"},              // all too short
		{"", "task"},                 // empty
		{"implement superlongkeywordhere", "implement"},
		{"the implementationofthings", "implementati"}, // truncated to 12
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, taskSlug(tt.input))
		})
	}
}
