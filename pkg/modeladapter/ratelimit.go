package modeladapter

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

type tokenEntry struct {
	timestamp time.Time
	tokens    int
}

// RateLimitedCompleter wraps a Completer with proactive TPM-based throttling
// and reactive 429 retry with exponential backoff.
type RateLimitedCompleter struct {
	inner      Completer
	mu         sync.Mutex
	window     []tokenEntry
	tpm        int           // tokens-per-minute limit (0 = no proactive limiting)
	maxRetries int           // max retries on 429
	baseDelay  time.Duration // initial backoff delay

	// nowFunc is used for testing; defaults to time.Now.
	nowFunc func() time.Time
	// sleepFunc is used for testing; defaults to a context-aware sleep.
	sleepFunc func(ctx context.Context, d time.Duration) error
}

// RateLimitOpts configures the RateLimitedCompleter.
type RateLimitOpts struct {
	TPM        int           // Tokens per minute (0 = no proactive limit).
	MaxRetries int           // Max retries on 429 (default 3).
	BaseDelay  time.Duration // Initial backoff delay (default 1s).
}

// NewRateLimitedCompleter wraps a Completer with rate limiting.
func NewRateLimitedCompleter(inner Completer, opts RateLimitOpts) *RateLimitedCompleter {
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.BaseDelay <= 0 {
		opts.BaseDelay = time.Second
	}

	return &RateLimitedCompleter{
		inner:      inner,
		tpm:        opts.TPM,
		maxRetries: opts.MaxRetries,
		baseDelay:  opts.BaseDelay,
		nowFunc:    time.Now,
		sleepFunc:  contextSleep,
	}
}

// SetNowFunc overrides the time source (for testing).
func (r *RateLimitedCompleter) SetNowFunc(fn func() time.Time) { r.nowFunc = fn }

// SetSleepFunc overrides the sleep function (for testing).
func (r *RateLimitedCompleter) SetSleepFunc(fn func(ctx context.Context, d time.Duration) error) {
	r.sleepFunc = fn
}

// contextSleep sleeps for d or until ctx is cancelled.
func contextSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// pruneWindow removes entries older than 1 minute. Must be called with mu held.
func (r *RateLimitedCompleter) pruneWindow(now time.Time) {
	cutoff := now.Add(-time.Minute)
	i := 0
	for i < len(r.window) && !r.window[i].timestamp.After(cutoff) {
		i++
	}
	if i > 0 {
		r.window = r.window[i:]
	}
}

// windowTotal returns the sum of tokens in the current window. Must be called with mu held.
func (r *RateLimitedCompleter) windowTotal() int {
	total := 0
	for _, e := range r.window {
		total += e.tokens
	}
	return total
}

// waitForCapacity blocks until there is capacity in the TPM window.
func (r *RateLimitedCompleter) waitForCapacity(ctx context.Context) error {
	if r.tpm <= 0 {
		return nil
	}

	for {
		r.mu.Lock()
		now := r.nowFunc()
		r.pruneWindow(now)
		total := r.windowTotal()

		if total < r.tpm {
			r.mu.Unlock()
			return nil
		}

		// Find when the oldest entry expires to free capacity.
		var waitDur time.Duration
		if len(r.window) > 0 {
			waitDur = max(r.window[0].timestamp.Add(time.Minute).Sub(now), 0)
		}
		r.mu.Unlock()

		if waitDur == 0 {
			continue
		}

		if err := r.sleepFunc(ctx, waitDur); err != nil {
			return err
		}
	}
}

// recordTokens adds a token entry to the sliding window.
func (r *RateLimitedCompleter) recordTokens(tokens int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.window = append(r.window, tokenEntry{
		timestamp: r.nowFunc(),
		tokens:    tokens,
	})
}

// Complete implements Completer with proactive TPM throttling and 429 retry.
func (r *RateLimitedCompleter) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
	if err := r.waitForCapacity(ctx); err != nil {
		return message.Message{}, err
	}

	var lastErr error
	for attempt := range r.maxRetries + 1 {
		msg, err := r.inner.Complete(ctx, c)
		if err == nil {
			// Record token usage from the inner completer's tracker.
			if ur, ok := r.inner.(UsageReporter); ok {
				if tc, found := ur.UsageTracker().Last(); found {
					r.recordTokens(tc.Total())
				}
			}
			return msg, nil
		}

		var rle *RateLimitError
		if !errors.As(err, &rle) {
			return message.Message{}, err
		}

		lastErr = err

		if attempt >= r.maxRetries {
			break
		}

		// Compute backoff: baseDelay * 2^attempt, but use RetryAfter if larger.
		backoff := max(
			r.baseDelay*time.Duration(math.Pow(2, float64(attempt))), //nolint:mnd // exponential backoff formula
			rle.RetryAfter,
		)

		if err := r.sleepFunc(ctx, backoff); err != nil {
			return message.Message{}, err
		}
	}

	return message.Message{}, lastErr
}

// SetTools forwards to the inner completer if it implements ToolAware.
func (r *RateLimitedCompleter) SetTools(tools []toolbox.Tool) {
	if ta, ok := r.inner.(ToolAware); ok {
		ta.SetTools(tools)
	}
}

// UsageTracker forwards to the inner completer if it implements UsageReporter.
func (r *RateLimitedCompleter) UsageTracker() *usage.Tracker {
	if ur, ok := r.inner.(UsageReporter); ok {
		return ur.UsageTracker()
	}
	return &usage.Tracker{}
}

// ModelMaxTokens forwards to the inner completer if it implements UsageReporter.
func (r *RateLimitedCompleter) ModelMaxTokens() int {
	if ur, ok := r.inner.(UsageReporter); ok {
		return ur.ModelMaxTokens()
	}
	return 0
}
