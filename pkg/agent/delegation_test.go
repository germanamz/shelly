package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestListAgentsToolReturnsEnrichedEntries(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterEntry(Entry{
		Name:          "self",
		Description:   "Self",
		Skills:        []string{"planning"},
		EstimatedCost: "cheap",
	}, func() *Agent { return &Agent{} })
	reg.RegisterEntry(Entry{
		Name:           "coder",
		Description:    "Writes code",
		Skills:         []string{"coding", "testing"},
		EstimatedCost:  "medium",
		MaxConcurrency: 3,
	}, func() *Agent { return &Agent{} })

	a := &Agent{name: "self", configName: "self", registry: reg}
	tool := listAgentsTool(a)
	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var entries []Entry
	require.NoError(t, json.Unmarshal([]byte(result), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "coder", entries[0].Name)
	assert.Equal(t, []string{"coding", "testing"}, entries[0].Skills)
	assert.Equal(t, "medium", entries[0].EstimatedCost)
	assert.Equal(t, 3, entries[0].MaxConcurrency)
}

func TestListAgentsToolOmitsEmptyFields(t *testing.T) {
	reg := NewRegistry()
	reg.Register("self", "Self", func() *Agent { return &Agent{} })
	reg.Register("simple", "Simple agent", func() *Agent { return &Agent{} })

	a := &Agent{name: "self", configName: "self", registry: reg}
	tool := listAgentsTool(a)
	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	// Verify omitempty: no skills, estimated_cost, max_concurrency keys.
	assert.NotContains(t, result, "skills")
	assert.NotContains(t, result, "estimated_cost")
	assert.NotContains(t, result, "max_concurrency")
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
		delegation: delegationConfig{maxDepth: 2},
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
		delegation: delegationConfig{maxDepth: 0},
		depth:      0,
	}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"worker","task":"do","context":""}]}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max delegation depth")
}

func TestDelegateToolAgentNotFound(t *testing.T) {
	reg := NewRegistry()

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{parentTB}, delegation: delegationConfig{maxDepth: 1}}
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

// mockTaskCancelWatcher extends mockTaskBoard with WatchCanceled support.
type mockTaskCancelWatcher struct {
	mockTaskBoard
	cancelMu sync.Mutex
	channels map[string]chan struct{}
}

func newMockTaskCancelWatcher() *mockTaskCancelWatcher {
	return &mockTaskCancelWatcher{channels: make(map[string]chan struct{})}
}

func (m *mockTaskCancelWatcher) WatchCanceled(ctx context.Context, id string) <-chan struct{} {
	m.cancelMu.Lock()
	ch, ok := m.channels[id]
	if !ok {
		ch = make(chan struct{})
		m.channels[id] = ch
	}
	m.cancelMu.Unlock()

	out := make(chan struct{})
	go func() {
		defer close(out)
		select {
		case <-ch:
		case <-ctx.Done():
		}
	}()
	return out
}

// cancelTask simulates task cancellation by closing the channel.
func (m *mockTaskCancelWatcher) cancelTask(id string) {
	m.cancelMu.Lock()
	ch, ok := m.channels[id]
	if !ok {
		ch = make(chan struct{})
		m.channels[id] = ch
	}
	m.cancelMu.Unlock()
	close(ch)
}

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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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
	// Child doesn't call task_complete — task is auto-completed since it ran to natural conclusion.
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done without completion"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do","context":"ctx","task_id":"task-1"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done without completion", results[0].Result)

	// Task was claimed and auto-completed.
	require.Len(t, board.claims, 1)
	require.Len(t, board.updates, 1)
	assert.Equal(t, "task-1", board.updates[0].ID)
	assert.Equal(t, "completed", board.updates[0].Status)
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
		delegation: delegationConfig{
			maxDepth:      1,
			reflectionDir: t.TempDir(),
			taskBoard:     board,
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, delegation: delegationConfig{maxDepth: 1}}
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

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
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
		delegation: delegationConfig{maxDepth: 1},
		events:     eventConfig{eventFunc: ef},
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

		a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
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

		a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), toolboxes: []*toolbox.ToolBox{echoTB}, delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
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

