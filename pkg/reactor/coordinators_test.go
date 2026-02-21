package reactor

import (
	"context"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSequenceRunsAll(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
		member(newMockAgent("B"), "worker"),
		member(newMockAgent("C"), "worker"),
	}

	coord := NewSequence()
	ctx := context.Background()
	shared := chat.New()

	for i := range 3 {
		sel, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
		assert.False(t, sel.Done)
		assert.Equal(t, []int{i}, sel.Members)
	}
}

func TestSequenceDoneAfter(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewSequence()
	ctx := context.Background()
	shared := chat.New()

	_, _ = coord.Next(ctx, shared, members)

	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.True(t, sel.Done)
}

func TestLoopCycling(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
		member(newMockAgent("B"), "worker"),
	}

	coord := NewLoop(0)
	ctx := context.Background()
	shared := chat.New()

	for i := range 6 {
		sel, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
		assert.False(t, sel.Done)
		assert.Equal(t, []int{i % 2}, sel.Members)
	}
}

func TestLoopMaxRounds(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
		member(newMockAgent("B"), "worker"),
	}

	coord := NewLoop(2) // 2 rounds * 2 members = 4 steps
	ctx := context.Background()
	shared := chat.New()

	for range 4 {
		_, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
	}

	_, err := coord.Next(ctx, shared, members)
	assert.ErrorIs(t, err, ErrMaxRounds)
}

func TestLoopUnlimited(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLoop(0)
	ctx := context.Background()
	shared := chat.New()

	for range 100 {
		sel, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
		assert.False(t, sel.Done)
	}
}

func TestRoundRobinUntilPredicateStop(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
		member(newMockAgent("B"), "worker"),
	}

	shared := chat.New()
	coord := NewRoundRobinUntil(0, func(c *chat.Chat) bool {
		return c.Len() >= 3
	})
	ctx := context.Background()

	// Run 3 steps, adding a message each time so predicate triggers on 4th call.
	for i := range 3 {
		sel, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
		assert.False(t, sel.Done)
		assert.Equal(t, []int{i % 2}, sel.Members)
		shared.Append(message.NewText("x", role.User, "msg"))
	}

	// Predicate now true (3 messages in shared chat).
	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.True(t, sel.Done)
}

func TestRoundRobinUntilMaxRounds(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewRoundRobinUntil(2, func(_ *chat.Chat) bool { return false })
	ctx := context.Background()
	shared := chat.New()

	for range 2 {
		_, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
	}

	_, err := coord.Next(ctx, shared, members)
	assert.ErrorIs(t, err, ErrMaxRounds)
}

func TestRoundRobinUntilFirstStepSkip(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	// Predicate true immediately.
	coord := NewRoundRobinUntil(0, func(_ *chat.Chat) bool { return true })
	ctx := context.Background()
	shared := chat.New()

	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.True(t, sel.Done)
}

func TestRoleRoundRobinDispatch(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("researcher"), "research"),
		member(newMockAgent("writer"), "write"),
	}

	coord := NewRoleRoundRobin(1, "research", "write")
	ctx := context.Background()
	shared := chat.New()

	// First step: research role → index 0.
	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.Equal(t, []int{0}, sel.Members)

	// Second step: write role → index 1.
	sel, err = coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, sel.Members)
}

func TestRoleRoundRobinSkipsEmptyRoles(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("writer"), "write"),
	}

	// "research" has no members, should be skipped.
	coord := NewRoleRoundRobin(0, "research", "write")
	ctx := context.Background()
	shared := chat.New()

	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.Equal(t, []int{0}, sel.Members)
}

func TestRoleRoundRobinMaxRounds(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewRoleRoundRobin(2, "worker")
	ctx := context.Background()
	shared := chat.New()

	for range 2 {
		_, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
	}

	_, err := coord.Next(ctx, shared, members)
	assert.ErrorIs(t, err, ErrMaxRounds)
}

func TestRoleRoundRobinConcurrentSelection(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("r1"), "research"),
		member(newMockAgent("r2"), "research"),
		member(newMockAgent("w"), "write"),
	}

	coord := NewRoleRoundRobin(1, "research", "write")
	ctx := context.Background()
	shared := chat.New()

	// First step: research role → both researchers selected.
	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, sel.Members)

	// Second step: write role → single writer selected.
	sel, err = coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.Equal(t, []int{2}, sel.Members)
}

func TestRoleRoundRobinNoMatchingRoles(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	// No roles match any member — should signal done.
	coord := NewRoleRoundRobin(0, "nonexistent")
	ctx := context.Background()
	shared := chat.New()

	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.True(t, sel.Done)
}

func TestRoleRoundRobinEmptyOrder(t *testing.T) {
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewRoleRoundRobin(0)
	ctx := context.Background()
	shared := chat.New()

	sel, err := coord.Next(ctx, shared, members)
	require.NoError(t, err)
	assert.True(t, sel.Done)
}

// mockCompleter implements modeladapter.Completer for testing. It returns
// predetermined responses in order.
type mockCompleter struct {
	responses []message.Message
	index     int
}

func (m *mockCompleter) Complete(_ context.Context, c *chat.Chat) (message.Message, error) {
	if m.index >= len(m.responses) {
		return message.Message{}, errors.New("no more responses")
	}

	resp := m.responses[m.index]
	m.index++

	return resp, nil
}

