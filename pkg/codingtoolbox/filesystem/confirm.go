package filesystem

import (
	"context"
	"fmt"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// NotifyFunc is a non-blocking callback for displaying file changes when the
// session is trusted. It should not block for user input.
type NotifyFunc func(ctx context.Context, message string)

// computeDiff returns a unified diff between oldContent and newContent labeled
// with the given path. Returns an empty string when the contents are equal.
func computeDiff(path, oldContent, newContent string) string {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: path,
		ToFile:   path,
		Context:  3,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return fmt.Sprintf("(diff error: %v)", err)
	}

	return result
}

// confirmChange asks the user for approval before applying a file change. If
// the session is already trusted, it notifies the user of the change without
// blocking. The caller must provide a precomputed diff string.
func (f *FS) confirmChange(ctx context.Context, path, diff string) error {
	st := sessionTrustFromContext(ctx)

	if st != nil && st.IsTrusted() {
		if f.notify != nil {
			f.notify(ctx, fmt.Sprintf("File change: %s\n%s", path, diff))
		}
		return nil
	}

	question := fmt.Sprintf("Apply changes to %s?\n\n%s", path, diff)
	resp, err := f.ask(ctx, question, []string{"yes", "no", "trust this session"})
	if err != nil {
		return fmt.Errorf("confirm change: %w", err)
	}

	switch strings.ToLower(resp) {
	case "yes":
		return nil
	case "trust this session":
		if st != nil {
			st.Trust()
		}
		return nil
	default:
		return fmt.Errorf("file change denied for %s", path)
	}
}
