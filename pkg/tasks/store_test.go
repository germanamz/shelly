package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/agentctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustCreate is a test helper that calls Create and fails the test on error.
func mustCreate(t *testing.T, s *Store, task Task) string {
	t.Helper()
	id, err := s.Create(task)
	require.NoError(t, err)
	return id
}

// --- CRUD tests ---

func TestCreate(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "do something"})
	assert.Equal(t, "task-1", id)

	task, ok := s.Get(id)
	require.True(t, ok)
	assert.Equal(t, "do something", task.Title)
	assert.Equal(t, StatusPending, task.Status)
}

func TestCreateSequentialIDs(t *testing.T) {
	s := &Store{}

	id1 := mustCreate(t, s, Task{Title: "first"})
	id2 := mustCreate(t, s, Task{Title: "second"})
	id3 := mustCreate(t, s, Task{Title: "third"})

	assert.Equal(t, "task-1", id1)
	assert.Equal(t, "task-2", id2)
	assert.Equal(t, "task-3", id3)
}

func TestCreateRejectsNonPendingStatus(t *testing.T) {
	s := &Store{}

	_, err := s.Create(Task{Title: "task", Status: StatusInProgress})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not accept Status")
}

func TestCreateRejectsAssignee(t *testing.T) {
	s := &Store{}

	_, err := s.Create(Task{Title: "task", Assignee: "worker"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not accept Assignee")
}

func TestCreateAllowsZeroStatusAndEmptyAssignee(t *testing.T) {
	s := &Store{}

	// Zero-value Status ("") and empty Assignee should be fine.
	id, err := s.Create(Task{Title: "ok task"})
	require.NoError(t, err)
	assert.Equal(t, "task-1", id)

	// Explicit StatusPending should also be fine.
	id2, err := s.Create(Task{Title: "also ok", Status: StatusPending})
	require.NoError(t, err)
	assert.Equal(t, "task-2", id2)
}

func TestGetNotFound(t *testing.T) {
	s := &Store{}

	_, ok := s.Get("task-999")
	assert.False(t, ok)
}

func TestGetReturnsCopy(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "original", BlockedBy: []string{"task-0"}})

	cp, _ := s.Get("task-1")
	cp.BlockedBy[0] = "mutated"

	original, _ := s.Get("task-1")
	assert.Equal(t, "original", original.Title)
	assert.Equal(t, []string{"task-0"}, original.BlockedBy)
}

func TestListAll(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "a"})
	mustCreate(t, s, Task{Title: "b"})

	tasks := s.List(Filter{})
	assert.Len(t, tasks, 2)
	assert.Equal(t, "task-1", tasks[0].ID)
	assert.Equal(t, "task-2", tasks[1].ID)
}

func TestListFilterByStatus(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "pending"})
	id2 := mustCreate(t, s, Task{Title: "done"})
	completed := StatusCompleted
	require.NoError(t, s.Update(id2, Update{Status: &completed}))

	pending := StatusPending
	tasks := s.List(Filter{Status: &pending})
	assert.Len(t, tasks, 1)
	assert.Equal(t, "pending", tasks[0].Title)
}

func TestListFilterByAssignee(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "unassigned"})
	id2 := mustCreate(t, s, Task{Title: "assigned"})
	require.NoError(t, s.Claim(id2, "worker"))

	assignee := "worker"
	tasks := s.List(Filter{Assignee: &assignee})
	assert.Len(t, tasks, 1)
	assert.Equal(t, "assigned", tasks[0].Title)
}

func TestListFilterByBlocked(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "blocker"})
	mustCreate(t, s, Task{Title: "blocked", BlockedBy: []string{"task-1"}})

	notBlocked := false
	tasks := s.List(Filter{Blocked: &notBlocked})
	assert.Len(t, tasks, 1)
	assert.Equal(t, "blocker", tasks[0].Title)

	blocked := true
	tasks = s.List(Filter{Blocked: &blocked})
	assert.Len(t, tasks, 1)
	assert.Equal(t, "blocked", tasks[0].Title)
}