func llmReply(text string) message.Message {
	return message.NewText("", role.Assistant, text)
}

func TestLLMCoordinatorSingleMember(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [0], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("researcher"), "research"),
		member(newMockAgent("writer"), "write"),
	}

	coord := NewLLMCoordinator(comp, 10)
	sel, err := coord.Next(context.Background(), chat.New(), members)
	require.NoError(t, err)
	assert.False(t, sel.Done)
	assert.Equal(t, []int{0}, sel.Members)
}

func TestLLMCoordinatorMultiMember(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [0, 1], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("researcher"), "research"),
		member(newMockAgent("writer"), "write"),
	}

	coord := NewLLMCoordinator(comp, 10)
	sel, err := coord.Next(context.Background(), chat.New(), members)
	require.NoError(t, err)
	assert.False(t, sel.Done)
	assert.Equal(t, []int{0, 1}, sel.Members)
}

func TestLLMCoordinatorDoneSignal(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [], "done": true}`),
	}}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLLMCoordinator(comp, 10)
	sel, err := coord.Next(context.Background(), chat.New(), members)
	require.NoError(t, err)
	assert.True(t, sel.Done)
}

func TestLLMCoordinatorInvalidJSONRetrySuccess(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`not json at all`),
		llmReply(`{"members": [0], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLLMCoordinator(comp, 10)
	sel, err := coord.Next(context.Background(), chat.New(), members)
	require.NoError(t, err)
	assert.Equal(t, []int{0}, sel.Members)
}

func TestLLMCoordinatorInvalidJSONTwiceFails(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`not json`),
		llmReply(`still not json`),
	}}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLLMCoordinator(comp, 10)
	_, err := coord.Next(context.Background(), chat.New(), members)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm coordinator")
}

func TestLLMCoordinatorOutOfRangeIndex(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [5], "done": false}`),
		// Retry response also out of range.
		llmReply(`{"members": [5], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLLMCoordinator(comp, 10)
	_, err := coord.Next(context.Background(), chat.New(), members)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestLLMCoordinatorMaxRounds(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [0], "done": false}`),
		llmReply(`{"members": [0], "done": false}`),
		llmReply(`{"members": [0], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLLMCoordinator(comp, 2) // 2 rounds * 1 member = 2 steps
	ctx := context.Background()
	shared := chat.New()

	for range 2 {
		_, err := coord.Next(ctx, shared, members)
		require.NoError(t, err)
	}

	_, err := coord.Next(ctx, shared, members)
	assert.ErrorIs(t, err, ErrMaxRounds)
}

func TestLLMCoordinatorContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	comp := &errCompleter{err: context.Canceled}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLLMCoordinator(comp, 10)
	_, err := coord.Next(ctx, chat.New(), members)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLLMCoordinatorEmptyDescriptors(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [0], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("researcher"), "research"),
	}

	coord := NewLLMCoordinator(comp, 10)
	sel, err := coord.Next(context.Background(), chat.New(), members)
	require.NoError(t, err)
	assert.Equal(t, []int{0}, sel.Members)

	// Verify system prompt does not contain description separator.
	sysPrompt := coord.chat.SystemPrompt()
	assert.NotContains(t, sysPrompt, "—")
}

func TestLLMCoordinatorWithDescriptors(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [1], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("researcher"), "research"),
		member(newMockAgent("writer"), "write"),
	}

	coord := NewLLMCoordinator(comp, 10,
		MemberDescriptor{Description: "Searches the web"},
		MemberDescriptor{Description: "Writes articles"},
	)
	sel, err := coord.Next(context.Background(), chat.New(), members)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, sel.Members)

	// Verify descriptions appear in system prompt.
	sysPrompt := coord.chat.SystemPrompt()
	assert.Contains(t, sysPrompt, "Searches the web")
	assert.Contains(t, sysPrompt, "Writes articles")
}

func TestLLMCoordinatorCodeFenceStripping(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply("```json\n{\"members\": [0], \"done\": false}\n```"),
	}}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	coord := NewLLMCoordinator(comp, 10)
	sel, err := coord.Next(context.Background(), chat.New(), members)
	require.NoError(t, err)
	assert.Equal(t, []int{0}, sel.Members)
}

func TestLLMCoordinatorSharedChatContext(t *testing.T) {
	comp := &mockCompleter{responses: []message.Message{
		llmReply(`{"members": [0], "done": false}`),
	}}
	members := []TeamMember{
		member(newMockAgent("A"), "worker"),
	}

	shared := chat.New(
		message.NewText("user", role.User, "Hello"),
		message.NewText("A", role.Assistant, "Hi there"),
	)

	coord := NewLLMCoordinator(comp, 10)
	_, err := coord.Next(context.Background(), shared, members)
	require.NoError(t, err)

	// Coordinator's chat should have: system prompt + context message + LLM reply.
	assert.Equal(t, 3, coord.chat.Len())
	contextMsg := coord.chat.At(1)
	assert.Contains(t, contextMsg.TextContent(), "Hello")
	assert.Contains(t, contextMsg.TextContent(), "Hi there")
}

// errCompleter always returns an error.
type errCompleter struct {
	err error
}

func (e *errCompleter) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return message.Message{}, e.err
}
