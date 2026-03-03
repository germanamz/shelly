// Package batch provides a batching decorator for the Completer interface.
// It collects concurrent Complete() calls and submits them as a single batch
// to provider-specific batch APIs for cost reduction (typically 50% discount).
//
// The BatchCompleter is transparent to callers — each Complete() call blocks
// until its result is ready, falling back to synchronous completion on error.
package batch

import (
	"context"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Submitter is the provider-specific interface for batch operations.
// Each LLM provider implements this to translate requests into their
// native batch API format.
type Submitter interface {
	// SubmitBatch sends a batch of requests to the provider's batch API.
	// It returns a batch ID for polling and correlation.
	SubmitBatch(ctx context.Context, reqs []Request) (batchID string, err error)

	// PollBatch checks the status of a submitted batch.
	// It returns a map of request ID → Result, a done flag, and any error.
	// When done is false, the caller should poll again after an interval.
	PollBatch(ctx context.Context, batchID string) (results map[string]Result, done bool, err error)

	// CancelBatch attempts to cancel an in-progress batch.
	CancelBatch(ctx context.Context, batchID string) error
}

// Request is a single completion request within a batch.
type Request struct {
	ID    string         // UUID for correlation (maps to provider's custom_id).
	Chat  *chat.Chat     // Conversation to complete.
	Tools []toolbox.Tool // Available tools for this call.
}

// Result is the outcome of a single request within a batch.
type Result struct {
	Message message.Message
	Usage   usage.TokenCount
	Err     error
}

// pendingRequest ties a Request to its result delivery channel.
type pendingRequest struct {
	ctx    context.Context //nolint:containedctx // stored to propagate caller cancellation to fallback
	req    Request
	result chan Result
}
