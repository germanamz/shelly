package delegate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAgent implements reactor.NamedAgent for testing.
type mockAgent struct {
	name    string
	chat    *chat.Chat
	replies []message.Message
	index   int
	err     error
}

func (m *mockAgent) Run(_ context.Context) (message.Message, error) {
	if m.err != nil {
		return message.Message{}, m.err
	}

	if m.index >= len(m.replies) {
		return message.Message{}, errors.New("no more replies")
	}

	reply := m.replies[m.index]
	reply.Sender = m.name
	m.index++
	m.chat.Append(reply)

	return reply, nil
}

func (m *mockAgent) AgentName() string     { return m.name }
func (m *mockAgent) AgentChat() *chat.Chat { return m.chat }

func newMockAgent(name string, replies ...message.Message) *mockAgent {
	return &mockAgent{
		name:    name,
		chat:    chat.New(),
		replies: replies,
	}
}

func TestNewAgentTool(t *testing.T) {
	agent := newMockAgent("helper", message.NewText("", role.Assistant, "done"))
	at := NewAgentTool(agent, "A helper agent")

	assert.NotNil(t, at)
	assert.Equal(t, agent, at.agent)
	assert.Equal(t, "A helper agent", at.description)
}

func TestToolReturnsCorrectMetadata(t *testing.T) {
	agent := newMockAgent("helper", message.NewText("", role.Assistant, "done"))
	at := NewAgentTool(agent, "A helper agent")

	tool := at.Tool()

	assert.Equal(t, "helper", tool.Name)
	assert.Equal(t, "A helper agent", tool.Description)
	assert.NotNil(t, tool.InputSchema)
	assert.NotNil(t, tool.Handler)
}

func TestSingleDelegation(t *testing.T) {
	agent := newMockAgent("researcher",
		message.NewText("", role.Assistant, "Here is the research result."),
	)
	at := NewAgentTool(agent, "Research agent")
	tool := at.Tool()

	input := json.RawMessage(`{"task":"Find information about Go"}`)
	result, err := tool.Handler(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, "Here is the research result.", result)

	// The agent's chat should have the user message + the agent's reply.
	require.Equal(t, 2, agent.chat.Len())
	assert.Equal(t, "Find information about Go", agent.chat.At(0).TextContent())
	assert.Equal(t, role.User, agent.chat.At(0).Role)
}

func TestMultipleDelegations(t *testing.T) {
	agent := newMockAgent("helper",
		message.NewText("", role.Assistant, "First answer."),
		message.NewText("", role.Assistant, "Second answer."),
	)
	at := NewAgentTool(agent, "Helper")
	tool := at.Tool()

	result1, err := tool.Handler(context.Background(), json.RawMessage(`{"task":"task one"}`))
	require.NoError(t, err)
	assert.Equal(t, "First answer.", result1)

	result2, err := tool.Handler(context.Background(), json.RawMessage(`{"task":"task two"}`))
	require.NoError(t, err)
	assert.Equal(t, "Second answer.", result2)

	// Chat should accumulate: user1, reply1, user2, reply2.
	assert.Equal(t, 4, agent.chat.Len())
}

func TestDelegationErrorPropagation(t *testing.T) {
	agentErr := errors.New("agent failed")
	agent := &mockAgent{
		name: "broken",
		chat: chat.New(),
		err:  agentErr,
	}
	at := NewAgentTool(agent, "Broken agent")
	tool := at.Tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"task":"do something"}`))

	require.ErrorIs(t, err, agentErr)
}

func TestDelegationInvalidJSON(t *testing.T) {
	agent := newMockAgent("helper", message.NewText("", role.Assistant, "done"))
	at := NewAgentTool(agent, "Helper")
	tool := at.Tool()

	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestDelegationContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	agent := &mockAgent{
		name: "slow",
		chat: chat.New(),
		err:  context.Canceled,
	}
	at := NewAgentTool(agent, "Slow agent")
	tool := at.Tool()

	_, err := tool.Handler(ctx, json.RawMessage(`{"task":"do something"}`))

	assert.ErrorIs(t, err, context.Canceled)
}

func TestDelegationToolNameMatchesAgentName(t *testing.T) {
	agent := newMockAgent("my-special-agent", message.NewText("", role.Assistant, "ok"))
	at := NewAgentTool(agent, "desc")

	tool := at.Tool()

	assert.Equal(t, "my-special-agent", tool.Name)
}

func TestDelegationDefaultInputSchema(t *testing.T) {
	agent := newMockAgent("a", message.NewText("", role.Assistant, "ok"))
	at := NewAgentTool(agent, "desc")

	tool := at.Tool()

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	_, hasTask := props["task"]
	assert.True(t, hasTask)
}
