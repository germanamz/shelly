package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListAgentsToolExcludesSelf(t *testing.T) {
	reg := NewRegistry()
	reg.Register("self", "Self agent", func() *Agent { return &Agent{} })
	reg.Register("other", "Other agent", func() *Agent { return &Agent{} })

	a := &Agent{name: "self", registry: reg}
	tool := listAgentsTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))

	require.NoError(t, err)

	var entries []Entry
	require.NoError(t, json.Unmarshal([]byte(result), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "other", entries[0].Name)
}

func TestDelegateToolInvalidInput(t *testing.T) {
	a := &Agent{name: "orch", registry: NewRegistry()}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestDelegateToolSelfDelegation(t *testing.T) {
	a := &Agent{name: "orch", registry: NewRegistry()}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"agent":"orch","task":"loop","context":""}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-delegation")
}

func TestDelegateToolMaxDepth(t *testing.T) {
	a := &Agent{
		name:     "orch",
		registry: NewRegistry(),
		options:  Options{MaxDelegationDepth: 2},
		depth:    2,
	}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"agent":"worker","task":"do","context":""}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max delegation depth")
}

func TestDelegateToolAgentNotFound(t *testing.T) {
	a := &Agent{name: "orch", registry: NewRegistry()}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"agent":"missing","task":"do","context":""}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDelegateToolSuccess(t *testing.T) {
	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done by worker"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New()}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"agent":"worker","task":"do the thing","context":"some context"}`))

	require.NoError(t, err)
	assert.Equal(t, "done by worker", result)
}

func TestSpawnToolEmptyTasks(t *testing.T) {
	a := &Agent{name: "orch", registry: NewRegistry()}
	tool := spawnTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[]}`))

	require.NoError(t, err)
	assert.Equal(t, "[]", result)
}

func TestSpawnToolSelfDelegation(t *testing.T) {
	a := &Agent{name: "orch", registry: NewRegistry()}
	tool := spawnTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"orch","task":"loop","context":""}]}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-delegation")
}

func TestSpawnToolSuccess(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", "Agent A", func() *Agent {
		return New("a", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "result-a"),
			},
		}, Options{})
	})
	reg.Register("b", "Agent B", func() *Agent {
		return New("b", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "result-b"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New()}
	tool := spawnTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"a","task":"task-a","context":"ctx-a"},{"agent":"b","task":"task-b","context":"ctx-b"}]}`,
	))

	require.NoError(t, err)

	var results []spawnResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 2)
	assert.Equal(t, "a", results[0].Agent)
	assert.Equal(t, "result-a", results[0].Result)
	assert.Equal(t, "b", results[1].Agent)
	assert.Equal(t, "result-b", results[1].Result)
}

func TestSpawnToolAgentNotFound(t *testing.T) {
	reg := NewRegistry()

	a := &Agent{name: "orch", registry: reg, chat: chat.New()}
	tool := spawnTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"missing","task":"do","context":""}]}`,
	))

	require.NoError(t, err)

	var results []spawnResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "not found")
}

func TestOrchestrationToolBoxRegistered(t *testing.T) {
	reg := NewRegistry()
	a := &Agent{name: "orch", registry: reg}

	tb := orchestrationToolBox(a)
	tools := tb.Tools()

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	assert.True(t, names["list_agents"])
	assert.True(t, names["delegate_to_agent"])
	assert.True(t, names["spawn_agents"])
}

func TestDelegateToolChildGetsRegistry(t *testing.T) {
	// Worker that tries to list agents - proves it got the registry.
	workerCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "list_agents",
					Arguments: `{}`,
				},
			),
			message.NewText("", role.Assistant, "listed"),
		},
	}

	reg := NewRegistry()
	reg.Register("worker", "Worker", func() *Agent {
		return New("worker", "", "", workerCompleter, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New()}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"agent":"worker","task":"list them","context":"some context"}`))

	require.NoError(t, err)
	assert.Equal(t, "listed", result)
}

