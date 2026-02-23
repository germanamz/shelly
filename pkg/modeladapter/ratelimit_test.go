package modeladapter_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCompleter is a test double for modeladapter.Completer that also
// implements UsageReporter and RateLimitInfoReporter.
type fakeCompleter struct {
	tracker       usage.Tracker
	maxTokens     int
	handler       func(ctx context.Context, c *chat.Chat) (message.Message, error)
	rateLimitInfo *modeladapter.RateLimitInfo
}

func (f *fakeCompleter) Complete(ctx context.Context, c *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	return f.handler(ctx, c)
}

func (f *fakeCompleter) UsageTracker() *usage.Tracker                   { return &f.tracker }
func (f *fakeCompleter) ModelMaxTokens() int                            { return f.maxTokens }
func (f *fakeCompleter) LastRateLimitInfo() *modeladapter.RateLimitInfo { return f.rateLimitInfo }

func okMessage() message.Message {
	return message.Message{Role: role.Assistant}
}

func TestRateLimitedCompleter_PassthroughOnSuccess(t *testing.T) {
	fc := &fakeCompleter{
		maxTokens: 4096,
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			return okMessage(), nil
		},
	}

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{})
	msg, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.Equal(t, role.Assistant, msg.Role)
}

func TestRateLimitedCompleter_RetryOn429(t *testing.T) {
	var calls atomic.Int32
	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			if calls.Add(1) <= 2 {
				return message.Message{}, &modeladapter.RateLimitError{Body: "slow down"}
			}
			return okMessage(), nil
		},
	}

	sleeps := 0
	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		MaxRetries: 3,
		BaseDelay:  time.Millisecond,
	})
	rl.SetSleepFunc(func(_ context.Context, _ time.Duration) error {
		sleeps++
		return nil
	})
	rl.SetRandFunc(func() float64 { return 0.5 }) // zero jitter

	msg, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.Equal(t, role.Assistant, msg.Role)
	assert.Equal(t, int32(3), calls.Load())
	assert.Equal(t, 2, sleeps)
}

func TestRateLimitedCompleter_MaxRetriesExhausted(t *testing.T) {
	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			return message.Message{}, &modeladapter.RateLimitError{Body: "overloaded"}
		},
	}

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		MaxRetries: 2,
		BaseDelay:  time.Millisecond,
	})
	rl.SetSleepFunc(func(_ context.Context, _ time.Duration) error { return nil })

	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.Error(t, err)

	var rle *modeladapter.RateLimitError
	require.ErrorAs(t, err, &rle)
	assert.Equal(t, "overloaded", rle.Body)
}

func TestRateLimitedCompleter_ContextCancellation(t *testing.T) {
	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			return message.Message{}, &modeladapter.RateLimitError{Body: "wait"}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		MaxRetries: 5,
		BaseDelay:  time.Millisecond,
	})
	rl.SetSleepFunc(func(_ context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	})

	_, err := rl.Complete(ctx, &chat.Chat{}, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRateLimitedCompleter_InputTPMThrottling(t *testing.T) {
	fc := &fakeCompleter{}
	fc.handler = func(_ context.Context, _ *chat.Chat) (message.Message, error) {
		fc.tracker.Add(usage.TokenCount{InputTokens: 80, OutputTokens: 20})
		return okMessage(), nil
	}

	now := time.Now()
	currentTime := now
	sleepCalled := false

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		InputTPM:   80, // exactly matches per-call input usage
		MaxRetries: 1,
		BaseDelay:  time.Millisecond,
	})
	rl.SetNowFunc(func() time.Time { return currentTime })
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepCalled = true
		currentTime = currentTime.Add(d)
		return nil
	})

	// First call: 80 input tokens used, hits the 80 input TPM limit.
	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.False(t, sleepCalled)

	// Second call: window has 80 input tokens (>= input TPM), should throttle.
	_, err = rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.True(t, sleepCalled)
}

func TestRateLimitedCompleter_OutputTPMThrottling(t *testing.T) {
	fc := &fakeCompleter{}
	fc.handler = func(_ context.Context, _ *chat.Chat) (message.Message, error) {
		fc.tracker.Add(usage.TokenCount{InputTokens: 20, OutputTokens: 80})
		return okMessage(), nil
	}

	now := time.Now()
	currentTime := now
	sleepCalled := false

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		OutputTPM:  80, // exactly matches per-call output usage
		MaxRetries: 1,
		BaseDelay:  time.Millisecond,
	})
	rl.SetNowFunc(func() time.Time { return currentTime })
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepCalled = true
		currentTime = currentTime.Add(d)
		return nil
	})

	// First call: 80 output tokens used, hits the 80 output TPM limit.
	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.False(t, sleepCalled)

	// Second call: window has 80 output tokens (>= output TPM), should throttle.
	_, err = rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.True(t, sleepCalled)
}

func TestRateLimitedCompleter_IndependentLimits(t *testing.T) {
	fc := &fakeCompleter{}
	fc.handler = func(_ context.Context, _ *chat.Chat) (message.Message, error) {
		// High input, low output.
		fc.tracker.Add(usage.TokenCount{InputTokens: 90, OutputTokens: 10})
		return okMessage(), nil
	}

	now := time.Now()
	currentTime := now
	sleepCalled := false

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		InputTPM:   90,  // exactly matches per-call input usage
		OutputTPM:  200, // output limit is generous
		MaxRetries: 1,
		BaseDelay:  time.Millisecond,
	})
	rl.SetNowFunc(func() time.Time { return currentTime })
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepCalled = true
		currentTime = currentTime.Add(d)
		return nil
	})

	// First call: 90 input, 10 output — hits input limit but output is fine.
	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.False(t, sleepCalled)

	// Second call: input at 90 (>= 90 limit), should throttle even though output (10) is well under 200.
	_, err = rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.True(t, sleepCalled)
}

