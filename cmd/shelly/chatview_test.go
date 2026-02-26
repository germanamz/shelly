package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveContainer_TopLevel(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("agent-a", "🤖", "")

	ac := cv.resolveContainer("agent-a")
	require.NotNil(t, ac)
	assert.Equal(t, "agent-a", ac.agent)
}

func TestResolveContainer_Nested(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("parent", "🤖", "")
	cv.startAgent("child", "🦾", "parent")

	ac := cv.resolveContainer("child")
	require.NotNil(t, ac)
	assert.Equal(t, "child", ac.agent)
	assert.Equal(t, 4, ac.maxShow, "nested agents should have maxShow=4")
}

func TestResolveContainer_NotFound(t *testing.T) {
	cv := newChatView(false)
	ac := cv.resolveContainer("nonexistent")
	assert.Nil(t, ac)
}

func TestStartAgent_TopLevel(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("agent-a", "🤖", "")

	assert.Contains(t, cv.agents, "agent-a")
	assert.Equal(t, []string{"agent-a"}, cv.agentOrder)
	assert.Empty(t, cv.subAgents)
}

func TestStartAgent_Nested(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("parent", "🤖", "")
	cv.startAgent("child", "📝", "parent")

	// Child should be in subAgents, not top-level agents.
	assert.NotContains(t, cv.agents, "child")
	assert.Contains(t, cv.subAgents, "child")

	// Child should appear as a display item inside the parent.
	parentAC := cv.agents["parent"]
	require.NotNil(t, parentAC)
	require.Len(t, parentAC.items, 1)
	sa, ok := parentAC.items[0].(*subAgentMessage)
	require.True(t, ok)
	assert.Equal(t, "child", sa.container.agent)
	assert.Equal(t, "📝", sa.container.prefix)
}

func TestStartAgent_DuplicateTopLevel(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("agent-a", "🤖", "")
	cv.startAgent("agent-a", "📝", "") // duplicate — should be ignored

	assert.Len(t, cv.agentOrder, 1)
	assert.Equal(t, "🤖", cv.agents["agent-a"].prefix, "prefix should not change on duplicate")
}

func TestEndAgent_TopLevel(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("agent-a", "🤖", "")
	// Add a tool call so we get a summary.
	cv.agents["agent-a"].addToolCall("call-1", "read_file", `{"path":"foo.go"}`)
	cv.agents["agent-a"].completeToolCall("call-1", "contents", false)

	cmd := cv.endAgent("agent-a", "")

	assert.NotContains(t, cv.agents, "agent-a")
	assert.Empty(t, cv.agentOrder)
	assert.NotNil(t, cmd, "should return a tea.Println command for the summary")
}

func TestEndAgent_SubAgent(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("parent", "🤖", "")
	cv.startAgent("child", "🦾", "parent")

	cmd := cv.endAgent("child", "parent")

	// Sub-agent should be removed from subAgents map.
	assert.NotContains(t, cv.subAgents, "child")
	// Parent should still exist.
	assert.Contains(t, cv.agents, "parent")
	// No println command — summary is rendered inline.
	assert.Nil(t, cmd)

	// The sub-agent's container should be marked done.
	parentAC := cv.agents["parent"]
	sa, ok := parentAC.items[0].(*subAgentMessage)
	require.True(t, ok)
	assert.True(t, sa.container.done)
}

func TestEndAgent_NotFound(t *testing.T) {
	cv := newChatView(false)
	cmd := cv.endAgent("nonexistent", "")
	assert.Nil(t, cmd)
}

func TestSubAgentMessageView(t *testing.T) {
	ac := newAgentContainer("worker", "🦾", 4)
	ac.addToolCall("call-1", "exec", `{"cmd":"ls"}`)
	ac.completeToolCall("call-1", "file.go", false)

	sa := &subAgentMessage{container: ac}
	view := sa.View(80)

	assert.Contains(t, view, "🦾 worker")
	assert.Contains(t, view, treePipe)
	assert.Contains(t, view, "exec")
	assert.True(t, sa.IsLive(), "should be live while container is not done")

	ac.done = true
	assert.False(t, sa.IsLive(), "should not be live after container is done")
	doneView := sa.View(80)
	assert.Contains(t, doneView, "(done)")
}

func TestAdvanceSpinners_Recursion(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("parent", "🤖", "")
	cv.startAgent("child", "🦾", "parent")

	childAC := cv.subAgents["child"].container
	childAC.addToolCall("call-1", "exec", `{"cmd":"test"}`)

	// Advance spinners on the chatView — should recurse into sub-agents.
	cv.advanceSpinners()

	parentAC := cv.agents["parent"]
	assert.Equal(t, 1, parentAC.frameIdx)
	assert.Equal(t, 1, childAC.frameIdx)

	// The pending tool call in the child should also advance.
	tc := childAC.findPendingCall()
	require.NotNil(t, tc)
	assert.Equal(t, 1, tc.frameIdx)
}

func TestDeeplyNestedAgents(t *testing.T) {
	cv := newChatView(false)
	cv.startAgent("A", "🤖", "")
	cv.startAgent("B", "🦾", "A")
	cv.startAgent("C", "📝", "B")

	// C should be nested inside B, which is nested inside A.
	assert.Contains(t, cv.subAgents, "B")
	assert.Contains(t, cv.subAgents, "C")

	acC := cv.resolveContainer("C")
	require.NotNil(t, acC)
	assert.Equal(t, "C", acC.agent)

	// End C, then B, then A.
	cv.endAgent("C", "B")
	assert.NotContains(t, cv.subAgents, "C")
	assert.True(t, acC.done)

	cv.endAgent("B", "A")
	assert.NotContains(t, cv.subAgents, "B")

	cmd := cv.endAgent("A", "")
	assert.NotContains(t, cv.agents, "A")
	// A had sub-agent items but no direct tool calls, so summary may be empty.
	_ = cmd
}

func TestPlanMessage(t *testing.T) {
	pm := &planMessage{agent: "planner", prefix: "📝", text: "Step 1: do thing"}
	view := pm.View(80)
	assert.Contains(t, view, "📝 planner plan:")
	assert.Contains(t, view, "Step 1")
	assert.False(t, pm.IsLive())
	assert.Equal(t, "plan", pm.Kind())
}