func TestDelegateToolErrorRollsBackTask(t *testing.T) {
	// child.Run() returns a non-ErrMaxIterations error → task updated to "failed".
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &errorCompleter{
			err: errors.New("completer crashed"),
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do","context":"ctx","task_id":"task-err"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "completer crashed")

	// Task was claimed and rolled back to "failed".
	require.Len(t, board.claims, 1)
	assert.Equal(t, "task-err", board.claims[0].ID)
	require.Len(t, board.updates, 1)
	assert.Equal(t, "task-err", board.updates[0].ID)
	assert.Equal(t, "failed", board.updates[0].Status)
}

func TestDelegateToolErrorRollbackUpdateFails(t *testing.T) {
	// child.Run() error + UpdateTaskStatus error → Warning populated.
	board := &mockTaskBoard{
		updateFn: func(_, _ string) error {
			return errors.New("board unavailable")
		},
	}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &errorCompleter{
			err: errors.New("completer crashed"),
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do","context":"ctx","task_id":"task-err"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "completer crashed")
	assert.Contains(t, results[0].Warning, "task board update failed")
	assert.Contains(t, results[0].Warning, "board unavailable")
}

// --- prependContext tests ---

func TestPrependContextUnderBudget(t *testing.T) {
	child := &Agent{chat: chat.New()}
	ctx := "short context"
	prependContext(child, ctx)

	require.Equal(t, 1, child.chat.Len())
	assert.Contains(t, child.chat.At(0).TextContent(), "short context")
	assert.NotContains(t, child.chat.At(0).TextContent(), "[context truncated]")
}

func TestPrependContextOverBudget(t *testing.T) {
	child := &Agent{chat: chat.New()}
	ctx := strings.Repeat("x", maxDelegateContextRunes+1000)
	prependContext(child, ctx)

	require.Equal(t, 1, child.chat.Len())
	text := child.chat.At(0).TextContent()
	assert.Contains(t, text, "… [context truncated]")
	// The truncated content should be shorter than the original.
	assert.Less(t, len(text), len(ctx))
}

func TestPrependContextEmpty(t *testing.T) {
	child := &Agent{chat: chat.New()}
	prependContext(child, "")

	assert.Equal(t, 0, child.chat.Len())
}

func TestDelegateToolNoCompletionUpdatesTask(t *testing.T) {
	// Child finishes without error but no CompletionResult → task auto-completed.
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "all done"),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do","context":"ctx","task_id":"task-nc"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "all done", results[0].Result)
	assert.Empty(t, results[0].Warning)

	// Task was claimed and auto-completed.
	require.Len(t, board.claims, 1)
	assert.Equal(t, "task-nc", board.claims[0].ID)
	require.Len(t, board.updates, 1)
	assert.Equal(t, "task-nc", board.updates[0].ID)
	assert.Equal(t, "completed", board.updates[0].Status)
}

// contextCompleter blocks until the context is canceled, then returns context.Canceled.
type contextCompleter struct{}

func (c *contextCompleter) Complete(ctx context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	<-ctx.Done()
	return message.Message{}, ctx.Err()
}

func TestDelegateTaskCancellationPropagation(t *testing.T) {
	board := newMockTaskCancelWatcher()

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &contextCompleter{}, Options{MaxIterations: 100})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, taskBoard: board}}
	tool := delegateTool(a)

	done := make(chan string, 1)
	go func() {
		result, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"worker","task":"do work","context":"ctx","task_id":"task-42"}]}`))
		if err != nil {
			done <- err.Error()
		} else {
			done <- result
		}
	}()

	// Wait for the claim to be recorded.
	require.Eventually(t, func() bool {
		board.mu.Lock()
		defer board.mu.Unlock()
		return len(board.claims) > 0
	}, 2*time.Second, 10*time.Millisecond)

	// Cancel the task on the board.
	board.cancelTask("task-42")

	// The delegation should complete with a canceled result.
	select {
	case result := <-done:
		var results []delegateResult
		require.NoError(t, json.Unmarshal([]byte(result), &results))
		require.Len(t, results, 1)
		assert.Equal(t, "task canceled", results[0].Error)
	case <-time.After(5 * time.Second):
		t.Fatal("delegation did not complete after task cancellation")
	}

	// Task should have been updated to canceled.
	board.mu.Lock()
	defer board.mu.Unlock()
	require.NotEmpty(t, board.updates)
	lastUpdate := board.updates[len(board.updates)-1]
	assert.Equal(t, "task-42", lastUpdate.ID)
	assert.Equal(t, "canceled", lastUpdate.Status)
}

func TestDelegateToolEmitsDelegationProgressEvents(t *testing.T) {
	var mu sync.Mutex
	var progressEvents []DelegationEvent

	notifier := EventNotifier(func(_ context.Context, kind string, _ string, data any) {
		if kind == "delegation_progress" {
			mu.Lock()
			defer mu.Unlock()
			progressEvents = append(progressEvents, data.(DelegationEvent))
		}
	})

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				// First reply: assistant text with tool call (keeps loop going,
				// triggers progress event from the text portion).
				message.New("", role.Assistant,
					content.Text{Text: "I am working on it"},
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"all done"}`,
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
		delegation: delegationConfig{maxDepth: 1},
		events:     eventConfig{notifier: notifier},
	}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do it","context":"ctx"}]}`,
	))
	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "completed", results[0].Completion.Status)

	mu.Lock()
	defer mu.Unlock()

	// Expect at least one progress event and one result event.
	var hasProgress, hasResult bool
	for _, ev := range progressEvents {
		switch ev.Kind {
		case DelegationProgress:
			hasProgress = true
			assert.Equal(t, "I am working on it", ev.Message)
			assert.Equal(t, "orch", ev.Parent)
			assert.True(t, strings.HasPrefix(ev.Agent, "worker-"))
		case DelegationResult:
			hasResult = true
			require.NotNil(t, ev.Result)
			assert.Equal(t, "all done", ev.Result.Result)
			assert.Equal(t, "orch", ev.Parent)
		}
	}
	assert.True(t, hasProgress, "expected at least one DelegationProgress event")
	assert.True(t, hasResult, "expected a DelegationResult event")
}