func TestUpdate(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})

	newDesc := "updated description"
	completed := StatusCompleted
	require.NoError(t, s.Update(id, Update{
		Status:      &completed,
		Description: &newDesc,
		Metadata:    map[string]any{"key": "value"},
	}))

	task, ok := s.Get(id)
	require.True(t, ok)
	assert.Equal(t, StatusCompleted, task.Status)
	assert.Equal(t, "updated description", task.Description)
	assert.Equal(t, map[string]any{"key": "value"}, task.Metadata)
}

func TestUpdateNotFound(t *testing.T) {
	s := &Store{}

	err := s.Update("task-999", Update{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateBlockedBy(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})

	newBlockedBy := []string{"task-x"}
	require.NoError(t, s.Update(id, Update{BlockedBy: &newBlockedBy}))

	task, _ := s.Get(id)
	assert.Equal(t, []string{"task-x"}, task.BlockedBy)
}

func TestUpdateMetadataMerges(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task", Metadata: map[string]any{"a": 1}})

	require.NoError(t, s.Update(id, Update{Metadata: map[string]any{"b": 2}}))

	task, _ := s.Get(id)
	assert.Equal(t, map[string]any{"a": 1, "b": 2}, task.Metadata)
}

// --- Claim tests ---

func TestClaimSuccess(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})

	require.NoError(t, s.Claim(id, "worker"))

	task, _ := s.Get(id)
	assert.Equal(t, "worker", task.Assignee)
	assert.Equal(t, StatusInProgress, task.Status)
}

func TestClaimAlreadyAssigned(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	require.NoError(t, s.Claim(id, "worker-1"))

	err := s.Claim(id, "worker-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already assigned")
}

func TestClaimSameAgentIdempotent(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	require.NoError(t, s.Claim(id, "worker"))
	require.NoError(t, s.Claim(id, "worker"))
}

func TestClaimBlocked(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "blocker"})
	id := mustCreate(t, s, Task{Title: "blocked", BlockedBy: []string{"task-1"}})

	err := s.Claim(id, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestClaimTerminal(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	completed := StatusCompleted
	require.NoError(t, s.Update(id, Update{Status: &completed}))

	err := s.Claim(id, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal")
}

func TestClaimNotFound(t *testing.T) {
	s := &Store{}

	err := s.Claim("task-999", "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Reassign tests ---

func TestReassign(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	require.NoError(t, s.Claim(id, "worker-1"))

	require.NoError(t, s.Reassign(id, "worker-2"))

	task, _ := s.Get(id)
	assert.Equal(t, "worker-2", task.Assignee)
	assert.Equal(t, StatusInProgress, task.Status)
}

func TestReassignSameAgent(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	require.NoError(t, s.Claim(id, "worker"))

	require.NoError(t, s.Reassign(id, "worker"))

	task, _ := s.Get(id)
	assert.Equal(t, "worker", task.Assignee)
}

func TestReassignFromUnassigned(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})

	require.NoError(t, s.Reassign(id, "worker"))

	task, _ := s.Get(id)
	assert.Equal(t, "worker", task.Assignee)
	assert.Equal(t, StatusInProgress, task.Status)
}

func TestReassignTerminal(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	completed := StatusCompleted
	require.NoError(t, s.Update(id, Update{Status: &completed}))

	err := s.Reassign(id, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal")
}

func TestReassignBlocked(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "blocker"})
	id := mustCreate(t, s, Task{Title: "blocked", BlockedBy: []string{"task-1"}})

	err := s.Reassign(id, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestReassignNotFound(t *testing.T) {
	s := &Store{}

	err := s.Reassign("task-999", "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- IsBlocked tests ---

func TestIsBlocked(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "blocker"})
	mustCreate(t, s, Task{Title: "blocked", BlockedBy: []string{"task-1"}})

	assert.True(t, s.IsBlocked("task-2"))

	// Complete the blocker.
	completed := StatusCompleted
	require.NoError(t, s.Update("task-1", Update{Status: &completed}))

	assert.False(t, s.IsBlocked("task-2"))
}

func TestIsBlockedNonexistentDep(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "task", BlockedBy: []string{"task-999"}})

	// Nonexistent deps are conservatively treated as blocking.
	assert.True(t, s.IsBlocked("task-1"))
}

// --- Watch tests ---

func TestWatchAlreadyDone(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	completed := StatusCompleted
	require.NoError(t, s.Update(id, Update{Status: &completed}))

	task, err := s.WatchCompleted(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, task.Status)
}

func TestWatchBlocksUntilDone(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var result Task
	var watchErr error

	go func() {
		result, watchErr = s.WatchCompleted(ctx, id)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	completed := StatusCompleted
	require.NoError(t, s.Update(id, Update{Status: &completed}))

	<-done
	require.NoError(t, watchErr)
	assert.Equal(t, StatusCompleted, result.Status)
}

func TestWatchFailed(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})
	failed := StatusFailed
	require.NoError(t, s.Update(id, Update{Status: &failed}))

	task, err := s.WatchCompleted(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, task.Status)
}

