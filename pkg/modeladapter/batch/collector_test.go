package batch_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSubmitter implements batch.Submitter for testing.
type mockSubmitter struct {
	mu             sync.Mutex
	batches        map[string][]batch.Request
	results        map[string]map[string]batch.Result
	pollsUntilDone int // How many polls before reporting done.
	pollCount      atomic.Int32
	submitErr      error
	pollErr        error
	cancelCalled   atomic.Bool
}

func newMockSubmitter() *mockSubmitter {
	return &mockSubmitter{
		batches:        make(map[string][]batch.Request),
		results:        make(map[string]map[string]batch.Result),
		pollsUntilDone: 1, // default: done on first poll
	}
}

func (m *mockSubmitter) SubmitBatch(_ context.Context, reqs []batch.Request) (string, error) {
	if m.submitErr != nil {
		return "", m.submitErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	batchID := fmt.Sprintf("batch-%d", len(m.batches)+1)
	m.batches[batchID] = reqs

	// Auto-generate success results for each request.
	results := make(map[string]batch.Result, len(reqs))
	for _, r := range reqs {
		results[r.ID] = batch.Result{
			Message: message.NewText("", role.Assistant, "batch response for "+r.ID),
			Usage:   usage.TokenCount{InputTokens: 10, OutputTokens: 5},
		}
	}
	m.results[batchID] = results

	return batchID, nil
}

func (m *mockSubmitter) PollBatch(_ context.Context, batchID string) (map[string]batch.Result, bool, error) {
	if m.pollErr != nil {
		return nil, false, m.pollErr
	}
	count := int(m.pollCount.Add(1))
	if count < m.pollsUntilDone {
		return nil, false, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	results := m.results[batchID]
	return results, true, nil
}

func (m *mockSubmitter) CancelBatch(_ context.Context, _ string) error {
	m.cancelCalled.Store(true)
	return nil
}

func (m *mockSubmitter) batchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.batches)
}

// mockCompleter is a sync fallback completer for testing.
type mockCompleter struct {
	calls   atomic.Int32
	tracker usage.Tracker
}

func (m *mockCompleter) Complete(_ context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	m.calls.Add(1)
	m.tracker.Add(usage.TokenCount{InputTokens: 20, OutputTokens: 10})
	return message.NewText("", role.Assistant, "sync fallback"), nil
}

func (m *mockCompleter) UsageTracker() *usage.Tracker { return &m.tracker }
func (m *mockCompleter) ModelMaxTokens() int          { return 4096 }

func TestCollector_SingleRequest(t *testing.T) {
	sub := newMockSubmitter()
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: 10 * time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	ch := chat.New(message.NewText("", role.User, "hello"))
	msg, err := c.Complete(context.Background(), ch, nil)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, msg.Role)
	assert.Contains(t, msg.TextContent(), "batch response")
	assert.Equal(t, 1, sub.batchCount())
	assert.Equal(t, int32(0), inner.calls.Load(), "should not fall back to sync")
}

func TestCollector_ConcurrentRequests_BatchTogether(t *testing.T) {
	sub := newMockSubmitter()
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: 50 * time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	var wg sync.WaitGroup
	results := make([]message.Message, 3)
	errs := make([]error, 3)

	for i := range 3 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch := chat.New(message.NewText("", role.User, fmt.Sprintf("request %d", idx)))
			results[idx], errs[idx] = c.Complete(context.Background(), ch, nil)
		}(i)
	}

	wg.Wait()

	for i := range 3 {
		require.NoError(t, errs[i], "request %d", i)
		assert.Equal(t, role.Assistant, results[i].Role)
	}

	// All 3 should be in one batch.
	assert.Equal(t, 1, sub.batchCount(), "concurrent requests should batch together")
}

func TestCollector_MaxBatchSize_FlushesEarly(t *testing.T) {
	sub := newMockSubmitter()
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: time.Minute, // Very long window.
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
		MaxBatchSize:  2,
	})

	var wg sync.WaitGroup
	errs := make([]error, 2)

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch := chat.New(message.NewText("", role.User, "hello"))
			_, errs[idx] = c.Complete(context.Background(), ch, nil)
		}(i)
		// Small delay to ensure both enqueue before the timer would fire.
		time.Sleep(time.Millisecond)
	}

	wg.Wait()

	for i := range 2 {
		require.NoError(t, errs[i])
	}
	assert.Equal(t, 1, sub.batchCount())
}

func TestCollector_SubmitError_FallsBackToSync(t *testing.T) {
	sub := newMockSubmitter()
	sub.submitErr = fmt.Errorf("batch API unavailable")
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: 10 * time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	ch := chat.New(message.NewText("", role.User, "hello"))
	msg, err := c.Complete(context.Background(), ch, nil)

	require.NoError(t, err)
	assert.Equal(t, "sync fallback", msg.TextContent())
	assert.Equal(t, int32(1), inner.calls.Load())
}

func TestCollector_PollError_FallsBackToSync(t *testing.T) {
	sub := newMockSubmitter()
	sub.pollErr = fmt.Errorf("poll failed")
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: 10 * time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	ch := chat.New(message.NewText("", role.User, "hello"))
	msg, err := c.Complete(context.Background(), ch, nil)

	require.NoError(t, err)
	assert.Equal(t, "sync fallback", msg.TextContent())
	assert.Equal(t, int32(1), inner.calls.Load())
}

func TestCollector_ContextCancellation(t *testing.T) {
	sub := newMockSubmitter()
	sub.pollsUntilDone = 1000 // Never finish.
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ch := chat.New(message.NewText("", role.User, "hello"))
	_, err := c.Complete(ctx, ch, nil)

	assert.Error(t, err)
}

func TestCollector_UsageTracker(t *testing.T) {
	sub := newMockSubmitter()
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: 10 * time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	ch := chat.New(message.NewText("", role.User, "hello"))
	_, err := c.Complete(context.Background(), ch, nil)
	require.NoError(t, err)

	total := c.UsageTracker().Total()
	assert.Equal(t, 10, total.InputTokens)
	assert.Equal(t, 5, total.OutputTokens)
}

func TestCollector_ModelMaxTokens_ForwardsToInner(t *testing.T) {
	sub := newMockSubmitter()
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{})

	assert.Equal(t, 4096, c.ModelMaxTokens())
}

func TestCollector_EventHandler(t *testing.T) {
	sub := newMockSubmitter()
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: 10 * time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	var mu sync.Mutex
	var events []batch.EventKind

	c.SetEventHandler(func(kind batch.EventKind, _ map[string]any) {
		mu.Lock()
		events = append(events, kind)
		mu.Unlock()
	})

	ch := chat.New(message.NewText("", role.User, "hello"))
	_, err := c.Complete(context.Background(), ch, nil)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, events, batch.EventSubmitted)
	assert.Contains(t, events, batch.EventPolling)
	assert.Contains(t, events, batch.EventCompleted)
}

func TestCollector_MultiplePolls(t *testing.T) {
	sub := newMockSubmitter()
	sub.pollsUntilDone = 3 // Need 3 polls before done.
	inner := &mockCompleter{}
	c := batch.NewCollector(inner, sub, batch.CollectorOpts{
		CollectWindow: 10 * time.Millisecond,
		PollInterval:  time.Millisecond,
		Timeout:       5 * time.Second,
	})

	ch := chat.New(message.NewText("", role.User, "hello"))
	msg, err := c.Complete(context.Background(), ch, nil)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, msg.Role)
	assert.GreaterOrEqual(t, int(sub.pollCount.Load()), 3)
}
