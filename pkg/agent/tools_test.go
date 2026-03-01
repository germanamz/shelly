package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	a := &Agent{name: "self", configName: "self", registry: reg}
	tool := listAgentsTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))

	require.NoError(t, err)

	var entries []Entry
	require.NoError(t, json.Unmarshal([]byte(result), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "other", entries[0].Name)
}

func TestListAgentsToolExcludesSelfCaseInsensitive(t *testing.T) {
	reg := NewRegistry()
	reg.Register("Self", "Self agent", func() *Agent { return &Agent{} })
	reg.Register("other", "Other agent", func() *Agent { return &Agent{} })

	a := &Agent{name: "self", configName: "self", registry: reg}
	tool := listAgentsTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))

	require.NoError(t, err)

	var entries []Entry
	require.NoError(t, json.Unmarshal([]byte(result), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "other", entries[0].Name)
}

func TestDelegateToolInvalidInput(t *testing.T) {
	a := &Agent{name: "orch", configName: "orch", registry: NewRegistry()}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestDelegateToolSelfDelegation(t *testing.T) {
	a := &Agent{name: "orch", configName: "orch", registry: NewRegistry()}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"orch","task":"loop","context":""}]}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-delegation")
}

func TestDelegateToolSelfDelegationCaseInsensitive(t *testing.T) {
	a := &Agent{name: "Orchestrator", configName: "Orchestrator", registry: NewRegistry()}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"orchestrator","task":"loop","context":""}]}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-delegation")
}

func TestDelegateToolMaxDepth(t *testing.T) {
	a := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   NewRegistry(),
		options:    Options{MaxDelegationDepth: 2},
		depth:      2,
	}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"worker","task":"do","context":""}]}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max delegation depth")
}

func TestDelegateToolZeroDepthBlocked(t *testing.T) {
	a := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   NewRegistry(),
		options:    Options{MaxDelegationDepth: 0},
		depth:      0,
	}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"worker","task":"do","context":""}]}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max delegation depth")
}

func TestDelegateToolAgentNotFound(t *testing.T) {
	reg := NewRegistry()

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"missing","task":"do","context":""}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "not found")
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"some context"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "worker", results[0].Agent)
	assert.Equal(t, "done by worker", results[0].Result)
}

