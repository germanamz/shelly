package main

import (
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
)

func TestChatViewEmpty(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)
	view := cv.View()
	assert.Contains(t, view, "shelly")
}

func TestChatViewUserMessage(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)
	cv.commitUserMessage("hello world")

	assert.True(t, cv.hasMessages)
	assert.Contains(t, cv.committed.String(), "User")
	assert.Contains(t, cv.committed.String(), "hello world")
}

func TestChatViewAssistantFinalAnswer(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)

	msg := message.NewText("assistant", role.Assistant, "Here is my answer")
	cv.addMessage(msg)

	assert.Contains(t, cv.committed.String(), "Here is my answer")
}

func TestChatViewAssistantToolCalls(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)

	msg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"test.txt"}`},
	)
	cv.addMessage(msg)

	ac, ok := cv.agents["assistant"]
	assert.True(t, ok)
	assert.Len(t, ac.items, 1)
}

func TestChatViewToolResult(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)

	// Agent makes a tool call.
	callMsg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"test.txt"}`},
	)
	cv.addMessage(callMsg)

	// Tool result arrives.
	resultMsg := message.New("assistant", role.Tool,
		content.ToolResult{ToolCallID: "tc-1", Content: "file contents", IsError: false},
	)
	cv.addMessage(resultMsg)

	ac := cv.agents["assistant"]
	tc, ok := ac.items[0].(*toolCallItem)
	assert.True(t, ok)
	assert.True(t, tc.completed)
	assert.Equal(t, "file contents", tc.result)
}

func TestChatViewParallelToolCalls(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)

	// Multiple calls of the same tool → grouped.
	msg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"a.txt"}`},
		content.ToolCall{ID: "tc-2", Name: "fs_read", Arguments: `{"path":"b.txt"}`},
	)
	cv.addMessage(msg)

	ac := cv.agents["assistant"]
	assert.Len(t, ac.items, 1) // single group
	tg, ok := ac.items[0].(*toolGroupItem)
	assert.True(t, ok)
	assert.Len(t, tg.calls, 2)
}

func TestChatViewStartEndAgent(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)

	cv.startAgent("myAgent", "🤖", "")
	assert.Contains(t, cv.agents, "myAgent")
	assert.Len(t, cv.agentOrder, 1)

	cv.endAgent("myAgent", "")
	assert.NotContains(t, cv.agents, "myAgent")
	assert.Contains(t, cv.committed.String(), "Finished in")
}

func TestChatViewSubAgent(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)

	cv.startAgent("parent", "🤖", "")
	cv.startAgent("child", "🦾", "parent")

	// Child should be in subAgents, not top-level.
	assert.NotContains(t, cv.agents, "child")
	assert.Contains(t, cv.subAgents, "child")

	// Parent should have a sub-agent item.
	parentAC := cv.agents["parent"]
	assert.Len(t, parentAC.items, 1)
	_, ok := parentAC.items[0].(*subAgentItem)
	assert.True(t, ok)

	// End child.
	cv.endAgent("child", "parent")
	assert.NotContains(t, cv.subAgents, "child")
}

func TestChatViewIgnoreSystemAndUser(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)

	cmd := cv.addMessage(message.NewText("sys", role.System, "system prompt"))
	assert.Nil(t, cmd)

	cmd = cv.addMessage(message.NewText("user", role.User, "user message"))
	assert.Nil(t, cmd)
}

func TestChatViewProcessingSpinner(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)
	cv.hasMessages = true
	cv.setProcessing(true)

	view := cv.View()
	assert.Contains(t, view, cv.processingMsg)
}

func TestChatViewClear(t *testing.T) {
	cv := newChatView()
	cv.setSize(80, 24)
	cv.commitUserMessage("hello")
	cv.startAgent("agent", "🤖", "")

	cv.Clear()

	assert.Empty(t, cv.agents)
	assert.Empty(t, cv.subAgents)
	assert.Empty(t, cv.committed.String())
	assert.False(t, cv.hasMessages)
}
