package modeladapter

import (
	"context"
	"errors"
	"math"
	"math/rand"
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

// RateLimitedCompleter wraps a Completer with proactive TPM/RPM-based throttling
// and reactive 429 retry with exponential backoff and jitter.
// Input and output tokens are tracked and throttled independently.
type RateLimitedCompleter struct {
	inner           Completer
	mu              sync.Mutex
	completeMu      sync.Mutex
	window          []tokenEntry
	inputTPM        int           // input tokens-per-minute limit (0 = no limit)
	outputTPM       int           // output tokens-per-minute limit (0 = no limit)
	rpm             int           // requests-per-minute limit (0 = no limit)
	maxRetries      int           // max retries on 429
	baseDelay       time.Duration // initial backoff delay
	fallbackTracker usage.Tracker // stable fallback tracker when inner lacks UsageReporter

	// nowFunc is used for testing; defaults to time.Now.
	nowFunc func() time.Time
	// sleepFunc is used for testing; defaults to a context-aware sleep.
	sleepFunc func(ctx context.Context, d time.Duration) error
	// randFunc returns a random float64 in [0,1); used for jitter. Defaults to rand.Float64.
	randFunc func() float64
}

// RateLimitOpts configures the RateLimitedCompleter.
type RateLimitOpts struct {
	InputTPM   int           // Input tokens per minute (0 = no limit).
	OutputTPM  int           // Output tokens per minute (0 = no limit).
	RPM        int           // Requests per minute (0 = no limit).
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
		rpm:        opts.RPM,
		maxRetries: opts.MaxRetries,
		baseDelay:  opts.BaseDelay,
		nowFunc:    time.Now,
		sleepFunc:  contextSleep,
		randFunc:   rand.Float64,
	}
}

// SetNowFunc overrides the time source (for testing).
func (r *RateLimitedCompleter) SetNowFunc(fn func() time.Time) { r.nowFunc = fn }

// SetSleepFunc overrides the sleep function (for testing).
func (r *RateLimitedCompleter) SetSleepFunc(fn func(ctx context.Context, d time.Duration) error) {
	r.sleepFunc = fn
}

// SetRandFunc overrides the random number generator (for testing).
func (r *RateLimitedCompleter) SetRandFunc(fn func() float64) { r.randFunc = fn }

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
		r.window = append(r.window[:0:0], r.window[i:]...)
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

// waitForCapacity blocks until there is capacity in both TPM and RPM windows.
func (r *RateLimitedCompleter) waitForCapacity(ctx context.Context) error {
	if r.inputTPM <= 0 && r.outputTPM <= 0 && r.rpm <= 0 {
		return nil
	}

	for {
		r.mu.Lock()
		now := r.nowFunc()
		r.pruneWindow(now)
		inputTotal, outputTotal := r.windowTotals()

		inputOK := r.inputTPM <= 0 || inputTotal < r.inputTPM
		outputOK := r.outputTPM <= 0 || outputTotal < r.outputTPM
		rpmOK := r.rpm <= 0 || len(r.window) < r.rpm

		if inputOK && outputOK && rpmOK {
			r.mu.Unlock()
			return nil
		}

		// Find when the oldest entry expires to free capacity.
		var waitDur time.Duration
		if len(r.window) > 0 {
			waitDur = max(r.window[0].timestamp.Add(time.Minute).Sub(now), 0)
		}
		r.mu.Unlock()

		const minWait = 10 * time.Millisecond
		if waitDur < minWait {
			waitDur = minWait
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

// jitter applies ±25% random jitter to a duration.
func (r *RateLimitedCompleter) jitter(d time.Duration) time.Duration {
	// Scale factor in [0.75, 1.25).
	factor := 0.75 + r.randFunc()*0.5 //nolint:mnd // jitter range: ±25%
	return time.Duration(float64(d) * factor)
}

// Complete implements Completer with proactive TPM/RPM throttling and 429 retry.
func (r *RateLimitedCompleter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	if err := r.waitForCapacity(ctx); err != nil {
		return message.Message{}, err
	}

	var lastErr error
	for attempt := range r.maxRetries + 1 {
		msg, err := func() (message.Message, error) {
			r.completeMu.Lock()
			defer r.completeMu.Unlock()

			var beforeTotal usage.TokenCount
			if ur, ok := r.inner.(UsageReporter); ok {
				beforeTotal = ur.UsageTracker().Total()
			}

			m, e := r.inner.Complete(ctx, c, tools)
			if e == nil {
				if ur, ok := r.inner.(UsageReporter); ok {
					afterTotal := ur.UsageTracker().Total()
					r.recordTokens(
						afterTotal.InputTokens-beforeTotal.InputTokens,
						afterTotal.OutputTokens-beforeTotal.OutputTokens,
					)
				}
			}

			return m, e
		}()

		if err == nil {
			if sleepErr := r.adaptFromServerInfo(ctx); sleepErr != nil {
				return message.Message{}, sleepErr
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

		// Compute backoff: baseDelay * 2^attempt, but use RetryAfter if larger. Apply jitter.
		backoff := r.jitter(max(
			r.baseDelay*time.Duration(math.Pow(2, float64(attempt))), //nolint:mnd // exponential backoff formula
			rle.RetryAfter,
		))

		if err := r.sleepFunc(ctx, backoff); err != nil {
			return message.Message{}, err
		}
	}

	// Defensive guard: if maxRetries was somehow 0 or negative and the loop
	// never executed, lastErr may be nil. Return a clear error instead of a
	// false success with an empty message.
	if lastErr == nil {
		lastErr = errors.New("rate limit: exhausted retries without a successful completion")
	}

	return message.Message{}, lastErr
}

// adaptFromServerInfo checks whether the inner completer reports near-zero
// remaining capacity via RateLimitInfoReporter. If so, it preemptively sleeps
// until the provider's reset time.
func (r *RateLimitedCompleter) adaptFromServerInfo(ctx context.Context) error {
	reporter, ok := r.inner.(RateLimitInfoReporter)
	if !ok {
		return nil
	}

	info := reporter.LastRateLimitInfo()
	if info == nil {
		return nil
	}

	now := r.nowFunc()
	var sleepUntil time.Time

	if info.RemainingRequests <= 1 && !info.RequestsReset.IsZero() && info.RequestsReset.After(now) {
		sleepUntil = info.RequestsReset
	}

	if info.RemainingTokens <= 1 && !info.TokensReset.IsZero() && info.TokensReset.After(now) {
		if info.TokensReset.After(sleepUntil) {
			sleepUntil = info.TokensReset
		}
	}

	if sleepUntil.IsZero() {
		return nil
	}

	return r.sleepFunc(ctx, sleepUntil.Sub(now))
}

// UsageTracker forwards to the inner completer if it implements UsageReporter.
func (r *RateLimitedCompleter) UsageTracker() *usage.Tracker {
	if ur, ok := r.inner.(UsageReporter); ok {
		return ur.UsageTracker()
	}
	return &r.fallbackTracker
}

// ModelMaxTokens forwards to the inner completer if it implements UsageReporter.
func (r *RateLimitedCompleter) ModelMaxTokens() int {
	if ur, ok := r.inner.(UsageReporter); ok {
		return ur.ModelMaxTokens()
	}
	return 0
}