func TestDelegateToolEmptyTasks(t *testing.T) {
	a := &Agent{name: "orch", configName: "orch", registry: NewRegistry()}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[]}`))

	require.NoError(t, err)
	assert.Equal(t, "[]", result)
}

func TestDelegateToolConcurrentSuccess(t *testing.T) {
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"a","task":"task-a","context":"ctx-a"},{"agent":"b","task":"task-b","context":"ctx-b"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 2)
	assert.Equal(t, "a", results[0].Agent)
	assert.Equal(t, "result-a", results[0].Result)
	assert.Equal(t, "b", results[1].Agent)
	assert.Equal(t, "result-b", results[1].Result)
}

func TestOrchestrationToolBoxRegistered(t *testing.T) {
	reg := NewRegistry()
	a := &Agent{name: "orch", configName: "orch", registry: reg}

	tb := orchestrationToolBox(a)
	tools := tb.Tools()

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	assert.True(t, names["list_agents"])
	assert.True(t, names["delegate"])
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
		return New("worker", "", "", workerCompleter, Options{MaxDelegationDepth: 1})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"list them","context":"some context"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "listed", results[0].Result)
}

func TestDelegateToolResilientErrors(t *testing.T) {
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"ok","task":"go","context":""},{"agent":"fail","task":"go","context":""},{"agent":"missing","task":"go","context":""}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{parentTB}, options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"use inherited tool","context":"inherited context"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "worker", results[0].Agent)
	assert.Equal(t, "done with inherited", results[0].Result)
	assert.Empty(t, results[0].Error)
}

// --- task_id integration tests ---

// mockTaskBoard records ClaimTask and UpdateTaskStatus calls for testing.
// It is safe for concurrent use.
type mockTaskBoard struct {
	mu       sync.Mutex
	claims   []mockClaim
	updates  []mockStatusUpdate
	claimFn  func(id, agent string) error  // optional custom claim behaviour
	updateFn func(id, status string) error // optional custom update behaviour
}

type mockClaim struct {
	ID    string
	Agent string
}

type mockStatusUpdate struct {
	ID     string
	Status string
}

func (m *mockTaskBoard) ClaimTask(id, agent string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claims = append(m.claims, mockClaim{ID: id, Agent: agent})
	if m.claimFn != nil {
		return m.claimFn(id, agent)
	}
	return nil
}

func (m *mockTaskBoard) UpdateTaskStatus(id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, mockStatusUpdate{ID: id, Status: status})
	if m.updateFn != nil {
		return m.updateFn(id, status)
	}
	return nil
}

func TestDelegateToolWithTaskID(t *testing.T) {
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"did it"}`,
					},
				),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"some context","task_id":"task-1"}]}`,
	))

	require.NoError(t, err)

	// Verify task was claimed for the child agent (instance name starts with config name).
	require.Len(t, board.claims, 1)
	assert.Equal(t, "task-1", board.claims[0].ID)
	assert.True(t, strings.HasPrefix(board.claims[0].Agent, "worker-"), "expected instance name starting with 'worker-', got %q", board.claims[0].Agent)

	// Verify task status was updated based on completion result.
	require.Len(t, board.updates, 1)
	assert.Equal(t, "task-1", board.updates[0].ID)
	assert.Equal(t, "completed", board.updates[0].Status)

	// Result should contain structured completion.
	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "completed", results[0].Completion.Status)
}

func TestDelegateToolWithTaskIDPreClaimed(t *testing.T) {
	// Simulates Issue 9: orchestrator claims a task, then delegates with task_id.
	// The mock board rejects re-claims by a different agent (like Store.Claim does).
	board := &mockTaskBoard{
		claimFn: func(id, agent string) error {
			// First call (from orchestrator) succeeds. Second call (from delegation
			// to worker) should also succeed because taskBoardAdapter uses Reassign.
			return nil
		},
	}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"did it"}`,
					},
				),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"some context","task_id":"task-1"}]}`,
	))

	require.NoError(t, err)

	// Verify task was claimed for the child agent (instance name starts with config name).
	require.Len(t, board.claims, 1)
	assert.Equal(t, "task-1", board.claims[0].ID)
	assert.True(t, strings.HasPrefix(board.claims[0].Agent, "worker-"), "expected instance name starting with 'worker-', got %q", board.claims[0].Agent)

	// Verify task was completed.
	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "completed", results[0].Completion.Status)
}

func TestDelegateToolWithTaskIDNoBoard(t *testing.T) {
	// task_id provided but no TaskBoard set — graceful no-op.
	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do","context":"ctx","task_id":"task-1"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done", results[0].Result)
}

func TestDelegateToolWithTaskIDNoCompletion(t *testing.T) {
	// Child doesn't call task_complete — task stays in_progress (no status update).
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done without completion"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do","context":"ctx","task_id":"task-1"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done without completion", results[0].Result)

	// Task was claimed but status was not updated (no completion result).
	require.Len(t, board.claims, 1)
	assert.Empty(t, board.updates)
}

func TestDelegateToolPropagatesOptionsToChild(t *testing.T) {
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"failed","summary":"oops"}`,
					},
				),
			},
		}, Options{})
	})

	a := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		options: Options{
			TaskBoard:          board,
			ReflectionDir:      t.TempDir(),
			MaxDelegationDepth: 1,
		},
	}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"ctx","task_id":"task-prop"}]}`,
	))

	require.NoError(t, err)

	// Task was claimed and updated — proves TaskBoard was propagated.
	require.Len(t, board.claims, 1)
	assert.Equal(t, "task-prop", board.claims[0].ID)
	require.Len(t, board.updates, 1)
	assert.Equal(t, "failed", board.updates[0].Status)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "failed", results[0].Completion.Status)
}

func TestDelegateToolConcurrentWithTaskID(t *testing.T) {
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("a", "Agent A", func() *Agent {
		return New("a", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"a done"}`,
					},
				),
			},
		}, Options{})
	})
	reg.Register("b", "Agent B", func() *Agent {
		return New("b", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"failed","summary":"b failed"}`,
					},
				),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"a","task":"task-a","context":"ctx-a","task_id":"task-1"},{"agent":"b","task":"task-b","context":"ctx-b","task_id":"task-2"}]}`,
	))

	require.NoError(t, err)

	// Both tasks should be claimed.
	require.Len(t, board.claims, 2)

	// Both tasks should have status updates.
	require.Len(t, board.updates, 2)

	// Sort for deterministic assertions (concurrent goroutines).
	claimsByID := make(map[string]string)
	for _, c := range board.claims {
		claimsByID[c.ID] = c.Agent
	}
	assert.True(t, strings.HasPrefix(claimsByID["task-1"], "a-"), "expected instance name starting with 'a-', got %q", claimsByID["task-1"])
	assert.True(t, strings.HasPrefix(claimsByID["task-2"], "b-"), "expected instance name starting with 'b-', got %q", claimsByID["task-2"])

	updatesByID := make(map[string]string)
	for _, u := range board.updates {
		updatesByID[u.ID] = u.Status
	}
	assert.Equal(t, "completed", updatesByID["task-1"])
	assert.Equal(t, "failed", updatesByID["task-2"])
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"file.go contains X\nconstraint: no breaking changes"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done", results[0].Result)

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

