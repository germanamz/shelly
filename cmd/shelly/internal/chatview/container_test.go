package chatview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgentContainer(t *testing.T) {
	ac := NewAgentContainer("test-agent", "🤖", 0)
	assert.Equal(t, "test-agent", ac.Agent)
	assert.Equal(t, "🤖", ac.Prefix)
	assert.Equal(t, 0, ac.MaxShow)
	assert.False(t, ac.Done)
	assert.NotNil(t, ac.CallIndex)
	assert.NotEmpty(t, ac.SpinMsg)
}

func TestNewAgentContainerDefaultPrefix(t *testing.T) {
	ac := NewAgentContainer("test", "", 0)
	assert.Equal(t, "🤖", ac.Prefix)
}

func TestAddToolCall(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)
	tc := ac.AddToolCall("call-1", "fs_read", `{"path": "test.txt"}`)

	require.Len(t, ac.Items, 1)
	assert.Equal(t, "call-1", tc.CallID)
	assert.Equal(t, "fs_read", tc.ToolName)
	assert.False(t, tc.Completed)
}

func TestCompleteToolCall(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)
	ac.AddToolCall("call-1", "fs_read", `{"path": "test.txt"}`)

	ac.CompleteToolCall("call-1", "file content here", false)

	tc := ac.CallIndex["call-1"]
	assert.True(t, tc.Completed)
	assert.Equal(t, "file content here", tc.Result)
	assert.False(t, tc.IsError)
}

func TestCompleteToolCallWithError(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)
	ac.AddToolCall("call-1", "fs_read", `{}`)

	ac.CompleteToolCall("call-1", "file not found", true)

	tc := ac.CallIndex["call-1"]
	assert.True(t, tc.Completed)
	assert.True(t, tc.IsError)
}

func TestToolGrouping(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)

	tg := ac.AddToolGroup("fs_read", 4)
	ac.AddGroupCall(tg, "call-1", `{}`)
	ac.AddGroupCall(tg, "call-2", `{}`)

	require.Len(t, ac.Items, 1) // single group item
	require.Len(t, tg.Calls, 2)
	assert.True(t, tg.IsLive())

	ac.CompleteToolCall("call-1", "result1", false)
	assert.True(t, tg.IsLive()) // still has pending call

	ac.CompleteToolCall("call-2", "result2", false)
	assert.False(t, tg.IsLive()) // all done
}

func TestFindLastToolGroup(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)

	tg1 := ac.AddToolGroup("fs_read", 4)
	ac.AddToolCall("single", "fs_write", `{}`)
	tg2 := ac.AddToolGroup("fs_read", 4)

	found := ac.FindLastToolGroup("fs_read")
	assert.Equal(t, tg2, found)
	assert.NotEqual(t, tg1, found)
}

func TestAddThinking(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)
	ac.AddThinking("some reasoning text")

	require.Len(t, ac.Items, 1)
	item, ok := ac.Items[0].(*ThinkingItem)
	require.True(t, ok)
	assert.Equal(t, "some reasoning text", item.Text)
	assert.False(t, item.IsLive())
}

func TestAddPlan(t *testing.T) {
	ac := NewAgentContainer("agent", "📝", 0)
	ac.AddPlan("step 1, step 2")

	require.Len(t, ac.Items, 1)
	item, ok := ac.Items[0].(*PlanItem)
	require.True(t, ok)
	assert.Equal(t, "step 1, step 2", item.Text)
}

func TestCollapsedSummary(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)
	ac.Done = true
	summary := ac.CollapsedSummary()

	assert.Contains(t, summary, "🤖 agent")
	assert.Contains(t, summary, "Finished in")
}

func TestViewThinking(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)
	// No items, should show "is thinking..."
	view := ac.View(80)
	assert.Contains(t, view, "is thinking...")
}

func TestAdvanceSpinners(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 0)
	ac.AddToolCall("call-1", "fs_read", `{}`)

	assert.Equal(t, 0, ac.FrameIdx)
	ac.AdvanceSpinners()
	assert.Equal(t, 1, ac.FrameIdx)
}

func TestSubAgentNesting(t *testing.T) {
	parent := NewAgentContainer("parent", "🤖", 0)
	child := NewAgentContainer("child", "🦾", 4)
	sa := &SubAgentItem{Container: child}
	parent.Items = append(parent.Items, sa)

	assert.True(t, sa.IsLive())
	assert.Equal(t, "sub_agent", sa.Kind())

	child.Done = true
	assert.False(t, sa.IsLive())
}

func TestWindowedView(t *testing.T) {
	ac := NewAgentContainer("agent", "🤖", 2)
	ac.AddThinking("text1")
	ac.AddThinking("text2")
	ac.AddThinking("text3")

	view := ac.View(80)
	assert.Contains(t, view, "... 1 more items")
}
