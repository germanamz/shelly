package modeladapter

import (
	"context"
	"sync"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

var (
	_ Completer     = (*AgentUsageCompleter)(nil)
	_ UsageReporter = (*AgentUsageCompleter)(nil)
)

// AgentUsageCompleter wraps a shared Completer and records per-agent token
// usage in an independent Tracker. The shared completer's own tracker continues
// to accumulate all calls (session-level totals), while this wrapper captures
// only the calls made through it.
//
// A shared diffLock must be provided when multiple AgentUsageCompleters wrap
// the same inner Completer. It serializes the before/after total diff that
// attributes usage to this agent, preventing concurrent calls from corrupting
// the diff window.
type AgentUsageCompleter struct {
	inner    Completer
	usage    usage.Tracker
	diffLock *sync.Mutex
}

// NewAgentUsageCompleter creates a per-agent completer wrapper. The diffLock
// must be shared among all wrappers of the same inner Completer.
func NewAgentUsageCompleter(inner Completer, diffLock *sync.Mutex) *AgentUsageCompleter {
	return &AgentUsageCompleter{inner: inner, diffLock: diffLock}
}

// Complete delegates to the inner Completer and records the token usage diff
// in the agent-scoped tracker.
func (auc *AgentUsageCompleter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	ur, hasUsage := auc.inner.(UsageReporter)

	auc.diffLock.Lock()

	var before usage.TokenCount
	if hasUsage {
		before = ur.UsageTracker().Total()
	}

	msg, err := auc.inner.Complete(ctx, c, tools)

	if err == nil && hasUsage {
		after := ur.UsageTracker().Total()
		auc.usage.Add(usage.TokenCount{
			InputTokens:              after.InputTokens - before.InputTokens,
			OutputTokens:             after.OutputTokens - before.OutputTokens,
			CacheCreationInputTokens: after.CacheCreationInputTokens - before.CacheCreationInputTokens,
			CacheReadInputTokens:     after.CacheReadInputTokens - before.CacheReadInputTokens,
		})
	}

	auc.diffLock.Unlock()

	return msg, err
}

// UsageTracker returns the agent-scoped usage tracker.
func (auc *AgentUsageCompleter) UsageTracker() *usage.Tracker { return &auc.usage }

// ModelMaxTokens delegates to the inner Completer if it implements UsageReporter.
func (auc *AgentUsageCompleter) ModelMaxTokens() int {
	if ur, ok := auc.inner.(UsageReporter); ok {
		return ur.ModelMaxTokens()
	}
	return 0
}

// Inner returns the wrapped Completer.
func (auc *AgentUsageCompleter) Inner() Completer { return auc.inner }