func TestDelegateToolConcurrentWithContext(t *testing.T) {
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"a","task":"task-a","context":"background for a"},{"agent":"b","task":"task-b","context":"background for b"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
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

// --- task_complete tool tests ---

func TestTaskCompleteToolSuccess(t *testing.T) {
	a := &Agent{name: "worker", configName: "worker", depth: 1}
	tool := taskCompleteTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"completed","summary":"Implemented feature X","files_modified":["foo.go","bar.go"],"tests_run":["TestFoo"],"caveats":"needs docs"}`,
	))

	require.NoError(t, err)
	assert.Equal(t, "Task marked as completed.", result)

	cr := a.CompletionResult()
	require.NotNil(t, cr)
	assert.Equal(t, "completed", cr.Status)
	assert.Equal(t, "Implemented feature X", cr.Summary)
	assert.Equal(t, []string{"foo.go", "bar.go"}, cr.FilesModified)
	assert.Equal(t, []string{"TestFoo"}, cr.TestsRun)
	assert.Equal(t, "needs docs", cr.Caveats)
}

func TestTaskCompleteToolFailed(t *testing.T) {
	a := &Agent{name: "worker", configName: "worker", depth: 1}
	tool := taskCompleteTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"failed","summary":"Could not find the module"}`,
	))

	require.NoError(t, err)
	assert.Equal(t, "Task marked as failed.", result)

	cr := a.CompletionResult()
	require.NotNil(t, cr)
	assert.Equal(t, "failed", cr.Status)
	assert.Equal(t, "Could not find the module", cr.Summary)
}