func TestWatchContextCancelled(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.WatchCompleted(ctx, id)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWatchNotFound(t *testing.T) {
	s := &Store{}

	_, err := s.WatchCompleted(context.Background(), "task-999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Concurrency tests ---

func TestConcurrentClaim(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "contested"})

	var wg sync.WaitGroup
	successes := make(chan string, 10)

	for i := range 10 {
		agent := fmt.Sprintf("worker-%d", i)
		wg.Go(func() {
			if err := s.Claim(id, agent); err == nil {
				successes <- agent
			}
		})
	}

	wg.Wait()
	close(successes)

	var winners []string
	for agent := range successes {
		winners = append(winners, agent)
	}
	assert.Len(t, winners, 1, "exactly one agent should win the claim")
}

func TestConcurrentCreateAndList(t *testing.T) {
	s := &Store{}

	var wg sync.WaitGroup

	for i := range 20 {
		title := fmt.Sprintf("task-%d", i)
		wg.Go(func() {
			_, _ = s.Create(Task{Title: title})
		})
	}

	for range 20 {
		wg.Go(func() {
			s.List(Filter{})
		})
	}

	wg.Wait()

	tasks := s.List(Filter{})
	assert.Len(t, tasks, 20)
}

// --- Tool integration tests ---

func TestToolCreate(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")

	tool, ok := tb.Get("ns_tasks_create")
	require.True(t, ok)

	ctx := agentctx.WithAgentName(context.Background(), "orchestrator")
	result, err := tool.Handler(ctx, json.RawMessage(`{"title":"research X","description":"find info"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task-1"}`, result)

	task, ok := s.Get("task-1")
	require.True(t, ok)
	assert.Equal(t, "research X", task.Title)
	assert.Equal(t, "find info", task.Description)
	assert.Equal(t, "orchestrator", task.CreatedBy)
}

func TestToolCreateMissingTitle(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")

	tool, _ := tb.Get("ns_tasks_create")
	_, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestToolList(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "a"})
	mustCreate(t, s, Task{Title: "b"})

	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_tasks_list")
	require.True(t, ok)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var tasks []Task
	require.NoError(t, json.Unmarshal([]byte(result), &tasks))
	assert.Len(t, tasks, 2)
}

func TestToolListWithFilter(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "a"})
	id2 := mustCreate(t, s, Task{Title: "b"})
	completed := StatusCompleted
	require.NoError(t, s.Update(id2, Update{Status: &completed}))

	tb := s.Tools("ns")
	tool, _ := tb.Get("ns_tasks_list")

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"status":"pending"}`))
	require.NoError(t, err)

	var tasks []Task
	require.NoError(t, json.Unmarshal([]byte(result), &tasks))
	assert.Len(t, tasks, 1)
	assert.Equal(t, "a", tasks[0].Title)
}

func TestToolGet(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "my task"})

	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_tasks_get")
	require.True(t, ok)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"id":"task-1"}`))
	require.NoError(t, err)

	var task Task
	require.NoError(t, json.Unmarshal([]byte(result), &task))
	assert.Equal(t, "my task", task.Title)
}

