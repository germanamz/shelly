package batch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Ensure Collector satisfies Completer and UsageReporter at compile time.
var (
	_ modeladapter.Completer     = (*Collector)(nil)
	_ modeladapter.UsageReporter = (*Collector)(nil)
)

// CollectorOpts configures a Collector.
type CollectorOpts struct {
	CollectWindow time.Duration // How long to wait for more requests before flushing (default 500ms).
	PollInterval  time.Duration // How often to poll batch status (default 5s).
	Timeout       time.Duration // Maximum time to wait for a batch result (default 1h).
	MaxBatchSize  int           // Maximum requests per batch before auto-flush (default 100).
}

func (o *CollectorOpts) withDefaults() CollectorOpts {
	out := *o
	if out.CollectWindow <= 0 {
		out.CollectWindow = 500 * time.Millisecond
	}
	if out.PollInterval <= 0 {
		out.PollInterval = 5 * time.Second
	}
	if out.Timeout <= 0 {
		out.Timeout = time.Hour
	}
	if out.MaxBatchSize <= 0 {
		out.MaxBatchSize = 100
	}
	return out
}

// EventHandler is called when batch lifecycle events occur.
// Implementations must be safe for concurrent use and should not block.
type EventHandler func(kind EventKind, data map[string]any)

// EventKind identifies the type of batch event.
type EventKind string

const (
	EventSubmitted EventKind = "batch_submitted"
	EventPolling   EventKind = "batch_polling"
	EventCompleted EventKind = "batch_completed"
	EventFallback  EventKind = "batch_fallback"
)

// Collector accumulates concurrent Complete() calls and flushes them
// as a single batch submission. It is shared per provider so that
// concurrent sub-agents using the same provider naturally batch together.
//
// Each Complete() call blocks until its result is delivered from the batch,
// or falls back to synchronous completion on timeout/error.
//
// Call Stop() to cancel in-flight batch operations and release resources.
type Collector struct {
	submitter Submitter
	inner     modeladapter.Completer // sync fallback
	opts      CollectorOpts
	onEvent   atomic.Pointer[EventHandler]

	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc

	mu         sync.Mutex
	pending    []pendingRequest
	timer      *time.Timer
	tracker    usage.Tracker // aggregated usage from batch results
	fallbackMu sync.Mutex    // serializes fallbackSingle calls to avoid usage tracker races

	// Test hooks.
	nowFunc   func() time.Time
	sleepFunc func(ctx context.Context, d time.Duration) error
	uuidFunc  func() string
}

// NewCollector creates a Collector that wraps inner with batch support.
// The submitter handles provider-specific batch API calls.
// The inner completer is used as a synchronous fallback on batch errors.
func NewCollector(inner modeladapter.Completer, submitter Submitter, opts CollectorOpts) *Collector {
	opts = opts.withDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		submitter:       submitter,
		inner:           inner,
		opts:            opts,
		lifecycleCtx:    ctx,
		lifecycleCancel: cancel,
		nowFunc:         time.Now,
		sleepFunc:       modeladapter.ContextSleep,
		uuidFunc:        func() string { return uuid.New().String() },
	}
}

// Stop cancels all in-flight batch operations. It should be called when the
// engine shuts down to prevent orphaned goroutines from continuing to poll
// provider APIs.
func (c *Collector) Stop() {
	c.lifecycleCancel()
}

// SetEventHandler registers a callback for batch lifecycle events.
// It is safe to call concurrently.
func (c *Collector) SetEventHandler(h EventHandler) {
	c.onEvent.Store(&h)
}

// SetNowFunc overrides the time source (for testing).
func (c *Collector) SetNowFunc(fn func() time.Time) { c.nowFunc = fn }

// SetSleepFunc overrides the sleep function (for testing).
func (c *Collector) SetSleepFunc(fn func(ctx context.Context, d time.Duration) error) {
	c.sleepFunc = fn
}

// SetUUIDFunc overrides the UUID generator (for testing).
func (c *Collector) SetUUIDFunc(fn func() string) { c.uuidFunc = fn }

// Complete implements modeladapter.Completer. It enqueues the request into
// the pending batch and blocks until the result is available.
func (c *Collector) Complete(ctx context.Context, ch *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	resultCh := make(chan Result, 1)

	req := pendingRequest{
		ctx: ctx,
		req: Request{
			ID:    c.uuidFunc(),
			Chat:  ch,
			Tools: tools,
		},
		result: resultCh,
	}

	c.enqueue(req)

	select {
	case <-ctx.Done():
		return message.Message{}, ctx.Err()
	case res := <-resultCh:
		if res.Err != nil {
			return message.Message{}, res.Err
		}
		c.tracker.Add(res.Usage)
		return res.Message, nil
	}
}

// UsageTracker returns the collector's aggregated usage tracker.
func (c *Collector) UsageTracker() *usage.Tracker {
	return &c.tracker
}

