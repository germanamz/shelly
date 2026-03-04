package codingtoolbox

import (
	"context"
	"sync"
)

// ApprovalOutcome is the result of an approval function.
type ApprovalOutcome struct {
	Err    error
	Shared bool // true = coalesced waiters benefit; false = waiters must retry.
}

// pendingApproval holds the outcome of a single in-flight approval so that
// concurrent callers waiting on the same key can share the result.
type pendingApproval struct {
	done    chan struct{}
	outcome ApprovalOutcome
}

// Approver coalesces concurrent permission checks for the same key.
// Only one goroutine runs the approval function per key; others wait.
type Approver struct {
	mu      sync.Mutex
	pending map[string]*pendingApproval
}

// NewApprover creates an Approver ready for use.
func NewApprover() *Approver {
	return &Approver{pending: make(map[string]*pendingApproval)}
}

// Ensure checks isApproved(); if it returns true the call succeeds immediately.
// Otherwise it runs approveFn with coalescing: if another goroutine is already
// prompting for the same key, the caller waits for that result.
//
// When approveFn returns Shared=false, coalesced waiters call retryFn instead
// of free-riding on the result. This supports exec's one-time "yes" semantic
// where approval of specific args does not transfer to other callers.
// retryFn may be nil when Shared is always true.
func (a *Approver) Ensure(
	ctx context.Context,
	key string,
	isApproved func() bool,
	approveFn func(ctx context.Context) ApprovalOutcome,
	retryFn func(ctx context.Context) error,
) error {
	// Fast path: already approved (no lock contention).
	if isApproved() {
		return nil
	}

	a.mu.Lock()

	// Re-check after acquiring lock (TOCTOU defense).
	if isApproved() {
		a.mu.Unlock()
		return nil
	}

	// If a prompt is already in-flight for this key, wait for its result.
	if pa, ok := a.pending[key]; ok {
		a.mu.Unlock()
		<-pa.done

		if pa.outcome.Err != nil {
			return pa.outcome.Err
		}

		// When the approval is not shared (e.g. one-time "yes"), coalesced
		// waiters must get their own approval.
		if !pa.outcome.Shared && retryFn != nil {
			return retryFn(ctx)
		}

		return nil
	}

	// We are the first — create a pending entry and release the lock.
	pa := &pendingApproval{done: make(chan struct{})}
	a.pending[key] = pa
	a.mu.Unlock()

	// Run the approval function (blocking).
	pa.outcome = approveFn(ctx)

	// Signal waiters and clean up.
	close(pa.done)
	a.mu.Lock()
	delete(a.pending, key)
	a.mu.Unlock()

	return pa.outcome.Err
}
