package chatview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
)

// newTestChatView creates a ChatViewModel with a sensible size for testing.
func newTestChatView() ChatViewModel {
	cv := New()
	cv, _ = cv.Update(msgs.ChatViewSetWidthMsg{Width: 80})
	cv, _ = cv.Update(msgs.ChatViewSetHeightMsg{Height: 40})
	return cv
}

func TestChatViewEmpty(t *testing.T) {
	cv := newTestChatView()
	view := cv.View()
	// No committed or live content — viewport should be empty.
	assert.Empty(t, strings.TrimSpace(view))
}

func TestChatViewUserMessage(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.ChatViewCommitUserMsg{Text: "hello world"})

	assert.True(t, cv.HasMessages)
	assert.Len(t, cv.committed, 1)
	assert.Contains(t, cv.committed[0], "hello world")
}

func TestChatViewUserMessageWithImageAttachment(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.ChatViewCommitUserMsg{
		Text: "check this image",
		Parts: []content.Part{
			content.Image{Data: make([]byte, 2048), MediaType: "image/png"},
		},
	})

	assert.True(t, cv.HasMessages)
	assert.Len(t, cv.committed, 1)
	assert.Contains(t, cv.committed[0], "check this image")
	assert.Contains(t, cv.committed[0], "[Image: image/png (2.0 KB)]")
}

func TestChatViewUserMessageWithDocumentAttachment(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.ChatViewCommitUserMsg{
		Text: "review this",
		Parts: []content.Part{
			content.Document{Path: "/tmp/report.pdf", Data: make([]byte, 150*1024), MediaType: "application/pdf"},
		},
	})

	assert.True(t, cv.HasMessages)
	assert.Len(t, cv.committed, 1)
	assert.Contains(t, cv.committed[0], "review this")
	assert.Contains(t, cv.committed[0], "[Document: /tmp/report.pdf (150.0 KB)]")
}

func TestChatViewUserMessageWithMultipleAttachments(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.ChatViewCommitUserMsg{
		Text: "files",
		Parts: []content.Part{
			content.Image{Data: make([]byte, 1024), MediaType: "image/jpeg"},
			content.Document{Path: "doc.pdf", Data: make([]byte, 500), MediaType: "application/pdf"},
		},
	})

	assert.Len(t, cv.committed, 1)
	assert.Contains(t, cv.committed[0], "[Image: image/jpeg (1.0 KB)]")
	assert.Contains(t, cv.committed[0], "[Document: doc.pdf (500 B)]")
}

func TestChatViewAssistantFinalAnswer(t *testing.T) {
	cv := newTestChatView()

	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "assistant", Prefix: "🤖"})
	msg := message.NewText("assistant", role.Assistant, "Here is my answer")
	cv, _ = cv.Update(msgs.ChatMessageMsg{Msg: msg})
	cv, _ = cv.Update(msgs.AgentEndMsg{Agent: "assistant"})

	// Summary should be in the committed buffer.
	assert.NotEmpty(t, cv.committed)
}

func TestChatViewAssistantToolCalls(t *testing.T) {
	cv := newTestChatView()

	msg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"test.txt"}`},
	)
	cv, _ = cv.Update(msgs.ChatMessageMsg{Msg: msg})

	ac, ok := cv.agents["assistant"]
	assert.True(t, ok)
	assert.Len(t, ac.Items, 1)
}

func TestChatViewToolResult(t *testing.T) {
	cv := newTestChatView()

	// Agent makes a tool call.
	callMsg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"test.txt"}`},
	)
	cv, _ = cv.Update(msgs.ChatMessageMsg{Msg: callMsg})

	// Tool result arrives.
	resultMsg := message.New("assistant", role.Tool,
		content.ToolResult{ToolCallID: "tc-1", Content: "file contents", IsError: false},
	)
	cv, _ = cv.Update(msgs.ChatMessageMsg{Msg: resultMsg})

	ac := cv.agents["assistant"]
	tc, ok := ac.Items[0].(*ToolCallItem)
	assert.True(t, ok)
	assert.True(t, tc.Completed)
	assert.Equal(t, "file contents", tc.Result)
}

