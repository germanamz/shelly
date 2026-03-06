package modeladapter_test

import (
	"context"
	"sync"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCompleter is a test double that records usage and returns canned responses.
type stubCompleter struct {
	usage     usage.Tracker
	maxTokens int
	callCount int
	reply     message.Message
	err       error
	addUsage  usage.TokenCount // usage to add per call
}

func (s *stubCompleter) Complete(_ context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	s.callCount++
	if s.err != nil {
		return message.Message{}, s.err
	}
	s.usage.Add(s.addUsage)
	return s.reply, nil
}

func (s *stubCompleter) UsageTracker() *usage.Tracker { return &s.usage }
func (s *stubCompleter) ModelMaxTokens() int          { return s.maxTokens }

func TestAgentUsageCompleter_TracksUsage(t *testing.T) {
	inner := &stubCompleter{
		addUsage:  usage.TokenCount{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 10},
		reply:     message.NewText("", role.Assistant, "hello"),
		maxTokens: 4096,
	}
	mu := &sync.Mutex{}
	auc := modeladapter.NewAgentUsageCompleter(inner, mu)

	c := chat.New()
	msg, err := auc.Complete(context.Background(), c, nil)
	require.NoError(t, err)
	assert.Equal(t, "hello", msg.TextContent())

	// Agent-scoped tracker should have the call's usage.
	total := auc.UsageTracker().Total()
	assert.Equal(t, 100, total.InputTokens)
	assert.Equal(t, 50, total.OutputTokens)
	assert.Equal(t, 10, total.CacheReadInputTokens)

	// Second call accumulates.
	_, err = auc.Complete(context.Background(), c, nil)
	require.NoError(t, err)
	total = auc.UsageTracker().Total()
	assert.Equal(t, 200, total.InputTokens)
	assert.Equal(t, 100, total.OutputTokens)
}

func TestAgentUsageCompleter_DelegatesToShared(t *testing.T) {
	inner := &stubCompleter{
		addUsage: usage.TokenCount{InputTokens: 100, OutputTokens: 50},
		reply:    message.NewText("", role.Assistant, "ok"),
	}
	mu := &sync.Mutex{}
	auc := modeladapter.NewAgentUsageCompleter(inner, mu)

	c := chat.New()
	_, err := auc.Complete(context.Background(), c, nil)
	require.NoError(t, err)

	// Inner (shared) tracker should also have the usage.
	sharedTotal := inner.UsageTracker().Total()
	assert.Equal(t, 100, sharedTotal.InputTokens)
	assert.Equal(t, 50, sharedTotal.OutputTokens)

	// Call count verifies delegation.
	assert.Equal(t, 1, inner.callCount)
}

func TestAgentUsageCompleter_IndependentTrackers(t *testing.T) {
	inner := &stubCompleter{
		addUsage: usage.TokenCount{InputTokens: 100, OutputTokens: 50},
		reply:    message.NewText("", role.Assistant, "ok"),
	}
	mu := &sync.Mutex{}
	auc1 := modeladapter.NewAgentUsageCompleter(inner, mu)
	auc2 := modeladapter.NewAgentUsageCompleter(inner, mu)

	c := chat.New()

	// Agent 1 makes two calls.
	_, _ = auc1.Complete(context.Background(), c, nil)
	_, _ = auc1.Complete(context.Background(), c, nil)

	// Agent 2 makes one call.
	_, _ = auc2.Complete(context.Background(), c, nil)

	// Agent trackers are independent.
	assert.Equal(t, 200, auc1.UsageTracker().Total().InputTokens)
	assert.Equal(t, 100, auc2.UsageTracker().Total().InputTokens)

	// Shared tracker has all three calls.
	assert.Equal(t, 300, inner.UsageTracker().Total().InputTokens)
}

func TestAgentUsageCompleter_ErrorDoesNotRecordUsage(t *testing.T) {
	inner := &stubCompleter{
		addUsage: usage.TokenCount{InputTokens: 100, OutputTokens: 50},
		err:      assert.AnError,
	}
	mu := &sync.Mutex{}
	auc := modeladapter.NewAgentUsageCompleter(inner, mu)

	c := chat.New()
	_, err := auc.Complete(context.Background(), c, nil)
	require.Error(t, err)

	// No usage recorded on error.
	assert.Equal(t, 0, auc.UsageTracker().Total().InputTokens)
}

func TestAgentUsageCompleter_ModelMaxTokens(t *testing.T) {
	inner := &stubCompleter{maxTokens: 8192}
	mu := &sync.Mutex{}
	auc := modeladapter.NewAgentUsageCompleter(inner, mu)

	assert.Equal(t, 8192, auc.ModelMaxTokens())
}

func TestAgentUsageCompleter_Inner(t *testing.T) {
	inner := &stubCompleter{}
	mu := &sync.Mutex{}
	auc := modeladapter.NewAgentUsageCompleter(inner, mu)

	assert.Same(t, inner, auc.Inner())
}

// noUsageCompleter does not implement UsageReporter.
type noUsageCompleter struct {
	reply message.Message
}

func (n *noUsageCompleter) Complete(_ context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	return n.reply, nil
}

func TestAgentUsageCompleter_InnerWithoutUsageReporter(t *testing.T) {
	inner := &noUsageCompleter{reply: message.NewText("", role.Assistant, "ok")}
	mu := &sync.Mutex{}
	auc := modeladapter.NewAgentUsageCompleter(inner, mu)

	c := chat.New()
	msg, err := auc.Complete(context.Background(), c, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", msg.TextContent())

	// Agent tracker stays empty when inner doesn't track usage.
	assert.Equal(t, 0, auc.UsageTracker().Total().InputTokens)
	assert.Equal(t, 0, auc.ModelMaxTokens())
}

func TestAgentUsageCompleter_ConcurrentSafety(t *testing.T) {
	inner := &stubCompleter{
		addUsage: usage.TokenCount{InputTokens: 10, OutputTokens: 5},
		reply:    message.NewText("", role.Assistant, "ok"),
	}
	mu := &sync.Mutex{}

	const numAgents = 5
	const callsPerAgent = 20

	aucs := make([]*modeladapter.AgentUsageCompleter, numAgents)
	for i := range aucs {
		aucs[i] = modeladapter.NewAgentUsageCompleter(inner, mu)
	}

	var wg sync.WaitGroup
	for _, auc := range aucs {
		wg.Go(func() {
			c := chat.New()
			for range callsPerAgent {
				_, _ = auc.Complete(context.Background(), c, nil)
			}
		})
	}
	wg.Wait()

	// Each agent should have exactly its own calls tracked.
	for i, auc := range aucs {
		total := auc.UsageTracker().Total()
		assert.Equal(t, callsPerAgent*10, total.InputTokens, "agent %d input", i)
		assert.Equal(t, callsPerAgent*5, total.OutputTokens, "agent %d output", i)
	}

	// Shared tracker should have all calls.
	sharedTotal := inner.UsageTracker().Total()
	assert.Equal(t, numAgents*callsPerAgent*10, sharedTotal.InputTokens)
	assert.Equal(t, numAgents*callsPerAgent*5, sharedTotal.OutputTokens)
}