func TestSpawnToolResilientErrors(t *testing.T) {
	reg := NewRegistry()
	reg.Register("ok", "OK agent", func() *Agent {
		return New("ok", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "success"),
			},
		}, Options{})
	})
	reg.Register("fail", "Fail agent", func() *Agent {
		return New("fail", "", "", &errorCompleter{
			err: errors.New("boom"),
		}, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New()}
	tool := spawnTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"ok","task":"go","context":""},{"agent":"fail","task":"go","context":""},{"agent":"missing","task":"go","context":""}]}`,
	))

	require.NoError(t, err)

	var results []spawnResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 3)

	// ok agent succeeds.
	assert.Equal(t, "ok", results[0].Agent)
	assert.Equal(t, "success", results[0].Result)
	assert.Empty(t, results[0].Error)

	// fail agent returns error but doesn't cancel others.
	assert.Equal(t, "fail", results[1].Agent)
	assert.Contains(t, results[1].Error, "boom")

	// missing agent returns not found error.
	assert.Equal(t, "missing", results[2].Agent)
	assert.Contains(t, results[2].Error, "not found")
}

func TestSpawnToolToolboxInheritance(t *testing.T) {
	parentTB := toolbox.New()
	parentTB.Register(toolbox.Tool{
		Name:        "inherited_tool",
		Description: "Inherited from parent",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "inherited_result", nil
		},
	})

	// Worker calls inherited_tool.
	reg := NewRegistry()
	reg.Register("worker", "Worker", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{ID: "c1", Name: "inherited_tool", Arguments: `{}`},
				),
				message.NewText("", role.Assistant, "done with inherited"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{parentTB}}
	tool := spawnTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"use inherited tool","context":"inherited context"}]}`,
	))

	require.NoError(t, err)

	var results []spawnResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "worker", results[0].Agent)
	assert.Equal(t, "done with inherited", results[0].Result)
	assert.Empty(t, results[0].Error)
}

func TestDelegateToolWithContext(t *testing.T) {
	var capturedMessages []message.Message

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &capturingCompleter{
			capture: &capturedMessages,
			reply:   message.NewText("", role.Assistant, "done"),
		}, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New()}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"agent":"worker","task":"do the thing","context":"file.go contains X\nconstraint: no breaking changes"}`,
	))

	require.NoError(t, err)
	assert.Equal(t, "done", result)

	// Child should have 2 user messages: context then task.
	var userMsgs []message.Message
	for _, m := range capturedMessages {
		if m.Role == role.User {
			userMsgs = append(userMsgs, m)
		}
	}
	require.Len(t, userMsgs, 2)
	assert.Contains(t, userMsgs[0].TextContent(), "<delegation_context>")
	assert.Contains(t, userMsgs[0].TextContent(), "file.go contains X")
	assert.Equal(t, "do the thing", userMsgs[1].TextContent())
}

func TestSpawnToolWithContext(t *testing.T) {
	var capturedA []message.Message
	var capturedB []message.Message

	reg := NewRegistry()
	reg.Register("a", "Agent A", func() *Agent {
		return New("a", "", "", &capturingCompleter{
			capture: &capturedA,
			reply:   message.NewText("", role.Assistant, "result-a"),
		}, Options{})
	})
	reg.Register("b", "Agent B", func() *Agent {
		return New("b", "", "", &capturingCompleter{
			capture: &capturedB,
			reply:   message.NewText("", role.Assistant, "result-b"),
		}, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New()}
	tool := spawnTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"a","task":"task-a","context":"background for a"},{"agent":"b","task":"task-b","context":"background for b"}]}`,
	))

	require.NoError(t, err)

	var results []spawnResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 2)

	// Agent "a": context then task.
	var userMsgsA []message.Message
	for _, m := range capturedA {
		if m.Role == role.User {
			userMsgsA = append(userMsgsA, m)
		}
	}
	require.Len(t, userMsgsA, 2)
	assert.Contains(t, userMsgsA[0].TextContent(), "<delegation_context>")
	assert.Contains(t, userMsgsA[0].TextContent(), "background for a")
	assert.Equal(t, "task-a", userMsgsA[1].TextContent())

	// Agent "b": context then task.
	var userMsgsB []message.Message
	for _, m := range capturedB {
		if m.Role == role.User {
			userMsgsB = append(userMsgsB, m)
		}
	}
	require.Len(t, userMsgsB, 2)
	assert.Contains(t, userMsgsB[0].TextContent(), "<delegation_context>")
	assert.Contains(t, userMsgsB[0].TextContent(), "background for b")
	assert.Equal(t, "task-b", userMsgsB[1].TextContent())
}

func TestDelegateToolToolboxInheritance(t *testing.T) {
	parentTB := toolbox.New()
	parentTB.Register(toolbox.Tool{
		Name:        "inherited_tool",
		Description: "Inherited from parent",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "inherited_result", nil
		},
	})

	// Worker calls inherited_tool.
	reg := NewRegistry()
	reg.Register("worker", "Worker", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{ID: "c1", Name: "inherited_tool", Arguments: `{}`},
				),
				message.NewText("", role.Assistant, "done with inherited"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{parentTB}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"agent":"worker","task":"use inherited tool","context":"inherited context"}`,
	))

	require.NoError(t, err)
	assert.Equal(t, "done with inherited", result)
}