func TestChatViewParallelToolCalls(t *testing.T) {
	cv := newTestChatView()

	// Multiple calls of the same tool → grouped.
	msg := message.New("assistant", role.Assistant,
		content.ToolCall{ID: "tc-1", Name: "fs_read", Arguments: `{"path":"a.txt"}`},
		content.ToolCall{ID: "tc-2", Name: "fs_read", Arguments: `{"path":"b.txt"}`},
	)
	cv, _ = cv.Update(msgs.ChatMessageMsg{Msg: msg})

	ac := cv.agents["assistant"]
	assert.Len(t, ac.Items, 1) // single group
	tg, ok := ac.Items[0].(*ToolGroupItem)
	assert.True(t, ok)
	assert.Len(t, tg.Calls, 2)
}

func TestChatViewStartEndAgent(t *testing.T) {
	cv := newTestChatView()

	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "myAgent", Prefix: "🤖"})
	assert.Contains(t, cv.agents, "myAgent")
	assert.Len(t, cv.agentOrder, 1)

	cv, _ = cv.Update(msgs.AgentEndMsg{Agent: "myAgent"})
	assert.NotContains(t, cv.agents, "myAgent")
	// Summary should be committed to the buffer.
	assert.NotEmpty(t, cv.committed)
}

func TestChatViewSubAgent(t *testing.T) {
	cv := newTestChatView()

	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "parent", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "parent"})

	// Child should be in subAgents, not top-level.
	assert.NotContains(t, cv.agents, "child")
	assert.Contains(t, cv.subAgents, "child")

	// Parent should have a sub-agent item.
	parentAC := cv.agents["parent"]
	assert.Len(t, parentAC.Items, 1)
	_, ok := parentAC.Items[0].(*AgentContainer)
	assert.True(t, ok)

	// End child.
	cv, _ = cv.Update(msgs.AgentEndMsg{Agent: "child", Parent: "parent"})
	assert.NotContains(t, cv.subAgents, "child")
}

func TestChatViewIgnoreSystemAndUser(t *testing.T) {
	cv := newTestChatView()

	cv, cmd := cv.Update(msgs.ChatMessageMsg{Msg: message.NewText("sys", role.System, "system prompt")})
	assert.Nil(t, cmd)

	_, cmd = cv.Update(msgs.ChatMessageMsg{Msg: message.NewText("user", role.User, "user message")})
	assert.Nil(t, cmd)
}

func TestChatViewProcessingSpinner(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.ChatViewSetProcessingMsg{Processing: true})

	// Advance spinners + rebuild to get content into viewport.
	cv, _ = cv.Update(msgs.ChatViewAdvanceSpinnersMsg{})
	view := cv.View()
	assert.Contains(t, view, cv.ProcessingMsg)
}

func TestChatViewClear(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.ChatViewCommitUserMsg{Text: "hello"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "agent", Prefix: "🤖"})

	cv, _ = cv.Update(msgs.ChatViewClearMsg{})

	assert.Empty(t, cv.agents)
	assert.Empty(t, cv.subAgents)
	assert.False(t, cv.HasMessages)
	assert.Empty(t, cv.committed)
}

func TestChatViewAppendMsg(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.ChatViewAppendMsg{Content: "hello from append"})

	assert.Len(t, cv.committed, 1)
	assert.Equal(t, "hello from append", cv.committed[0])
	assert.Contains(t, cv.View(), "hello from append")
}

// --- Phase 1: Sub-Agent Data API tests ---

func TestSubAgents_Empty(t *testing.T) {
	cv := newTestChatView()
	assert.Nil(t, cv.SubAgents())
}

