package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgentContainer(t *testing.T) {
	ac := newAgentContainer("test-agent", "🤖", 0)
	assert.Equal(t, "test-agent", ac.agent)
	assert.Equal(t, "🤖", ac.prefix)
	assert.Equal(t, 0, ac.maxShow)
	assert.False(t, ac.done)
	assert.NotNil(t, ac.callIndex)
	assert.NotEmpty(t, ac.spinMsg)
}

func TestNewAgentContainerDefaultPrefix(t *testing.T) {
	ac := newAgentContainer("test", "", 0)
	assert.Equal(t, "🤖", ac.prefix)
}

func TestAddToolCall(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)
	tc := ac.addToolCall("call-1", "fs_read", `{"path": "test.txt"}`)

	require.Len(t, ac.items, 1)
	assert.Equal(t, "call-1", tc.callID)
	assert.Equal(t, "fs_read", tc.toolName)
	assert.False(t, tc.completed)
}

func TestCompleteToolCall(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)
	ac.addToolCall("call-1", "fs_read", `{"path": "test.txt"}`)

	ac.completeToolCall("call-1", "file content here", false)

	tc := ac.callIndex["call-1"]
	assert.True(t, tc.completed)
	assert.Equal(t, "file content here", tc.result)
	assert.False(t, tc.isError)
}

func TestCompleteToolCallWithError(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)
	ac.addToolCall("call-1", "fs_read", `{}`)

	ac.completeToolCall("call-1", "file not found", true)

	tc := ac.callIndex["call-1"]
	assert.True(t, tc.completed)
	assert.True(t, tc.isError)
}

func TestToolGrouping(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)

	tg := ac.addToolGroup("fs_read", 4)
	ac.addGroupCall(tg, "call-1", `{}`)
	ac.addGroupCall(tg, "call-2", `{}`)

	require.Len(t, ac.items, 1) // single group item
	require.Len(t, tg.calls, 2)
	assert.True(t, tg.IsLive())

	ac.completeToolCall("call-1", "result1", false)
	assert.True(t, tg.IsLive()) // still has pending call

	ac.completeToolCall("call-2", "result2", false)
	assert.False(t, tg.IsLive()) // all done
}

func TestFindLastToolGroup(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)

	tg1 := ac.addToolGroup("fs_read", 4)
	ac.addToolCall("single", "fs_write", `{}`)
	tg2 := ac.addToolGroup("fs_read", 4)

	found := ac.findLastToolGroup("fs_read")
	assert.Equal(t, tg2, found)
	assert.NotEqual(t, tg1, found)
}

func TestAddThinking(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)
	ac.addThinking("some reasoning text")

	require.Len(t, ac.items, 1)
	item, ok := ac.items[0].(*thinkingItem)
	require.True(t, ok)
	assert.Equal(t, "some reasoning text", item.text)
	assert.False(t, item.IsLive())
}

func TestAddPlan(t *testing.T) {
	ac := newAgentContainer("agent", "📝", 0)
	ac.addPlan("step 1, step 2")

	require.Len(t, ac.items, 1)
	item, ok := ac.items[0].(*planItem)
	require.True(t, ok)
	assert.Equal(t, "step 1, step 2", item.text)
}

func TestCollapsedSummary(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)
	ac.done = true
	summary := ac.collapsedSummary()

	assert.Contains(t, summary, "🤖 agent")
	assert.Contains(t, summary, "Finished in")
}

func TestViewThinking(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)
	// No items, should show "is thinking..."
	view := ac.View(80)
	assert.Contains(t, view, "is thinking...")
}

func TestAdvanceSpinners(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 0)
	ac.addToolCall("call-1", "fs_read", `{}`)

	assert.Equal(t, 0, ac.frameIdx)
	ac.advanceSpinners()
	assert.Equal(t, 1, ac.frameIdx)
}

func TestSubAgentNesting(t *testing.T) {
	parent := newAgentContainer("parent", "🤖", 0)
	child := newAgentContainer("child", "🦾", 4)
	sa := &subAgentItem{container: child}
	parent.items = append(parent.items, sa)

	assert.True(t, sa.IsLive())
	assert.Equal(t, "sub_agent", sa.Kind())

	child.done = true
	assert.False(t, sa.IsLive())
}

func TestWindowedView(t *testing.T) {
	ac := newAgentContainer("agent", "🤖", 2)
	ac.addThinking("text1")
	ac.addThinking("text2")
	ac.addThinking("text3")

	view := ac.View(80)
	assert.Contains(t, view, "... 1 more items")
}