func TestDelegateToolEmitsResultOnError(t *testing.T) {
	var mu sync.Mutex
	var resultEvents []DelegationEvent

	notifier := EventNotifier(func(_ context.Context, kind string, _ string, data any) {
		if kind == "delegation_progress" {
			mu.Lock()
			defer mu.Unlock()
			if ev, ok := data.(DelegationEvent); ok && ev.Kind == DelegationResult {
				resultEvents = append(resultEvents, ev)
			}
		}
	})

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &errorCompleter{err: errors.New("boom")}, Options{})
	})

	a := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{maxDepth: 1},
		events:     eventConfig{notifier: notifier},
	}
	tool := delegateTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"fail","context":"ctx"}]}`,
	))
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, resultEvents, 1)
	assert.Equal(t, DelegationResult, resultEvents[0].Kind)
	assert.Contains(t, resultEvents[0].Result.Error, "boom")
}

// --- handoff tests ---

func TestDelegateToolHandoffSuccess(t *testing.T) {
	// Child calls handoff to transfer control to a peer. The peer completes.
	reg := NewRegistry()
	reg.Register("planner", "Plans", func() *Agent {
		return New("planner", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "h1",
						Name:      "handoff",
						Arguments: `{"target_agent":"coder","reason":"needs coding","context":"plan: refactor module X"}`,
					},
				),
			},
		}, Options{MaxHandoffs: 3})
	})
	reg.Register("coder", "Codes", func() *Agent {
		return New("coder", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"refactored module X"}`,
					},
				),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, maxHandoffs: 3}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"planner","task":"refactor module X","context":"project context"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "completed", results[0].Completion.Status)
	assert.Equal(t, "refactored module X", results[0].Completion.Summary)
}

func TestDelegateToolHandoffChainLimit(t *testing.T) {
	// Each agent hands off to the next, eventually hitting the chain limit.
	reg := NewRegistry()
	reg.Register("a", "Agent A", func() *Agent {
		return New("a", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "h1",
						Name:      "handoff",
						Arguments: `{"target_agent":"b","reason":"needs b","context":"ctx from a"}`,
					},
				),
			},
		}, Options{MaxHandoffs: 2})
	})
	reg.Register("b", "Agent B", func() *Agent {
		return New("b", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "h2",
						Name:      "handoff",
						Arguments: `{"target_agent":"a","reason":"needs a","context":"ctx from b"}`,
					},
				),
			},
		}, Options{MaxHandoffs: 2})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, maxHandoffs: 2}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"a","task":"ping pong","context":"ctx"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "handoff chain limit")
}