func TestRateLimitedCompleter_InterfaceForwarding(t *testing.T) {
	fc := &fakeCompleter{
		maxTokens: 8192,
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			return okMessage(), nil
		},
	}

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{})

	// UsageReporter forwarding.
	assert.Equal(t, 8192, rl.ModelMaxTokens())
	assert.Same(t, fc.UsageTracker(), rl.UsageTracker())
}

func TestRateLimitedCompleter_NonRateLimitErrorNotRetried(t *testing.T) {
	var calls int
	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			calls++
			return message.Message{}, assert.AnError
		},
	}

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		MaxRetries: 3,
		BaseDelay:  time.Millisecond,
	})

	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 1, calls, "non-rate-limit errors should not be retried")
}

func TestRateLimitedCompleter_RetryAfterUsed(t *testing.T) {
	var calls atomic.Int32
	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			if calls.Add(1) <= 1 {
				return message.Message{}, &modeladapter.RateLimitError{
					RetryAfter: 10 * time.Second,
					Body:       "slow",
				}
			}
			return okMessage(), nil
		},
	}

	var sleepDur time.Duration
	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		MaxRetries: 2,
		BaseDelay:  time.Second,
	})
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepDur = d
		return nil
	})
	rl.SetRandFunc(func() float64 { return 0.5 }) // zero jitter (factor = 1.0)

	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	// RetryAfter (10s) should be used because it's larger than baseDelay * 2^0 (1s).
	assert.Equal(t, 10*time.Second, sleepDur)
}

func TestRateLimitedCompleter_RPMThrottling(t *testing.T) {
	fc := &fakeCompleter{}
	fc.handler = func(_ context.Context, _ *chat.Chat) (message.Message, error) {
		fc.tracker.Add(usage.TokenCount{InputTokens: 10, OutputTokens: 10})
		return okMessage(), nil
	}

	now := time.Now()
	currentTime := now
	sleepCalled := false

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		RPM:        1, // only 1 request per minute
		MaxRetries: 1,
		BaseDelay:  time.Millisecond,
	})
	rl.SetNowFunc(func() time.Time { return currentTime })
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepCalled = true
		currentTime = currentTime.Add(d)
		return nil
	})

	// First call succeeds without throttling.
	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.False(t, sleepCalled)

	// Second call: window has 1 entry (>= RPM of 1), should throttle.
	_, err = rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.True(t, sleepCalled)
}

func TestRateLimitedCompleter_RPMAndTPMCombined(t *testing.T) {
	fc := &fakeCompleter{}
	fc.handler = func(_ context.Context, _ *chat.Chat) (message.Message, error) {
		fc.tracker.Add(usage.TokenCount{InputTokens: 10, OutputTokens: 5})
		return okMessage(), nil
	}

	now := time.Now()
	currentTime := now
	sleepCount := 0

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		RPM:        2,   // allow 2 requests per minute
		InputTPM:   100, // generous TPM limit
		MaxRetries: 1,
		BaseDelay:  time.Millisecond,
	})
	rl.SetNowFunc(func() time.Time { return currentTime })
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepCount++
		currentTime = currentTime.Add(d)
		return nil
	})

	// Two calls should succeed without throttling.
	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	_, err = rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, sleepCount)

	// Third call hits RPM limit (2 entries in window >= RPM of 2).
	_, err = rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, sleepCount)
}

func TestRateLimitedCompleter_BackoffJitter(t *testing.T) {
	var calls atomic.Int32
	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			if calls.Add(1) <= 1 {
				return message.Message{}, &modeladapter.RateLimitError{Body: "slow"}
			}
			return okMessage(), nil
		},
	}

	var sleepDur time.Duration
	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		MaxRetries: 2,
		BaseDelay:  time.Second,
	})
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepDur = d
		return nil
	})
	// randFunc returning 0.0 → factor = 0.75 (minimum jitter)
	rl.SetRandFunc(func() float64 { return 0.0 })

	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	// Base backoff for attempt 0: 1s * 2^0 = 1s. Jitter factor 0.75 → 750ms.
	assert.Equal(t, 750*time.Millisecond, sleepDur)
}

func TestRateLimitedCompleter_AdaptiveThrottle_LowRemaining(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	resetTime := now.Add(10 * time.Second)

	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			return okMessage(), nil
		},
		rateLimitInfo: &modeladapter.RateLimitInfo{
			RemainingRequests: 0,
			RequestsReset:     resetTime,
			RemainingTokens:   500,
			TokensReset:       time.Time{},
		},
	}

	var sleepDur time.Duration
	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{})
	rl.SetNowFunc(func() time.Time { return now })
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepDur = d
		return nil
	})

	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	// Should sleep until reset time (10 seconds from now).
	assert.Equal(t, 10*time.Second, sleepDur)
}

func TestRateLimitedCompleter_AdaptiveThrottle_NotTriggered(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	fc := &fakeCompleter{
		handler: func(_ context.Context, _ *chat.Chat) (message.Message, error) {
			return okMessage(), nil
		},
		rateLimitInfo: &modeladapter.RateLimitInfo{
			RemainingRequests: 50,
			RemainingTokens:   5000,
		},
	}

	sleepCalled := false
	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{})
	rl.SetNowFunc(func() time.Time { return now })
	rl.SetSleepFunc(func(_ context.Context, _ time.Duration) error {
		sleepCalled = true
		return nil
	})

	_, err := rl.Complete(context.Background(), &chat.Chat{}, nil)
	require.NoError(t, err)
	assert.False(t, sleepCalled, "adaptive throttle should not trigger with plenty of remaining capacity")
}
