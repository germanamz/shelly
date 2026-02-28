# tasks

Package `tasks` provides a shared task board for multi-agent coordination. Agents create, discover, claim, and complete tasks through their normal tool-calling loop. The `Store` is thread-safe and supports blocking watch for task completion.

## Architecture

```
tasks/
  store.go        Task/Status/Filter/Update types, Store with CRUD/Claim/Reassign/Watch, Tools integration
  store_test.go   Tests including concurrency, race safety, and tool integration
```

## Exported Types

### Status

```go
type Status string

const (
    StatusPending    Status = "pending"
    StatusInProgress Status = "in_progress"
    StatusCompleted  Status = "completed"
    StatusFailed     Status = "failed"
)
```

### Task

```go
type Task struct {
    ID          string         `json:"id"`
    Title       string         `json:"title"`
    Description string         `json:"description,omitempty"`
    Status      Status         `json:"status"`
    Assignee    string         `json:"assignee,omitempty"`
    BlockedBy   []string       `json:"blocked_by,omitempty"`
    Metadata    map[string]any `json:"metadata,omitempty"`
    CreatedBy   string         `json:"created_by,omitempty"`
}
```

`Metadata` values should be JSON-serializable primitives (`string`, `float64`, `bool`, `nil`). Mutable values (slices, maps) are shallow-copied and must not be mutated after being passed to `Create` or `Update`.

### Filter

```go
type Filter struct {
    Status   *Status
    Assignee *string
    Blocked  *bool
}
```

Controls which tasks are returned by `List`. All fields are optional; `nil` means no filter on that dimension.

### Update

```go
type Update struct {
    Status      *Status
    Description *string
    BlockedBy   *[]string
    Metadata    map[string]any
}
```

Describes a partial update to a task's mutable fields. `nil` pointer fields are left unchanged. `Metadata` is merged into existing metadata (not replaced).

### Store

```go
type Store struct { /* unexported fields */ }
```

A thread-safe task board. The zero value is ready to use. Internally uses `sync.RWMutex` for concurrency safety, `sync.Once` for lazy initialization, and a signal channel pattern for `WatchCompleted` notifications.

## Store Methods

### Core Operations

- **`Create(task Task) (string, error)`** -- adds a new task with auto-generated sequential ID (`task-1`, `task-2`, ...) and forces status to `pending`. Returns an error if the caller sets `Status` to a non-pending value, provides an `Assignee` (use `Claim` or `Reassign` after creation), lists a `BlockedBy` ID that does not exist in the store, or includes a self-referential `BlockedBy` entry.
- **`Get(id string) (Task, bool)`** -- returns a deep copy of the task with the given ID, or `false` if not found.
- **`List(filter Filter) []Task`** -- returns tasks matching the filter, sorted by ID. Supports filtering by status, assignee, and blocked state.
- **`Update(id string, upd Update) error`** -- applies a partial update to the task. Validates status values. Metadata is merged into existing metadata. Does not validate `BlockedBy` IDs.
- **`Claim(id, agent string) error`** -- atomically assigns a task to the given agent and sets status to `in_progress`. Returns an error if the task is blocked, already assigned to a different agent, or in a terminal state (`completed`/`failed`). Re-claiming by the same agent is idempotent.
- **`Reassign(id, agent string) error`** -- atomically assigns a task to a new agent, overriding any existing assignee. Used by delegation tools to transfer ownership. Returns an error if the task is blocked or in a terminal state.
- **`IsBlocked(id string) bool`** -- returns `true` if any of the task's `BlockedBy` dependencies are not yet completed. Nonexistent dependencies are considered blocking.

### Blocking Watch

```go
func (s *Store) WatchCompleted(ctx context.Context, id string) (Task, error)
```

Blocks until the task reaches `completed` or `failed` status, or the context is cancelled. Returns a deep copy of the final task state.

### Tool Integration

```go
func (s *Store) Tools(namespace string) *toolbox.ToolBox
```

Returns a `ToolBox` with six tools namespaced under the given prefix:

| Tool | Input | Output |
|------|-------|--------|
| `{ns}_tasks_create` | `{title, description?, blocked_by?, metadata?}` | `{"id":"task-1"}` |
| `{ns}_tasks_list` | `{status?, assignee?, blocked?}` | JSON array of tasks |
| `{ns}_tasks_get` | `{id}` | Full task JSON |
| `{ns}_tasks_claim` | `{id}` | `"ok"` |
| `{ns}_tasks_update` | `{id, status?, description?, blocked_by?, metadata?}` | `"ok"` |
| `{ns}_tasks_watch` | `{id}` | Final task JSON |

Tool handlers read agent identity via `agentctx.AgentNameFromContext(ctx)` for `create` (sets `created_by`) and `claim` (sets `assignee`).

## Task Lifecycle

```
pending --> in_progress --> completed
                       --> failed
```

- `Create` always sets status to `pending`
- `Claim` atomically sets assignee + status to `in_progress` (rejects if already assigned to a different agent)
- `Reassign` atomically overrides the assignee + status to `in_progress` (used by delegation tools to transfer ownership)
- `Update` can set any valid status
- `WatchCompleted` blocks until `completed` or `failed`

## Blocking and Dependencies

Tasks can declare `BlockedBy` dependencies (a list of task IDs). `Create` validates that all `BlockedBy` IDs exist in the store and rejects self-referential entries. `Update` does not validate `BlockedBy` IDs, so nonexistent IDs can be introduced via updates. A blocked task cannot be claimed or reassigned until all dependencies reach `completed` status. Nonexistent dependency IDs are treated as blocking. The `List` filter supports `Blocked: &true` / `&false` to find available or blocked work.

## Typical Workflow

```
1. Orchestrator creates tasks via {ns}_tasks_create
2. Orchestrator delegates to workers via delegate
3. Workers discover work via {ns}_tasks_list({status:"pending", blocked:false})
4. Worker claims a task via {ns}_tasks_claim (atomic, race-safe)
5. Worker completes via {ns}_tasks_update({id, status:"completed"})
6. Blocked tasks unblock when dependencies complete
7. Orchestrator watches via {ns}_tasks_watch({id})
```

The `pkg/agent` package provides additional integration on top of this store: `delegate` supports an optional `task_id` parameter for automatic claim/reassign, and sub-agents receive a `task_complete` tool for structured completion. See `pkg/agent/README.md` for details.

## Dependencies

- `pkg/agentctx` -- for reading agent identity from context in tool handlers.
- `pkg/tools/toolbox` -- for the `Tools()` method that exposes task operations as agent-callable tools.