func TestToolGetNotFound(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")

	tool, _ := tb.Get("ns_tasks_get")
	_, err := tool.Handler(context.Background(), json.RawMessage(`{"id":"task-999"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestToolClaim(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "task"})

	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_tasks_claim")
	require.True(t, ok)

	ctx := agentctx.WithAgentName(context.Background(), "worker-1")
	result, err := tool.Handler(ctx, json.RawMessage(`{"id":"task-1"}`))
	require.NoError(t, err)
	assert.Equal(t, "ok", result)

	task, _ := s.Get("task-1")
	assert.Equal(t, "worker-1", task.Assignee)
	assert.Equal(t, StatusInProgress, task.Status)
}

func TestToolUpdate(t *testing.T) {
	s := &Store{}

	mustCreate(t, s, Task{Title: "task"})

	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_tasks_update")
	require.True(t, ok)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"id":"task-1","status":"completed"}`))
	require.NoError(t, err)
	assert.Equal(t, "ok", result)

	task, _ := s.Get("task-1")
	assert.Equal(t, StatusCompleted, task.Status)
}

func TestToolWatch(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "task"})

	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_tasks_watch")
	require.True(t, ok)

	// Complete the task first so watch returns immediately.
	completed := StatusCompleted
	require.NoError(t, s.Update(id, Update{Status: &completed}))

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"id":"task-1"}`))
	require.NoError(t, err)

	var task Task
	require.NoError(t, json.Unmarshal([]byte(result), &task))
	assert.Equal(t, StatusCompleted, task.Status)
}

func TestToolNamespace(t *testing.T) {
	s := &Store{}
	tb := s.Tools("myapp")

	names := make(map[string]bool)
	for _, tool := range tb.Tools() {
		names[tool.Name] = true
	}

	assert.True(t, names["myapp_tasks_create"])
	assert.True(t, names["myapp_tasks_list"])
	assert.True(t, names["myapp_tasks_get"])
	assert.True(t, names["myapp_tasks_claim"])
	assert.True(t, names["myapp_tasks_update"])
	assert.True(t, names["myapp_tasks_watch"])
	assert.Len(t, names, 6)
}

func TestCreateDoesNotNotify(t *testing.T) {
	s := &Store{}
	s.init()

	// Capture the signal channel before Create.
	s.mu.RLock()
	sigBefore := s.signal
	s.mu.RUnlock()

	mustCreate(t, s, Task{Title: "task"})

	// The signal channel should be the same (not closed/replaced) because
	// Create should not call notify.
	s.mu.RLock()
	sigAfter := s.signal
	s.mu.RUnlock()

	assert.Equal(t, sigBefore, sigAfter, "Create should not call notify")
}

func TestWatchCompletedRaceSafe(t *testing.T) {
	s := &Store{}

	id := mustCreate(t, s, Task{Title: "race-test"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Launch many concurrent watchers and updaters to stress the race detector.
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			_, _ = s.WatchCompleted(ctx, id)
		})
	}

	// Let watchers start waiting.
	time.Sleep(20 * time.Millisecond)

	// Complete the task to unblock all watchers.
	completed := StatusCompleted
	require.NoError(t, s.Update(id, Update{Status: &completed}))

	wg.Wait()

	// All watchers should have returned; verify the task is completed.
	task, ok := s.Get(id)
	require.True(t, ok)
	assert.Equal(t, StatusCompleted, task.Status)
}

func TestToolInvalidInput(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")

	toolNames := []string{
		"ns_tasks_create", "ns_tasks_list", "ns_tasks_get",
		"ns_tasks_claim", "ns_tasks_update", "ns_tasks_watch",
	}

	for _, name := range toolNames {
		t.Run(name, func(t *testing.T) {
			tool, _ := tb.Get(name)
			_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))
			assert.Error(t, err)
		})
	}
}