func TestTaskCompleteToolInvalidStatus(t *testing.T) {
	a := &Agent{name: "worker", configName: "worker", depth: 1}
	tool := taskCompleteTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"unknown","summary":"whatever"}`,
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status must be")
}

func TestTaskCompleteToolInvalidInput(t *testing.T) {
	a := &Agent{name: "worker", configName: "worker", depth: 1}
	tool := taskCompleteTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestTaskCompleteToolDuplicateCallIgnored(t *testing.T) {
	a := &Agent{name: "worker", configName: "worker", depth: 1}
	tool := taskCompleteTool(a)

	result1, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"completed","summary":"first call"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Task marked as completed.", result1)

	// Second call should be ignored and return "already marked".
	result2, err := tool.Handler(context.Background(), json.RawMessage(
		`{"status":"failed","summary":"second call"}`,
	))
	require.NoError(t, err)
	assert.Contains(t, result2, "already marked")

	// Original completion result should be preserved.
	cr := a.CompletionResult()
	require.NotNil(t, cr)
	assert.Equal(t, "completed", cr.Status)
	assert.Equal(t, "first call", cr.Summary)
}

func TestDelegateToolWithCompletion(t *testing.T) {
	// Worker that calls task_complete.
	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"did it","files_modified":["a.go"]}`,
					},
				),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"some context"}]}`,
	))

	require.NoError(t, err)

	// Result should be structured JSON.
	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "completed", results[0].Completion.Status)
	assert.Equal(t, "did it", results[0].Completion.Summary)
	assert.Equal(t, []string{"a.go"}, results[0].Completion.FilesModified)
}

func TestDelegateToolWithoutCompletion(t *testing.T) {
	// Worker that stops without calling task_complete (backward compat).
	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done by worker"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"some context"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done by worker", results[0].Result)
	assert.Nil(t, results[0].Completion)
}

func TestDelegateToolConcurrentWithCompletion(t *testing.T) {
	reg := NewRegistry()
	// worker-a uses task_complete.
	reg.Register("a", "Agent A", func() *Agent {
		return New("a", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"a done","files_modified":["x.go"]}`,
					},
				),
			},
		}, Options{})
	})
	// worker-b stops without task_complete.
	reg.Register("b", "Agent B", func() *Agent {
		return New("b", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "result-b"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"a","task":"task-a","context":"ctx-a"},{"agent":"b","task":"task-b","context":"ctx-b"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 2)

	// Agent "a" has structured completion.
	assert.Equal(t, "a", results[0].Agent)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "completed", results[0].Completion.Status)
	assert.Equal(t, "a done", results[0].Completion.Summary)

	// Agent "b" has no completion (backward compat).
	assert.Equal(t, "b", results[1].Agent)
	assert.Nil(t, results[1].Completion)
	assert.Equal(t, "result-b", results[1].Result)
}

// --- iteration exhaustion tests ---

func TestDelegateToolIterationExhaustion(t *testing.T) {
	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		// Worker loops with tool calls and never produces a final answer.
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
				),
				message.New("", role.Assistant,
					content.ToolCall{ID: "c2", Name: "echo", Arguments: `{}`},
				),
				message.New("", role.Assistant,
					content.ToolCall{ID: "c3", Name: "echo", Arguments: `{}`},
				),
			},
		}, Options{MaxIterations: 2})
	})

	echoTB := toolbox.New()
	echoTB.Register(toolbox.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return string(input), nil
		},
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do stuff","context":"some context"}]}`,
	))

	// Should NOT return an error — returns structured CompletionResult instead.
	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "failed", results[0].Completion.Status)
	assert.Contains(t, results[0].Completion.Summary, "exhausted")
	assert.Contains(t, results[0].Completion.Caveats, "Iteration limit reached")
}

func TestDelegateToolIterationExhaustionWithTaskID(t *testing.T) {
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
				),
				message.New("", role.Assistant,
					content.ToolCall{ID: "c2", Name: "echo", Arguments: `{}`},
				),
			},
		}, Options{MaxIterations: 1})
	})

	echoTB := toolbox.New()
	echoTB.Register(toolbox.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return string(input), nil
		},
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do stuff","context":"some context","task_id":"task-42"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "failed", results[0].Completion.Status)

	// Task was claimed and status was updated to "failed".
	require.Len(t, board.claims, 1)
	assert.Equal(t, "task-42", board.claims[0].ID)
	require.Len(t, board.updates, 1)
	assert.Equal(t, "task-42", board.updates[0].ID)
	assert.Equal(t, "failed", board.updates[0].Status)
}

func TestDelegateToolConcurrentIterationExhaustion(t *testing.T) {
	reg := NewRegistry()
	// "slow" agent exhausts iterations.
	reg.Register("slow", "Slow agent", func() *Agent {
		return New("slow", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
				),
				message.New("", role.Assistant,
					content.ToolCall{ID: "c2", Name: "echo", Arguments: `{}`},
				),
			},
		}, Options{MaxIterations: 1})
	})
	// "fast" agent completes normally.
	reg.Register("fast", "Fast agent", func() *Agent {
		return New("fast", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done"),
			},
		}, Options{})
	})

	echoTB := toolbox.New()
	echoTB.Register(toolbox.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return string(input), nil
		},
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, options: Options{MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"slow","task":"slow task","context":"ctx"},{"agent":"fast","task":"fast task","context":"ctx"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 2)

	// "slow" agent: structured completion with failed status, no error string.
	assert.Equal(t, "slow", results[0].Agent)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "failed", results[0].Completion.Status)
	assert.Contains(t, results[0].Completion.Summary, "exhausted")
	assert.Empty(t, results[0].Error)

	// "fast" agent: normal completion, unaffected.
	assert.Equal(t, "fast", results[1].Agent)
	assert.Equal(t, "done", results[1].Result)
	assert.Empty(t, results[1].Error)
}

