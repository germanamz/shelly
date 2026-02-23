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
// implements UsageReporter and ToolAware.
type fakeCompleter struct {
	tracker   usage.Tracker
	maxTokens int
	tools     []toolbox.Tool
	handler   func(ctx context.Context, c *chat.Chat) (message.Message, error)
}

func (f *fakeCompleter) Complete(ctx context.Context, c *chat.Chat) (message.Message, error) {
	return f.handler(ctx, c)
}

func (f *fakeCompleter) UsageTracker() *usage.Tracker  { return &f.tracker }
func (f *fakeCompleter) ModelMaxTokens() int           { return f.maxTokens }
func (f *fakeCompleter) SetTools(tools []toolbox.Tool) { f.tools = tools }

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
	msg, err := rl.Complete(context.Background(), &chat.Chat{})
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

	msg, err := rl.Complete(context.Background(), &chat.Chat{})
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

	_, err := rl.Complete(context.Background(), &chat.Chat{})
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

	_, err := rl.Complete(ctx, &chat.Chat{})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRateLimitedCompleter_TPMThrottling(t *testing.T) {
	callCount := 0
	fc := &fakeCompleter{}
	fc.handler = func(_ context.Context, _ *chat.Chat) (message.Message, error) {
		callCount++
		// Simulate usage: each call uses 100 tokens.
		fc.tracker.Add(usage.TokenCount{InputTokens: 50, OutputTokens: 50})
		return okMessage(), nil
	}

	now := time.Now()
	currentTime := now
	sleepCalled := false

	rl := modeladapter.NewRateLimitedCompleter(fc, modeladapter.RateLimitOpts{
		TPM:        100,
		MaxRetries: 1,
		BaseDelay:  time.Millisecond,
	})
	rl.SetNowFunc(func() time.Time { return currentTime })
	rl.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		sleepCalled = true
		// Advance time past the window.
		currentTime = currentTime.Add(d)
		return nil
	})

	// First call: 100 tokens used, hits the 100 TPM limit.
	_, err := rl.Complete(context.Background(), &chat.Chat{})
	require.NoError(t, err)
	assert.False(t, sleepCalled)

	// Second call: window has 100 tokens (>= TPM), should throttle before calling.
	_, err = rl.Complete(context.Background(), &chat.Chat{})
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

	// ToolAware forwarding.
	tools := []toolbox.Tool{{Name: "test_tool"}}
	rl.SetTools(tools)
	assert.Equal(t, tools, fc.tools)
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

	_, err := rl.Complete(context.Background(), &chat.Chat{})
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

	_, err := rl.Complete(context.Background(), &chat.Chat{})
	require.NoError(t, err)
	// RetryAfter (10s) should be used because it's larger than baseDelay * 2^0 (1s).
	assert.Equal(t, 10*time.Second, sleepDur)
}
