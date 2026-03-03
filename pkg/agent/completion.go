package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// CompletionResult carries structured completion data from a sub-agent.
// Set by the task_complete tool, read by delegation tools after Run() returns.
type CompletionResult struct {
	Status        string   `json:"status"`                   // "completed" or "failed"
	Summary       string   `json:"summary"`                  // What was done or why it failed.
	FilesModified []string `json:"files_modified,omitempty"` // Files changed.
	TestsRun      []string `json:"tests_run,omitempty"`      // Tests executed.
	Caveats       string   `json:"caveats,omitempty"`        // Known limitations.
}

// completionHandler manages the task_complete tool state for sub-agents.
// It wraps a CompletionResult with a sync.Once guard to ensure at-most-once
// semantics.
type completionHandler struct {
	result *CompletionResult
	once   sync.Once
}

// tool returns the task_complete tool definition. The handler closure captures
// the completionHandler to set the result.
func (ch *completionHandler) tool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "task_complete",
		Description: "Signal task completion with structured metadata. Call this when you have finished your delegated task.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["completed","failed"],"description":"Whether the task was completed successfully or failed"},"summary":{"type":"string","description":"Concise description of what was done or why it failed"},"files_modified":{"type":"array","items":{"type":"string"},"description":"List of files that were modified"},"tests_run":{"type":"array","items":{"type":"string"},"description":"List of tests that were executed"},"caveats":{"type":"string","description":"Known limitations or follow-up work needed"}},"required":["status","summary"]}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var tci taskCompleteInput
			if err := json.Unmarshal(input, &tci); err != nil {
				return "", fmt.Errorf("task_complete: invalid input: %w", err)
			}

			if tci.Status != "completed" && tci.Status != "failed" {
				return "", fmt.Errorf("task_complete: status must be \"completed\" or \"failed\", got %q", tci.Status)
			}

			alreadySet := true
			ch.once.Do(func() {
				alreadySet = false
				ch.result = &CompletionResult{
					Status:        tci.Status,
					Summary:       tci.Summary,
					FilesModified: tci.FilesModified,
					TestsRun:      tci.TestsRun,
					Caveats:       tci.Caveats,
				}
			})

			if alreadySet {
				return "Task already marked — duplicate call ignored.", nil
			}

			return fmt.Sprintf("Task marked as %s.", tci.Status), nil
		},
	}
}

// Result returns the structured completion data, or nil if not yet set.
func (ch *completionHandler) Result() *CompletionResult { return ch.result }

// IsComplete returns true when the task_complete tool has been called.
func (ch *completionHandler) IsComplete() bool { return ch.result != nil }

type taskCompleteInput struct {
	Status        string   `json:"status"`
	Summary       string   `json:"summary"`
	FilesModified []string `json:"files_modified"`
	TestsRun      []string `json:"tests_run"`
	Caveats       string   `json:"caveats"`
}