// ModelMaxTokens forwards to the inner completer if it implements UsageReporter.
func (c *Collector) ModelMaxTokens() int {
	if ur, ok := c.inner.(modeladapter.UsageReporter); ok {
		return ur.ModelMaxTokens()
	}
	return 0
}

// enqueue adds a request to the pending queue and starts the collect window
// timer if it's not already running. If the queue hits max size, it flushes
// immediately.
func (c *Collector) enqueue(pr pendingRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pending = append(c.pending, pr)

	if len(c.pending) >= c.opts.MaxBatchSize {
		batch := c.drainLocked()
		go c.flushBatch(batch)
		return
	}

	if c.timer == nil {
		c.timer = time.AfterFunc(c.opts.CollectWindow, c.onTimerFire)
	}
}

// onTimerFire is called when the collect window expires.
func (c *Collector) onTimerFire() {
	c.mu.Lock()
	batch := c.drainLocked()
	c.mu.Unlock()

	if len(batch) > 0 {
		c.flushBatch(batch)
	}
}

// drainLocked removes and returns all pending requests. Must be called with mu held.
func (c *Collector) drainLocked() []pendingRequest {
	batch := c.pending
	c.pending = nil
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	return batch
}

// flushBatch submits a batch and polls for results. On any error,
// it falls back to synchronous completion for all requests in the batch.
func (c *Collector) flushBatch(batch []pendingRequest) {
	ctx, cancel := context.WithTimeout(c.lifecycleCtx, c.opts.Timeout)
	defer cancel()

	reqs := make([]Request, len(batch))
	for i, pr := range batch {
		reqs[i] = pr.req
	}

	c.emitEvent(EventSubmitted, map[string]any{
		"count": len(reqs),
	})

	batchID, err := c.submitter.SubmitBatch(ctx, reqs)
	if err != nil {
		c.emitEvent(EventFallback, map[string]any{
			"reason": "submit_error",
			"error":  err.Error(),
			"count":  len(reqs),
		})
		c.fallbackSync(batch)
		return
	}

	c.emitEvent(EventPolling, map[string]any{
		"batch_id": batchID,
		"count":    len(reqs),
	})

	results, err := c.pollUntilDone(ctx, batchID)
	if err != nil {
		c.emitEvent(EventFallback, map[string]any{
			"reason":   "poll_error",
			"error":    err.Error(),
			"batch_id": batchID,
			"count":    len(reqs),
		})
		c.fallbackSync(batch)
		return
	}

	c.emitEvent(EventCompleted, map[string]any{
		"batch_id": batchID,
		"count":    len(results),
	})

	c.deliverResults(batch, results)
}

// pollUntilDone polls the batch until all results are ready or timeout.
func (c *Collector) pollUntilDone(ctx context.Context, batchID string) (map[string]Result, error) {
	for {
		results, done, err := c.submitter.PollBatch(ctx, batchID)
		if err != nil {
			return nil, fmt.Errorf("poll batch %s: %w", batchID, err)
		}
		if done {
			return results, nil
		}

		if err := c.sleepFunc(ctx, c.opts.PollInterval); err != nil {
			// Context cancelled — attempt to cancel the batch.
			cancelCtx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
			_ = c.submitter.CancelBatch(cancelCtx, batchID)
			cancelFn()
			return nil, err
		}
	}
}

// deliverResults sends batch results to each request's channel.
// If a request's result is missing, it falls back to sync for that request.
func (c *Collector) deliverResults(batch []pendingRequest, results map[string]Result) {
	for _, pr := range batch {
		res, ok := results[pr.req.ID]
		if !ok {
			// Result missing for this request — fall back to sync.
			go c.fallbackSingle(pr)
			continue
		}
		pr.result <- res
	}
}

// fallbackSync runs all batch requests synchronously using the inner completer.
// Each request is dispatched in its own goroutine; results are delivered through
// the per-request channels so there is no need to wait for completion here.
func (c *Collector) fallbackSync(batch []pendingRequest) {
	for _, pr := range batch {
		go c.fallbackSingle(pr)
	}
}

// fallbackSingle runs a single request synchronously using the inner completer.
// It serializes access to the inner completer so that UsageTracker().Last()
// returns the correct usage for this specific call.
func (c *Collector) fallbackSingle(pr pendingRequest) {
	ctx := pr.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	c.fallbackMu.Lock()
	msg, err := c.inner.Complete(ctx, pr.req.Chat, pr.req.Tools)
	var tc usage.TokenCount
	if err == nil {
		if ur, ok := c.inner.(modeladapter.UsageReporter); ok {
			tc, _ = ur.UsageTracker().Last()
		}
	}
	c.fallbackMu.Unlock()

	if err != nil {
		pr.result <- Result{Err: err}
		return
	}
	pr.result <- Result{Message: msg, Usage: tc}
}

// emitEvent fires an event if a handler is registered.
func (c *Collector) emitEvent(kind EventKind, data map[string]any) {
	if h := c.onEvent.Load(); h != nil {
		(*h)(kind, data)
	}
}