func TestSubAgents_FlatList(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child-a", Prefix: "🦾", Parent: "root", ProviderLabel: "anthropic/claude-sonnet-4"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child-b", Prefix: "🦾", Parent: "root", ProviderLabel: "openai/gpt-4o"})

	infos := cv.SubAgents()
	assert.Len(t, infos, 2)

	// Sorted by ID.
	assert.Equal(t, "child-a", infos[0].ID)
	assert.Equal(t, "child-a", infos[0].Label)
	assert.Equal(t, "anthropic/claude-sonnet-4", infos[0].Provider)
	assert.Equal(t, "running", infos[0].Status)
	assert.Equal(t, "root", infos[0].ParentID)
	assert.Equal(t, 0, infos[0].Depth)
	assert.NotEmpty(t, infos[0].Color)

	assert.Equal(t, "child-b", infos[1].ID)
	assert.Equal(t, 0, infos[1].Depth)
}

func TestSubAgents_NestedDepth(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "grandchild", Prefix: "🦾", Parent: "child"})

	infos := cv.SubAgents()
	assert.Len(t, infos, 2)

	// child: direct child of root → depth 0
	childInfo := findInfoByID(infos, "child")
	assert.Equal(t, 0, childInfo.Depth)
	assert.Equal(t, "root", childInfo.ParentID)

	// grandchild: child of child → depth 1
	gcInfo := findInfoByID(infos, "grandchild")
	assert.Equal(t, 1, gcInfo.Depth)
	assert.Equal(t, "child", gcInfo.ParentID)
}

func TestSubAgents_ExcludesCompleted(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child-a", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child-b", Prefix: "🦾", Parent: "root"})

	// End child-a.
	cv, _ = cv.Update(msgs.AgentEndMsg{Agent: "child-a", Parent: "root"})

	infos := cv.SubAgents()
	assert.Len(t, infos, 1)
	assert.Equal(t, "child-b", infos[0].ID)
}

func TestFindContainer_Exists(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})

	// Top-level agent.
	rootAC := cv.FindContainer("root")
	assert.NotNil(t, rootAC)
	assert.Equal(t, "root", rootAC.Agent)

	// Sub-agent.
	childAC := cv.FindContainer("child")
	assert.NotNil(t, childAC)
	assert.Equal(t, "child", childAC.Agent)
}

func TestFindContainer_NotFound(t *testing.T) {
	cv := newTestChatView()
	assert.Nil(t, cv.FindContainer("nonexistent"))
}

func findInfoByID(infos []SubAgentInfo, id string) SubAgentInfo {
	for _, info := range infos {
		if info.ID == id {
			return info
		}
	}
	return SubAgentInfo{}
}

// --- Phase 6: Agent-Scoped Chat View tests ---

func TestFocusAgent_PushesStack(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})

	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})

	assert.Equal(t, "child", cv.ViewedAgent())
	assert.Len(t, cv.viewStack, 1)
	assert.Equal(t, "child", cv.viewStack[0].AgentID)
	assert.NotNil(t, cv.viewStack[0].Container)
}

func TestFocusAgent_NonexistentAgent(t *testing.T) {
	cv := newTestChatView()

	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "ghost"})

	assert.Empty(t, cv.ViewedAgent())
	assert.Empty(t, cv.viewStack)
}

func TestFocusAgent_DepthCapReached(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})

	// Create enough sub-agents and push them to fill the stack.
	for i := range maxViewStackDepth {
		name := fmt.Sprintf("agent-%d", i)
		parent := "root"
		if i > 0 {
			parent = fmt.Sprintf("agent-%d", i-1)
		}
		cv, _ = cv.Update(msgs.AgentStartMsg{Agent: name, Prefix: "🦾", Parent: parent})
		cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: name})
	}
	assert.Len(t, cv.viewStack, maxViewStackDepth)

	// One more should be rejected.
	extraName := "agent-overflow"
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: extraName, Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: extraName})
	assert.Len(t, cv.viewStack, maxViewStackDepth) // unchanged
}

func TestNavigateBack_PopsStack(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})

	cv, _ = cv.Update(msgs.ChatViewNavigateBackMsg{})

	assert.Empty(t, cv.ViewedAgent())
	assert.Empty(t, cv.viewStack)
}

func TestNavigateBack_AtRoot_Noop(t *testing.T) {
	cv := newTestChatView()

	cv, _ = cv.Update(msgs.ChatViewNavigateBackMsg{})

	assert.Empty(t, cv.ViewedAgent())
	assert.Empty(t, cv.viewStack)
}

