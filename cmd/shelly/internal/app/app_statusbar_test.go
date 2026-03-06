package app

import (
	"testing"

	"github.com/germanamz/shelly/cmd/shelly/internal/chatview"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProviderLabel(t *testing.T) {
	kind, model := parseProviderLabel("anthropic/claude-sonnet-4")
	assert.Equal(t, "anthropic", kind)
	assert.Equal(t, "claude-sonnet-4", model)
}

func TestParseProviderLabel_NoSlash(t *testing.T) {
	kind, model := parseProviderLabel("local")
	assert.Equal(t, "local", kind)
	assert.Empty(t, model)
}

func TestParseProviderLabel_Empty(t *testing.T) {
	kind, model := parseProviderLabel("")
	assert.Empty(t, kind)
	assert.Empty(t, model)
}

func TestInitAgentUsage_StoresProviderInfo(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("agent-1", "anthropic/claude-sonnet-4")

	info, ok := m.agentUsage["agent-1"]
	require.True(t, ok)
	assert.Equal(t, "anthropic/claude-sonnet-4", info.ProviderLabel)
	assert.Equal(t, "anthropic", info.ProviderKind)
	assert.Equal(t, "claude-sonnet-4", info.Model)
	assert.False(t, info.Ended)
}

func TestInitAgentUsage_DoesNotOverwrite(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("agent-1", "anthropic/claude-sonnet-4")
	// Record some usage.
	m.recordAgentUsage("agent-1", usage.TokenCount{InputTokens: 100}, false)

	// Re-init should not overwrite.
	m.initAgentUsage("agent-1", "openai/gpt-4")

	info := m.agentUsage["agent-1"]
	assert.Equal(t, "anthropic/claude-sonnet-4", info.ProviderLabel)
	assert.Equal(t, 100, info.Usage.InputTokens)
}

func TestRecordAgentUsage_PreservesProviderInfo(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("agent-1", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("agent-1", usage.TokenCount{InputTokens: 500, OutputTokens: 200}, false)

	info := m.agentUsage["agent-1"]
	assert.Equal(t, "anthropic/claude-sonnet-4", info.ProviderLabel)
	assert.Equal(t, "anthropic", info.ProviderKind)
	assert.Equal(t, 500, info.Usage.InputTokens)
	assert.Equal(t, 200, info.Usage.OutputTokens)
	assert.False(t, info.Ended)
}

func TestRecordAgentUsage_MarksEnded(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("agent-1", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("agent-1", usage.TokenCount{InputTokens: 100}, true)

	info := m.agentUsage["agent-1"]
	assert.True(t, info.Ended)
}

func TestAgentStatusSegments_WithUsage(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("coder-auth-7", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("coder-auth-7", usage.TokenCount{InputTokens: 10000, OutputTokens: 2400}, false)

	segments := m.agentStatusSegments("coder-auth-7")
	assert.Contains(t, segments, "coder-auth-7")
	assert.Contains(t, segments, "anthropic/claude-sonnet-4")

	// Should have a token count segment.
	hasTokens := false
	for _, s := range segments {
		if len(s) > 0 && s[len(s)-len("tokens"):] == "tokens" {
			hasTokens = true
		}
	}
	assert.True(t, hasTokens, "should have token count segment")
}

func TestAgentStatusSegments_NoUsage(t *testing.T) {
	m := newTestModel()
	// No usage recorded for this agent.
	segments := m.agentStatusSegments("unknown-agent")
	assert.Equal(t, []string{"unknown-agent"}, segments)
}

func TestAgentStatusSegments_ZeroTokens(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("agent-1", "anthropic/claude-sonnet-4")
	// Zero tokens — provider label still shows.
	segments := m.agentStatusSegments("agent-1")
	assert.Contains(t, segments, "agent-1")
	assert.Contains(t, segments, "anthropic/claude-sonnet-4")
	assert.Len(t, segments, 2) // just agent ID + provider label
}

func TestSessionStatusSegments_Empty(t *testing.T) {
	m := newTestModel()
	segments := m.sessionStatusSegments()
	assert.Empty(t, segments)
}

func TestSessionStatusSegments_WithData(t *testing.T) {
	m := newTestModel()
	m.tokenCount = "12.4k"
	m.sessionCost = "$0.12"
	m.cacheInfo = "cache 45%"

	segments := m.sessionStatusSegments()
	assert.Contains(t, segments, "12.4k tokens")
	assert.Contains(t, segments, "$0.12")
	assert.Contains(t, segments, "cache 45%")
}

func TestCleanupAgentUsage_RemovesUnreferencedEnded(t *testing.T) {
	m := newTestModel()
	m.chatView = chatview.New()

	m.initAgentUsage("agent-1", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("agent-1", usage.TokenCount{InputTokens: 100}, true) // ended

	m.initAgentUsage("agent-2", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("agent-2", usage.TokenCount{InputTokens: 200}, false) // still running

	m.cleanupAgentUsage()

	// agent-1 ended and not on view stack → removed.
	_, ok := m.agentUsage["agent-1"]
	assert.False(t, ok, "ended agent not on view stack should be cleaned up")

	// agent-2 still running → kept.
	_, ok = m.agentUsage["agent-2"]
	assert.True(t, ok, "running agent should not be cleaned up")
}

func TestCleanupAgentUsage_KeepsViewedEndedAgent(t *testing.T) {
	m := newTestModel()
	cv := chatview.New()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})
	m.chatView = cv

	m.initAgentUsage("child", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("child", usage.TokenCount{InputTokens: 100}, true) // ended but viewed

	m.cleanupAgentUsage()

	// child ended but on view stack → kept.
	_, ok := m.agentUsage["child"]
	assert.True(t, ok, "ended agent on view stack should be kept")
}

func TestOnAgentStart_InitsUsage(t *testing.T) {
	m := newTestModel()
	m.onAgentStart(msgs.AgentStartMsg{Agent: "sub-1", Parent: "root", ProviderLabel: "openai/gpt-4o"})

	info, ok := m.agentUsage["sub-1"]
	require.True(t, ok)
	assert.Equal(t, "openai/gpt-4o", info.ProviderLabel)
	assert.Equal(t, "openai", info.ProviderKind)
	assert.Equal(t, "gpt-4o", info.Model)
}

func TestOnAgentEnd_FreezesUsage(t *testing.T) {
	m := newTestModel()
	// Set up chatview with the agent on view stack so cleanup doesn't remove it.
	cv := chatview.New()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "sub-1", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "sub-1"})
	m.chatView = cv

	m.initAgentUsage("sub-1", "anthropic/claude-sonnet-4")
	finalUsage := usage.TokenCount{InputTokens: 500, OutputTokens: 100}

	m.onAgentEnd(msgs.AgentEndMsg{Agent: "sub-1", Parent: "root", Usage: &finalUsage})

	info, ok := m.agentUsage["sub-1"]
	require.True(t, ok)
	assert.True(t, info.Ended)
	assert.Equal(t, 500, info.Usage.InputTokens)
	assert.Equal(t, 100, info.Usage.OutputTokens)
	// Provider info preserved after end.
	assert.Equal(t, "anthropic/claude-sonnet-4", info.ProviderLabel)
}

func TestKeyboardHint_ViewingSubAgent(t *testing.T) {
	m := newTestModel()
	cv := chatview.New()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})
	m.chatView = cv

	hint := m.keyboardHint()
	assert.Contains(t, hint, "esc back to parent")
}

func TestAgentUsage_LiveUpdate(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("sub-1", "anthropic/claude-sonnet-4")

	// Simulate live usage update.
	m.recordAgentUsage("sub-1", usage.TokenCount{InputTokens: 1000, OutputTokens: 500}, false)

	info := m.agentUsage["sub-1"]
	assert.Equal(t, 1000, info.Usage.InputTokens)
	assert.Equal(t, 500, info.Usage.OutputTokens)
	assert.False(t, info.Ended)
	// Provider info preserved.
	assert.Equal(t, "anthropic/claude-sonnet-4", info.ProviderLabel)
}

func TestAgentUsage_CacheRatio(t *testing.T) {
	m := newTestModel()
	m.initAgentUsage("agent-1", "anthropic/claude-sonnet-4")
	m.recordAgentUsage("agent-1", usage.TokenCount{
		InputTokens:          100,
		OutputTokens:         50,
		CacheReadInputTokens: 400,
	}, false)

	segments := m.agentStatusSegments("agent-1")
	hasCacheSegment := false
	for _, s := range segments {
		if len(s) >= 5 && s[:5] == "cache" {
			hasCacheSegment = true
		}
	}
	assert.True(t, hasCacheSegment, "should have cache ratio segment")
}
