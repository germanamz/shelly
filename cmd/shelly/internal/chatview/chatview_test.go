package chatview

import (
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
)

func TestChatViewEmpty(t *testing.T) {
	cv := New()
	cv.SetWidth(80)
	view := cv.View()
	// No live content — managed area is empty; logo is printed via tea.Println at startup.
	assert.Empty(t, view)
}

func TestChatViewUserMessage(t *testing.T) {
	cv := New()
	cv.SetWidth(80)
	cmd := cv.CommitUserMessage("hello world")

	assert.True(t, cv.HasMessages)
	assert.NotNil(t, cmd) // content emitted as tea.Println cmd
}

func TestChatViewAssistantFinalAnswer(t *testing.T) {
	cv := New()
	cv.SetWidth(80)

	cv.StartAgent("assistant", "🤖", "")
	msg := message.NewText("assistant", role.Assistant, "Here is my answer")
	cv.AddMessage(msg)
	cmd := cv.EndAgent("assistant", "")

	assert.NotNil(t, cmd) // collapsed summary emitted as tea.Println cmd
}

func TestChatViewAssistantToolCalls(t *testing.T) {
	cv := New()
	cv.SetWidth(80)

	msg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"test.txt"}`},
	)
	cv.AddMessage(msg)

	ac, ok := cv.agents["assistant"]
	assert.True(t, ok)
	assert.Len(t, ac.Items, 1)
}

func TestChatViewToolResult(t *testing.T) {
	cv := New()
	cv.SetWidth(80)

	// Agent makes a tool call.
	callMsg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"test.txt"}`},
	)
	cv.AddMessage(callMsg)

	// Tool result arrives.
	resultMsg := message.New("assistant", role.Tool,
		content.ToolResult{ToolCallID: "tc-1", Content: "file contents", IsError: false},
	)
	cv.AddMessage(resultMsg)

	ac := cv.agents["assistant"]
	tc, ok := ac.Items[0].(*ToolCallItem)
	assert.True(t, ok)
	assert.True(t, tc.Completed)
	assert.Equal(t, "file contents", tc.Result)
}

func TestChatViewParallelToolCalls(t *testing.T) {
	cv := New()
	cv.SetWidth(80)

	// Multiple calls of the same tool → grouped.
	msg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"a.txt"}`},
		content.ToolCall{ID: "tc-2", Name: "fs_read", Arguments: `{"path":"b.txt"}`},
	)
	cv.AddMessage(msg)

	ac := cv.agents["assistant"]
	assert.Len(t, ac.Items, 1) // single group
	tg, ok := ac.Items[0].(*ToolGroupItem)
	assert.True(t, ok)
	assert.Len(t, tg.Calls, 2)
}

func TestChatViewStartEndAgent(t *testing.T) {
	cv := New()
	cv.SetWidth(80)

	cv.StartAgent("myAgent", "🤖", "")
	assert.Contains(t, cv.agents, "myAgent")
	assert.Len(t, cv.agentOrder, 1)

	cmd := cv.EndAgent("myAgent", "")
	assert.NotContains(t, cv.agents, "myAgent")
	assert.NotNil(t, cmd) // summary emitted as tea.Println cmd
}

func TestChatViewSubAgent(t *testing.T) {
	cv := New()
	cv.SetWidth(80)

	cv.StartAgent("parent", "🤖", "")
	cv.StartAgent("child", "🦾", "parent")

	// Child should be in subAgents, not top-level.
	assert.NotContains(t, cv.agents, "child")
	assert.Contains(t, cv.subAgents, "child")

	// Parent should have a sub-agent item.
	parentAC := cv.agents["parent"]
	assert.Len(t, parentAC.Items, 1)
	_, ok := parentAC.Items[0].(*SubAgentItem)
	assert.True(t, ok)

	// End child.
	cv.EndAgent("child", "parent")
	assert.NotContains(t, cv.subAgents, "child")
}

func TestChatViewIgnoreSystemAndUser(t *testing.T) {
	cv := New()
	cv.SetWidth(80)

	cmd := cv.AddMessage(message.NewText("sys", role.System, "system prompt"))
	assert.Nil(t, cmd)

	cmd = cv.AddMessage(message.NewText("user", role.User, "user message"))
	assert.Nil(t, cmd)
}

func TestChatViewProcessingSpinner(t *testing.T) {
	cv := New()
	cv.SetWidth(80)
	cv.SetProcessing(true)

	view := cv.View()
	assert.Contains(t, view, cv.ProcessingMsg)
}

func TestChatViewClear(t *testing.T) {
	cv := New()
	cv.SetWidth(80)
	cv.CommitUserMessage("hello")
	cv.StartAgent("agent", "🤖", "")

	cv.Clear()

	assert.Empty(t, cv.agents)
	assert.Empty(t, cv.subAgents)
	assert.False(t, cv.HasMessages)
}