func TestDelegateToolConcurrentIterationExhaustionWithTaskID(t *testing.T) {
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("slow", "Slow agent", func() *Agent {
		return New("slow", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
				),
				message.New("", role.Assistant,
					content.ToolCall{ID: "c2", Name: "echo", Arguments: `{}`},
				),
			},
		}, Options{MaxIterations: 1})
	})

	echoTB := toolbox.New()
	echoTB.Register(toolbox.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return string(input), nil
		},
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"slow","task":"slow task","context":"ctx","task_id":"task-99"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)

	assert.Equal(t, "slow", results[0].Agent)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "failed", results[0].Completion.Status)

	// Task was claimed and status was updated to "failed".
	require.Len(t, board.claims, 1)
	assert.Equal(t, "task-99", board.claims[0].ID)
	require.Len(t, board.updates, 1)
	assert.Equal(t, "task-99", board.updates[0].ID)
	assert.Equal(t, "failed", board.updates[0].Status)
}

func TestSearchReflectionsCapsFiles(t *testing.T) {
	dir := t.TempDir()

	// Create more than maxReflectionFiles (5) relevant reflection files.
	for i := range 10 {
		name := fmt.Sprintf("agent-%d.md", i)
		body := fmt.Sprintf("# Reflection\n\n## Task\nimplement authentication middleware refactor\n\n## Summary\nfailed %d\n", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}

	result := searchReflections(dir, "implement authentication middleware refactor")
	assert.NotEmpty(t, result)

	// Count how many reflection blocks are in the output (separated by "---").
	count := strings.Count(result, "# Reflection")
	assert.LessOrEqual(t, count, maxReflectionFiles)
}

func TestSearchReflectionsCapsBytes(t *testing.T) {
	dir := t.TempDir()

	// Create a few large reflection files that exceed maxReflectionBytes.
	for i := range 3 {
		name := fmt.Sprintf("agent-%d.md", i)
		// Each file is ~20KB, so 2 files = 40KB which exceeds maxReflectionBytes (32KB).
		body := fmt.Sprintf("# Reflection\n\n## Task\nimplement authentication middleware refactor\n\n## Summary\n%s\n", strings.Repeat("x", 20*1024))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}

	result := searchReflections(dir, "implement authentication middleware refactor")
	assert.NotEmpty(t, result)

	// Should have stopped before reading all 3.
	count := strings.Count(result, "# Reflection")
	assert.LessOrEqual(t, count, 2)
}

func TestDelegateToolPropagatesEventFuncToChild(t *testing.T) {
	var mu sync.Mutex
	var events []string

	ef := func(_ context.Context, kind string, _ any) {
		mu.Lock()
		events = append(events, kind)
		mu.Unlock()
	}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done"),
			},
		}, Options{})
	})

	a := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		options:    Options{EventFunc: ef, MaxDelegationDepth: 1},
	}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do the thing","context":"ctx"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done", results[0].Result)

	// The child agent should have emitted at least one "message_added" event
	// via the propagated EventFunc.
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, events, "message_added")
}

// --- taskSlug tests ---

func TestTaskSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"refactor the authentication module", "refactor"},
		{"fix bug in parser", "fix"},
		{"the and for with", "task"}, // all stop words
		{"a b", "task"},              // all too short
		{"", "task"},                 // empty
		{"implement superlongkeywordhere", "implement"},
		{"the implementationofthings", "implementati"}, // truncated to 12
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, taskSlug(tt.input))
		})
	}
}

