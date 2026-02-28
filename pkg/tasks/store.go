// Package tasks provides a shared task board for multi-agent coordination.
// Agents create, discover, claim, and complete tasks through their normal
// tool-calling loop. The Store is thread-safe and supports blocking watch
// for task completion.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/germanamz/shelly/pkg/agentctx"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Status represents the lifecycle state of a task.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Task represents a unit of work on the shared task board.
type Task struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Status      Status   `json:"status"`
	Assignee    string   `json:"assignee,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	// Metadata holds arbitrary key-value pairs. Values should be JSON-serializable
	// primitives (string, float64, bool, nil). Mutable values (slices, maps) are
	// shallow-copied and must not be mutated after being passed to Create or Update.
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedBy string         `json:"created_by,omitempty"`
}

// Filter controls which tasks are returned by List.
type Filter struct {
	Status   *Status
	Assignee *string
	Blocked  *bool
}

// Update describes a partial update to a task's mutable fields.
// Nil pointer fields are left unchanged.
type Update struct {
	Status      *Status
	Description *string
	BlockedBy   *[]string
	Metadata    map[string]any
}

// Store is a thread-safe task board. The zero value is ready to use.
type Store struct {
	mu     sync.RWMutex
	once   sync.Once
	signal chan struct{}
	tasks  map[string]*Task
	nextID int
}

// init ensures internal structures are allocated.
func (s *Store) init() {
	s.once.Do(func() {
		s.tasks = make(map[string]*Task)
		s.signal = make(chan struct{})
	})
}

// notify wakes all goroutines blocked in WatchCompleted. Must be called with mu held.
func (s *Store) notify() {
	close(s.signal)
	s.signal = make(chan struct{})
}

// Create adds a new task to the board with status "pending" and returns its
// auto-generated ID. The following fields are always overridden by Create:
//   - ID: auto-assigned as "task-N" (sequential).
//   - Status: forced to StatusPending regardless of the supplied value.
//
// Callers must not set Status to a non-zero value or Assignee to a non-empty
// string; doing so returns an error because those fields would be silently
// discarded and likely indicate a caller bug.
func (s *Store) Create(task Task) (string, error) {
	if task.Status != "" && task.Status != StatusPending {
		return "", fmt.Errorf("tasks: Create does not accept Status %q; new tasks always start as %q", task.Status, StatusPending)
	}
	if task.Assignee != "" {
		return "", fmt.Errorf("tasks: Create does not accept Assignee %q; use Claim or Reassign after creation", task.Assignee)
	}

	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	task.ID = fmt.Sprintf("task-%d", s.nextID)
	task.Status = StatusPending

	cp := task
	if cp.BlockedBy != nil {
		cp.BlockedBy = slices.Clone(cp.BlockedBy)
	}
	if cp.Metadata != nil {
		cp.Metadata = maps.Clone(cp.Metadata)
	}
	s.tasks[cp.ID] = &cp

	return cp.ID, nil
}

// Get returns a copy of the task with the given ID, or false if not found.
func (s *Store) Get(id string) (Task, bool) {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[id]
	if !ok {
		return Task{}, false
	}

	return s.copyTask(t), true
}

// List returns tasks matching the given filter, sorted by ID.
func (s *Store) List(filter Filter) []Task {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Task
	for _, t := range s.tasks {
		if filter.Status != nil && t.Status != *filter.Status {
			continue
		}
		if filter.Assignee != nil && t.Assignee != *filter.Assignee {
			continue
		}
		if filter.Blocked != nil {
			blocked := s.isBlockedLocked(t.ID)
			if *filter.Blocked != blocked {
				continue
			}
		}
		result = append(result, s.copyTask(t))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// Update applies a partial update to the task with the given ID.
func (s *Store) Update(id string, upd Update) error {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("tasks: task %q not found", id)
	}

	if upd.Status != nil {
		switch *upd.Status {
		case StatusPending, StatusInProgress, StatusCompleted, StatusFailed:
		default:
			return fmt.Errorf("tasks: invalid status %q", *upd.Status)
		}
		t.Status = *upd.Status
	}
	if upd.Description != nil {
		t.Description = *upd.Description
	}
	if upd.BlockedBy != nil {
		t.BlockedBy = slices.Clone(*upd.BlockedBy)
	}
	if upd.Metadata != nil {
		if t.Metadata == nil {
			t.Metadata = make(map[string]any)
		}
		maps.Copy(t.Metadata, upd.Metadata)
	}

	s.notify()

	return nil
}

// Claim atomically assigns a task to the given agent and sets its status to
// "in_progress". It returns an error if the task is blocked, already assigned
// to a different agent, or in a terminal state.
func (s *Store) Claim(id, agent string) error {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("tasks: task %q not found", id)
	}

	if t.Status == StatusCompleted || t.Status == StatusFailed {
		return fmt.Errorf("tasks: task %q is in terminal state %q", id, t.Status)
	}

	if t.Assignee != "" && t.Assignee != agent {
		return fmt.Errorf("tasks: task %q is already assigned to %q", id, t.Assignee)
	}

	if s.isBlockedLocked(id) {
		return fmt.Errorf("tasks: task %q is blocked", id)
	}

	t.Assignee = agent
	t.Status = StatusInProgress
	s.notify()

	return nil
}

// Reassign atomically assigns a task to a new agent, overriding any
// existing assignee. Used by delegation tools to transfer ownership
// from the orchestrator to the actual executor. Returns an error if
// the task is blocked or in a terminal state.
func (s *Store) Reassign(id, agent string) error {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("tasks: task %q not found", id)
	}

	if t.Status == StatusCompleted || t.Status == StatusFailed {
		return fmt.Errorf("tasks: task %q is in terminal state %q", id, t.Status)
	}

	if s.isBlockedLocked(id) {
		return fmt.Errorf("tasks: task %q is blocked", id)
	}

	t.Assignee = agent
	t.Status = StatusInProgress
	s.notify()

	return nil
}

// WatchCompleted blocks until the task reaches "completed" or "failed",
// or the context is cancelled.
func (s *Store) WatchCompleted(ctx context.Context, id string) (Task, error) {
	s.init()

	for {
		s.mu.RLock()
		t, ok := s.tasks[id]
		if !ok {
			s.mu.RUnlock()
			return Task{}, fmt.Errorf("tasks: task %q not found", id)
		}

		if t.Status == StatusCompleted || t.Status == StatusFailed {
			cp := s.copyTask(t)
			s.mu.RUnlock()
			return cp, nil
		}

		sig := s.signal
		s.mu.RUnlock()

		select {
		case <-ctx.Done():
			return Task{}, ctx.Err()
		case <-sig:
		}
	}
}

// IsBlocked returns true if any of the task's BlockedBy dependencies are
// not yet completed.
func (s *Store) IsBlocked(id string) bool {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isBlockedLocked(id)
}

// isBlockedLocked is the lock-free implementation of IsBlocked.
// Must be called with at least s.mu.RLock held.
func (s *Store) isBlockedLocked(id string) bool {
	t, ok := s.tasks[id]
	if !ok {
		return false
	}

	for _, dep := range t.BlockedBy {
		dt, ok := s.tasks[dep]
		if !ok {
			return true
		}
		if dt.Status != StatusCompleted {
			return true
		}
	}

	return false
}

// copyTask returns a deep-enough copy of a Task for safe external use.
// Note: Metadata is shallow-copied via maps.Copy. Mutable values stored inside
// (slices, nested maps) are shared between the copy and the original. Callers
// must treat returned Metadata values as read-only or risk data races.
func (s *Store) copyTask(t *Task) Task {
	cp := *t
	if t.BlockedBy != nil {
		cp.BlockedBy = make([]string, len(t.BlockedBy))
		copy(cp.BlockedBy, t.BlockedBy)
	}
	if t.Metadata != nil {
		cp.Metadata = make(map[string]any, len(t.Metadata))
		maps.Copy(cp.Metadata, t.Metadata)
	}

	return cp
}

// --- Tool integration ---

// Tools returns a ToolBox with task management tools namespaced under the
// given prefix.
func (s *Store) Tools(namespace string) *toolbox.ToolBox {
	tb := toolbox.New()

	tb.Register(
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_tasks_create", namespace),
			Description: "Create a new task on the shared task board.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"},"description":{"type":"string"},"blocked_by":{"type":"array","items":{"type":"string"}},"metadata":{"type":"object"}},"required":["title"]}`),
			Handler:     s.handleCreate,
		},
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_tasks_list", namespace),
			Description: "List tasks on the shared task board with optional filters.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["pending","in_progress","completed","failed"]},"assignee":{"type":"string"},"blocked":{"type":"boolean"}}}`),
			Handler:     s.handleList,
		},
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_tasks_get", namespace),
			Description: "Get a task by ID from the shared task board.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
			Handler:     s.handleGet,
		},
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_tasks_claim", namespace),
			Description: "Claim a task for the calling agent. Sets status to in_progress.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
			Handler:     s.handleClaim,
		},
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_tasks_update", namespace),
			Description: "Update a task's mutable fields (status, description, blocked_by, metadata).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed","failed"]},"description":{"type":"string"},"blocked_by":{"type":"array","items":{"type":"string"}},"metadata":{"type":"object"}},"required":["id"]}`),
			Handler:     s.handleUpdate,
		},
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_tasks_watch", namespace),
			Description: "Block until a task reaches completed or failed status.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
			Handler:     s.handleWatch,
		},
	)

	return tb
}

// --- Tool handler input types ---

type createInput struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	BlockedBy   []string       `json:"blocked_by"`
	Metadata    map[string]any `json:"metadata"`
}

type listInput struct {
	Status   *Status `json:"status"`
	Assignee *string `json:"assignee"`
	Blocked  *bool   `json:"blocked"`
}

type idInput struct {
	ID string `json:"id"`
}

type updateInput struct {
	ID          string         `json:"id"`
	Status      *Status        `json:"status"`
	Description *string        `json:"description"`
	BlockedBy   *[]string      `json:"blocked_by"`
	Metadata    map[string]any `json:"metadata"`
}

// --- Tool handlers ---

func (s *Store) handleCreate(ctx context.Context, input json.RawMessage) (string, error) {
	var in createInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.Title == "" {
		return "", errors.New("title is required")
	}

	task := Task{
		Title:       in.Title,
		Description: in.Description,
		BlockedBy:   in.BlockedBy,
		Metadata:    in.Metadata,
		CreatedBy:   agentctx.AgentNameFromContext(ctx),
	}

	id, err := s.Create(task)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return "", fmt.Errorf("failed to encode result: %w", err)
	}

	return string(b), nil
}

func (s *Store) handleList(_ context.Context, input json.RawMessage) (string, error) {
	var in listInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	filter := Filter(in)

	tasks := s.List(filter)

	b, err := json.Marshal(tasks)
	if err != nil {
		return "", fmt.Errorf("failed to encode tasks: %w", err)
	}

	return string(b), nil
}

func (s *Store) handleGet(_ context.Context, input json.RawMessage) (string, error) {
	var in idInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	task, ok := s.Get(in.ID)
	if !ok {
		return "", fmt.Errorf("task %q not found", in.ID)
	}

	b, err := json.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("failed to encode task: %w", err)
	}

	return string(b), nil
}

func (s *Store) handleClaim(ctx context.Context, input json.RawMessage) (string, error) {
	var in idInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	agent := agentctx.AgentNameFromContext(ctx)
	if err := s.Claim(in.ID, agent); err != nil {
		return "", err
	}

	return "ok", nil
}

func (s *Store) handleUpdate(_ context.Context, input json.RawMessage) (string, error) {
	var in updateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	upd := Update{
		Status:      in.Status,
		Description: in.Description,
		BlockedBy:   in.BlockedBy,
		Metadata:    in.Metadata,
	}

	if err := s.Update(in.ID, upd); err != nil {
		return "", err
	}

	return "ok", nil
}

func (s *Store) handleWatch(ctx context.Context, input json.RawMessage) (string, error) {
	var in idInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	task, err := s.WatchCompleted(ctx, in.ID)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("failed to encode task: %w", err)
	}

	return string(b), nil
}