func TestDelegateToolHandoffSelfRejected(t *testing.T) {
	reg := NewRegistry()
	reg.Register("coder", "Codes", func() *Agent {
		return New("coder", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "h1",
						Name:      "handoff",
						Arguments: `{"target_agent":"coder","reason":"I can do it better next time","context":"ctx"}`,
					},
				),
			},
		}, Options{MaxHandoffs: 3})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, maxHandoffs: 3}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"coder","task":"do something","context":"ctx"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "self-handoff is not allowed")
}

func TestDelegateToolHandoffToNonexistentAgent(t *testing.T) {
	reg := NewRegistry()
	reg.Register("planner", "Plans", func() *Agent {
		return New("planner", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "h1",
						Name:      "handoff",
						Arguments: `{"target_agent":"ghost","reason":"needs ghost","context":"ctx"}`,
					},
				),
			},
		}, Options{MaxHandoffs: 3})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, maxHandoffs: 3}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"planner","task":"find ghost","context":"ctx"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "handoff target \"ghost\" not found")
}

func TestDelegateToolHandoffWithTaskBoard(t *testing.T) {
	board := &mockTaskBoard{}

	reg := NewRegistry()
	reg.Register("planner", "Plans", func() *Agent {
		return New("planner", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "h1",
						Name:      "handoff",
						Arguments: `{"target_agent":"coder","reason":"needs coding","context":"plan ready"}`,
					},
				),
			},
		}, Options{MaxHandoffs: 3})
	})
	reg.Register("coder", "Codes", func() *Agent {
		return New("coder", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.New("", role.Assistant,
					content.ToolCall{
						ID:        "c1",
						Name:      "task_complete",
						Arguments: `{"status":"completed","summary":"coded it"}`,
					},
				),
			},
		}, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1, maxHandoffs: 3, taskBoard: board}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"planner","task":"code it","context":"ctx","task_id":"task-ho"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Completion)
	assert.Equal(t, "completed", results[0].Completion.Status)

	// Initial claim for planner child, then re-claim for coder peer.
	require.Len(t, board.claims, 2)
	assert.Equal(t, "task-ho", board.claims[0].ID)
	assert.True(t, strings.HasPrefix(board.claims[0].Agent, "planner-"))
	assert.Equal(t, "task-ho", board.claims[1].ID)
	assert.True(t, strings.HasPrefix(board.claims[1].Agent, "coder-"))

	// Final status update.
	require.Len(t, board.updates, 1)
	assert.Equal(t, "task-ho", board.updates[0].ID)
	assert.Equal(t, "completed", board.updates[0].Status)
}

func TestDelegateToolChildGetsInteractionChannel(t *testing.T) {
	// Worker calls request_input, gets auto-answered, then finishes.
	workerCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "request_input",
					Arguments: `{"question":"What is the target?"}`,
				},
			),
			message.NewText("", role.Assistant, "done with answer"),
		},
	}

	reg := NewRegistry()
	reg.Register("worker", "Worker", func() *Agent {
		return New("worker", "", "", workerCompleter, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"do work","context":"target is /tmp"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done with answer", results[0].Result)
}

func TestDelegateToolAutoAnswerUsesContext(t *testing.T) {
	// Worker calls request_input; verify the auto-answer contains the
	// delegation context.
	var captured []message.Message
	workerCompleter := &capturingAfterToolCompleter{
		toolReply: message.New("", role.Assistant,
			content.ToolCall{
				ID:        "c1",
				Name:      "request_input",
				Arguments: `{"question":"Where do I write output?"}`,
			},
		),
		finalReply: message.NewText("", role.Assistant, "wrote output"),
	}

	reg := NewRegistry()
	reg.Register("worker", "Worker", func() *Agent {
		return New("worker", "", "", workerCompleter, Options{})
	})

	a := &Agent{name: "orch", configName: "orch", registry: reg, chat: chat.New(), delegation: delegationConfig{maxDepth: 1}}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"worker","task":"write files","context":"output goes to /var/data"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "wrote output", results[0].Result)

	// Verify the tool result (auto-answer) contains the delegation context.
	_ = captured
	// The auto-answer is injected into the child's chat as a tool result.
	// We verify via the capturingAfterToolCompleter that the tool result
	// contained the delegation context.
	assert.Contains(t, workerCompleter.lastToolResult, "output goes to /var/data")
}

// capturingAfterToolCompleter returns the toolReply first, then captures
// the tool result from the chat before returning finalReply.
type capturingAfterToolCompleter struct {
	toolReply      message.Message
	finalReply     message.Message
	callCount      int
	lastToolResult string
}

func (c *capturingAfterToolCompleter) Complete(_ context.Context, ch *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	c.callCount++
	if c.callCount == 1 {
		return c.toolReply, nil
	}
	// Capture the last tool result message from the chat.
	msgs := ch.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == role.Tool {
			for _, p := range msgs[i].Parts {
				if tr, ok := p.(content.ToolResult); ok {
					c.lastToolResult = tr.Content
					break
				}
			}
			break
		}
	}
	return c.finalReply, nil
}