func TestConfigNamePreservedForSelfExclusion(t *testing.T) {
	// An agent with a unique instance name but a config name matching a
	// registry entry should still exclude itself from list_agents.
	reg := NewRegistry()
	reg.Register("coder", "Writes code", func() *Agent { return &Agent{} })
	reg.Register("reviewer", "Reviews code", func() *Agent { return &Agent{} })

	a := &Agent{
		name:       "coder-refactor-1", // unique instance name
		configName: "coder",            // matches registry entry
		registry:   reg,
	}

	tool := listAgentsTool(a)
	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var entries []Entry
	require.NoError(t, json.Unmarshal([]byte(result), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "reviewer", entries[0].Name)
}

func TestConfigNamePreservedForSelfDelegation(t *testing.T) {
	// Self-delegation check should use configName, not instance name.
	reg := NewRegistry()
	reg.Register("coder", "Writes code", func() *Agent { return &Agent{} })

	a := &Agent{
		name:       "coder-refactor-1",
		configName: "coder",
		registry:   reg,
	}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"coder","task":"loop","context":""}]}`,
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-delegation")
}

func TestDelegateToolUpdateStatusError(t *testing.T) {
	t.Run("successful completion path", func(t *testing.T) {
		board := &mockTaskBoard{
			updateFn: func(_, _ string) error {
				return errors.New("board unavailable")
			},
		}

		reg := NewRegistry()
		reg.Register("worker", "Does work", func() *Agent {
			return New("worker", "", "", &sequenceCompleter{
				replies: []message.Message{
					message.New("", role.Assistant,
						content.ToolCall{
							ID:        "c1",
							Name:      "task_complete",
							Arguments: `{"status":"completed","summary":"did it"}`,
						},
					),
				},
			}, Options{})
		})

		a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
		tool := delegateTool(a)

		result, err := tool.Handler(context.Background(), json.RawMessage(
			`{"tasks":[{"agent":"worker","task":"do","context":"ctx","task_id":"task-1"}]}`,
		))

		require.NoError(t, err)

		var results []delegateResult
		require.NoError(t, json.Unmarshal([]byte(result), &results))
		require.Len(t, results, 1)

		assert.Equal(t, "worker", results[0].Agent)
		assert.Contains(t, results[0].Warning, "task board update failed")
		assert.Contains(t, results[0].Warning, "board unavailable")
		assert.Empty(t, results[0].Error)
		// Child's completion and result are still present.
		require.NotNil(t, results[0].Completion)
		assert.Equal(t, "completed", results[0].Completion.Status)
		assert.Equal(t, "did it", results[0].Result)
	})

	t.Run("iteration exhaustion path", func(t *testing.T) {
		board := &mockTaskBoard{
			updateFn: func(_, _ string) error {
				return errors.New("board unavailable")
			},
		}

		reg := NewRegistry()
		reg.Register("worker", "Does work", func() *Agent {
			return New("worker", "", "", &sequenceCompleter{
				replies: []message.Message{
					message.New("", role.Assistant,
						content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
					),
					message.New("", role.Assistant,
						content.ToolCall{ID: "c2", Name: "echo", Arguments: `{}`},
					),
				},
			}, Options{MaxIterations: 1})
		})

		echoTB := toolbox.New()
		echoTB.Register(toolbox.Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler: func(_ context.Context, input json.RawMessage) (string, error) {
				return string(input), nil
			},
		})

		a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, options: Options{TaskBoard: board, MaxDelegationDepth: 1}}
		tool := delegateTool(a)

		result, err := tool.Handler(context.Background(), json.RawMessage(
			`{"tasks":[{"agent":"worker","task":"do stuff","context":"ctx","task_id":"task-42"}]}`,
		))

		require.NoError(t, err)

		var results []delegateResult
		require.NoError(t, json.Unmarshal([]byte(result), &results))
		require.Len(t, results, 1)

		assert.Equal(t, "worker", results[0].Agent)
		assert.Contains(t, results[0].Warning, "task board update failed")
		assert.Contains(t, results[0].Warning, "board unavailable")
		assert.Empty(t, results[0].Error)
		require.NotNil(t, results[0].Completion)
		assert.Equal(t, "failed", results[0].Completion.Status)
	})
}
