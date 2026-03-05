package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelegationProgressFunc_ForwardsOriginalEvents(t *testing.T) {
	var received []string
	parent := EventFunc(func(_ context.Context, kind string, _ any) {
		received = append(received, kind)
	})

	fn := delegationProgressFunc(parent, nil, "child-1", "parent")
	fn(context.Background(), "tool_call_start", ToolCallEventData{ToolName: "read"})
	fn(context.Background(), "tool_call_end", ToolCallEventData{ToolName: "read"})

	assert.Equal(t, []string{"tool_call_start", "tool_call_end"}, received)
}

func TestDelegationProgressFunc_EmitsProgressForAssistantMessages(t *testing.T) {
	var mu sync.Mutex
	var events []DelegationEvent

	notifier := EventNotifier(func(_ context.Context, kind string, agentName string, data any) {
		if kind == "delegation_progress" {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, data.(DelegationEvent))
		}
	})

	fn := delegationProgressFunc(nil, notifier, "child-1", "parent")

	// Emit an assistant message_added event.
	msg := message.NewText("child-1", "assistant", "I'm working on the task")
	fn(context.Background(), "message_added", MessageAddedEventData{
		Role:    "assistant",
		Message: msg,
	})

	require.Len(t, events, 1)
	assert.Equal(t, DelegationProgress, events[0].Kind)
	assert.Equal(t, "child-1", events[0].Agent)
	assert.Equal(t, "parent", events[0].Parent)
	assert.Equal(t, "I'm working on the task", events[0].Message)
}

func TestDelegationProgressFunc_IgnoresToolMessages(t *testing.T) {
	var called bool
	notifier := EventNotifier(func(_ context.Context, _ string, _ string, _ any) {
		called = true
	})

	fn := delegationProgressFunc(nil, notifier, "child-1", "parent")

	msg := message.NewText("child-1", "tool", "tool result content")
	fn(context.Background(), "message_added", MessageAddedEventData{
		Role:    "tool",
		Message: msg,
	})

	assert.False(t, called, "should not emit progress for tool messages")
}

func TestDelegationProgressFunc_IgnoresEmptyAssistantText(t *testing.T) {
	var called bool
	notifier := EventNotifier(func(_ context.Context, _ string, _ string, _ any) {
		called = true
	})

	fn := delegationProgressFunc(nil, notifier, "child-1", "parent")

	// Assistant message with only tool calls (no text content).
	msg := message.NewText("child-1", "assistant", "")
	fn(context.Background(), "message_added", MessageAddedEventData{
		Role:    "assistant",
		Message: msg,
	})

	assert.False(t, called, "should not emit progress for empty assistant text")
}

func TestDelegationProgressFunc_TruncatesLongMessages(t *testing.T) {
	var events []DelegationEvent

	notifier := EventNotifier(func(_ context.Context, _ string, _ string, data any) {
		events = append(events, data.(DelegationEvent))
	})

	fn := delegationProgressFunc(nil, notifier, "child-1", "parent")

	longText := strings.Repeat("a", maxProgressMessageRunes+100)
	msg := message.NewText("child-1", "assistant", longText)
	fn(context.Background(), "message_added", MessageAddedEventData{
		Role:    "assistant",
		Message: msg,
	})

	require.Len(t, events, 1)
	assert.Len(t, []rune(events[0].Message), maxProgressMessageRunes+len("..."))
	assert.True(t, strings.HasSuffix(events[0].Message, "..."))
}

func TestDelegationProgressFunc_NilParentNoNotifier(t *testing.T) {
	// Should not panic with nil parent and nil notifier.
	fn := delegationProgressFunc(nil, nil, "child-1", "parent")

	msg := message.NewText("child-1", "assistant", "hello")
	fn(context.Background(), "message_added", MessageAddedEventData{
		Role:    "assistant",
		Message: msg,
	})
}

func TestEmitDelegationResult_PublishesResultEvent(t *testing.T) {
	var events []DelegationEvent

	notifier := EventNotifier(func(_ context.Context, kind string, _ string, data any) {
		if kind == "delegation_progress" {
			events = append(events, data.(DelegationEvent))
		}
	})

	dr := delegateResult{Agent: "coder", Result: "done"}
	emitDelegationResult(notifier, context.Background(), "coder-fix-1", "orchestrator", dr)

	require.Len(t, events, 1)
	assert.Equal(t, DelegationResult, events[0].Kind)
	assert.Equal(t, "coder-fix-1", events[0].Agent)
	assert.Equal(t, "orchestrator", events[0].Parent)
	require.NotNil(t, events[0].Result)
	assert.Equal(t, "done", events[0].Result.Result)
}

func TestEmitDelegationResult_NilNotifierNoOp(t *testing.T) {
	// Should not panic.
	emitDelegationResult(nil, context.Background(), "child", "parent", delegateResult{})
}

func TestDelegationEventKind_Values(t *testing.T) {
	assert.Equal(t, DelegationStatus, DelegationEventKind("status"))
	assert.Equal(t, DelegationProgress, DelegationEventKind("progress"))
	assert.Equal(t, DelegationResult, DelegationEventKind("result"))
}
