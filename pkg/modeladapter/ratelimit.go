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

var _ Completer = (*RateLimitedCompleter)(nil)

type tokenEntry struct {
	timestamp    time.Time
	inputTokens  int
	outputTokens int
}

// RateLimitedCompleter wraps a Completer with proactive TPM-based throttling
// and reactive 429 retry with exponential backoff.
// Input and output tokens are tracked and throttled independently.
type RateLimitedCompleter struct {
	inner      Completer
	mu         sync.Mutex
	window     []tokenEntry
	inputTPM   int           // input tokens-per-minute limit (0 = no limit)
	outputTPM  int           // output tokens-per-minute limit (0 = no limit)
	maxRetries int           // max retries on 429
	baseDelay  time.Duration // initial backoff delay

	// nowFunc is used for testing; defaults to time.Now.
	nowFunc func() time.Time
	// sleepFunc is used for testing; defaults to a context-aware sleep.
	sleepFunc func(ctx context.Context, d time.Duration) error
}

// RateLimitOpts configures the RateLimitedCompleter.
type RateLimitOpts struct {
	InputTPM   int           // Input tokens per minute (0 = no limit).
	OutputTPM  int           // Output tokens per minute (0 = no limit).
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
		inputTPM:   opts.InputTPM,
		outputTPM:  opts.OutputTPM,
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

// windowTotals returns the sum of input and output tokens in the current window.
// Must be called with mu held.
func (r *RateLimitedCompleter) windowTotals() (inputTotal, outputTotal int) {
	for _, e := range r.window {
		inputTotal += e.inputTokens
		outputTotal += e.outputTokens
	}
	return inputTotal, outputTotal
}

// waitForCapacity blocks until there is capacity in both TPM windows.
func (r *RateLimitedCompleter) waitForCapacity(ctx context.Context) error {
	if r.inputTPM <= 0 && r.outputTPM <= 0 {
		return nil
	}

	for {
		r.mu.Lock()
		now := r.nowFunc()
		r.pruneWindow(now)
		inputTotal, outputTotal := r.windowTotals()

		inputOK := r.inputTPM <= 0 || inputTotal < r.inputTPM
		outputOK := r.outputTPM <= 0 || outputTotal < r.outputTPM

		if inputOK && outputOK {
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
func (r *RateLimitedCompleter) recordTokens(inputTokens, outputTokens int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.window = append(r.window, tokenEntry{
		timestamp:    r.nowFunc(),
		inputTokens:  inputTokens,
		outputTokens: outputTokens,
	})
}

// Complete implements Completer with proactive TPM throttling and 429 retry.
func (r *RateLimitedCompleter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	if err := r.waitForCapacity(ctx); err != nil {
		return message.Message{}, err
	}

	var lastErr error
	for attempt := range r.maxRetries + 1 {
		msg, err := r.inner.Complete(ctx, c, tools)
		if err == nil {
			// Record token usage from the inner completer's tracker.
			if ur, ok := r.inner.(UsageReporter); ok {
				if tc, found := ur.UsageTracker().Last(); found {
					r.recordTokens(tc.InputTokens, tc.OutputTokens)
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