func TestDelegateToolHandoffPeerGetsInteraction(t *testing.T) {
	// Planner hands off to coder, coder calls request_input, gets auto-answered.
	plannerCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "handoff",
					Arguments: `{"target_agent":"coder","reason":"need coding","context":"use Go"}`,
				},
			),
		},
	}

	coderCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c2",
					Name:      "request_input",
					Arguments: `{"question":"Which file to edit?"}`,
				},
			),
			message.NewText("", role.Assistant, "edited file"),
		},
	}

	reg := NewRegistry()
	reg.Register("planner", "Planner", func() *Agent {
		return New("planner", "", "", plannerCompleter, Options{MaxHandoffs: 3})
	})
	reg.Register("coder", "Coder", func() *Agent {
		return New("coder", "", "", coderCompleter, Options{MaxHandoffs: 3})
	})

	a := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{maxDepth: 1, maxHandoffs: 3},
	}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"planner","task":"build feature","context":"project uses Go"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "edited file", results[0].Result)
}

func TestDelegateToolHandoffPeerAutoAnswerUsesHandoffContext(t *testing.T) {
	// Planner hands off to coder with a specific handoff context.
	// Coder calls request_input and the auto-answer should contain the
	// handoff context ("use Go and tests"), NOT the original delegation
	// context ("original parent context").
	plannerCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "h1",
					Name:      "handoff",
					Arguments: `{"target_agent":"coder","reason":"need coding","context":"use Go and tests"}`,
				},
			),
		},
	}

	var coderSnapshots [][]message.Message
	coderCompleter := &snapshotCompleter{
		snapshots: &coderSnapshots,
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "ri1",
					Name:      "request_input",
					Arguments: `{"question":"Which file?"}`,
				},
			),
			message.NewText("", role.Assistant, "done"),
		},
	}

	reg := NewRegistry()
	reg.Register("planner", "Planner", func() *Agent {
		return New("planner", "", "", plannerCompleter, Options{MaxHandoffs: 3})
	})
	reg.Register("coder", "Coder", func() *Agent {
		return New("coder", "", "", coderCompleter, Options{MaxHandoffs: 3})
	})

	a := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{maxDepth: 1, maxHandoffs: 3},
	}
	tool := delegateTool(a)

	result, err := tool.Handler(context.Background(), json.RawMessage(
		`{"tasks":[{"agent":"planner","task":"build feature","context":"original parent context"}]}`,
	))

	require.NoError(t, err)

	var results []delegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "done", results[0].Result)

	// The second snapshot (after request_input tool result) should contain
	// the handoff context in the auto-answer, not the original context.
	require.GreaterOrEqual(t, len(coderSnapshots), 2, "expected at least 2 snapshots from coder")
	secondSnapshot := coderSnapshots[1]

	var autoAnswerContent string
	for _, msg := range secondSnapshot {
		for _, p := range msg.Parts {
			if tr, ok := p.(content.ToolResult); ok {
				autoAnswerContent += tr.Content
			}
		}
	}
	assert.Contains(t, autoAnswerContent, "use Go and tests", "auto-answer should contain handoff context")
	assert.NotContains(t, autoAnswerContent, "original parent context", "auto-answer should NOT contain original delegation context")
}