func TestNavigateBack_MultiLevel(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "grandchild", Prefix: "🦾", Parent: "child"})

	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "grandchild"})
	assert.Equal(t, "grandchild", cv.ViewedAgent())
	assert.Len(t, cv.viewStack, 2)

	cv, _ = cv.Update(msgs.ChatViewNavigateBackMsg{})
	assert.Equal(t, "child", cv.ViewedAgent())
	assert.Len(t, cv.viewStack, 1)

	cv, _ = cv.Update(msgs.ChatViewNavigateBackMsg{})
	assert.Empty(t, cv.ViewedAgent())
	assert.Empty(t, cv.viewStack)
}

func TestViewStack_CleanupOnAgentEnd(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child-a", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child-b", Prefix: "🦾", Parent: "root"})

	// Focus child-a, then go back, then focus child-b.
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child-a"})
	cv, _ = cv.Update(msgs.ChatViewNavigateBackMsg{})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child-b"})

	// child-a ends while viewing child-b.
	cv, _ = cv.Update(msgs.AgentEndMsg{Agent: "child-a", Parent: "root"})

	// child-a should be cleaned from the stack (it's not currently viewed).
	for _, entry := range cv.viewStack {
		assert.NotEqual(t, "child-a", entry.AgentID)
	}
}

func TestViewStack_AgentEndsWhileViewing(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})

	// Get a reference to the container before it's removed.
	container := cv.viewStack[0].Container
	assert.NotNil(t, container)

	// Agent ends while we're viewing it.
	cv, _ = cv.Update(msgs.AgentEndMsg{Agent: "child", Parent: "root"})

	// View stack should still have the entry (pinned pointer).
	assert.Equal(t, "child", cv.ViewedAgent())
	assert.Len(t, cv.viewStack, 1)
	assert.NotNil(t, cv.viewStack[0].Container)
	assert.True(t, cv.viewStack[0].Container.Done)
}

func TestBreadcrumb_Rendering(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})

	bc := cv.RenderBreadcrumb()
	assert.Contains(t, bc, "root")
	assert.Contains(t, bc, "child")
}

func TestBreadcrumb_EmptyAtRoot(t *testing.T) {
	cv := newTestChatView()
	assert.Empty(t, cv.RenderBreadcrumb())
}

func TestBreadcrumb_ReducesViewportHeight(t *testing.T) {
	cv := newTestChatView()
	assert.Equal(t, 0, cv.HeaderHeight())

	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})

	assert.Equal(t, 1, cv.HeaderHeight())
}

func TestClear_ResetsViewStack(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})
	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})

	cv, _ = cv.Update(msgs.ChatViewClearMsg{})

	assert.Empty(t, cv.ViewedAgent())
	assert.Empty(t, cv.viewStack)
}

func TestAgentScoped_MaxShowZero(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "root", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "child", Prefix: "🦾", Parent: "root"})

	// Add several items to the child container.
	childAC := cv.FindContainer("child")
	for i := range 10 {
		childAC.AddToolCall(fmt.Sprintf("tc-%d", i), "test_tool", `{"arg":"val"}`)
	}
	// Sub-agents default MaxShow=4; viewing should show all.
	assert.Equal(t, 4, childAC.MaxShow)

	cv, _ = cv.Update(msgs.ChatViewFocusAgentMsg{AgentID: "child"})
	view := cv.View()

	// All 10 tool calls should be visible (no "... more items" truncation).
	assert.NotContains(t, view, "more items")
}

func TestChatViewFlushAll(t *testing.T) {
	cv := newTestChatView()
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "a1", Prefix: "🤖"})
	cv, _ = cv.Update(msgs.AgentStartMsg{Agent: "a2", Prefix: "🤖"})

	cv, _ = cv.Update(msgs.ChatViewFlushAllMsg{})

	assert.Empty(t, cv.agents)
	assert.Empty(t, cv.agentOrder)
	// Summaries should be in committed buffer.
	assert.NotEmpty(t, cv.committed)
}
