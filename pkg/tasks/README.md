# tasks

Shared task board for multi-agent coordination. Agents create, discover, claim, and complete tasks through their normal tool-calling loop.

## Architecture

```
tasks/
├── store.go        Task type, Store, CRUD/Watch methods, Tools integration
└── store_test.go   Tests including concurrency and tool integration
```

### Store

```go
s := &tasks.Store{} // zero value is ready to use

id := s.Create(tasks.Task{Title: "Research X"})
task, ok := s.Get(id)
tasks := s.List(tasks.Filter{Status: &pending})
s.Update(id, tasks.Update{Status: &completed})
s.Claim(id, "worker-1")       // atomic assign + in_progress
task, err := s.WatchCompleted(ctx, id) // blocks until done
blocked := s.IsBlocked(id)
```

All methods are safe for concurrent use (`sync.RWMutex` internally).

### Task Lifecycle

```
pending → in_progress → completed
                      → failed
```

- `Create` always sets status to `pending`
- `Claim` atomically sets assignee + status to `in_progress`
- `Update` can set any status
- `WatchCompleted` blocks until `completed` or `failed`

### Blocking & Dependencies

Tasks can declare `BlockedBy` dependencies. A blocked task cannot be claimed until all dependencies reach `completed` status. The `List` filter supports `Blocked: &true/&false` to find available work.

### Tool Integration

```go
tb := s.Tools("shared")
// Registers 6 tools:
//   shared_tasks_create, shared_tasks_list, shared_tasks_get,
//   shared_tasks_claim, shared_tasks_update, shared_tasks_watch
```

| Tool | Input | Output |
|------|-------|--------|
| `{ns}_tasks_create` | `{title, description?, blocked_by?, metadata?}` | `{"id":"task-1"}` |
| `{ns}_tasks_list` | `{status?, assignee?, blocked?}` | JSON array of tasks |
| `{ns}_tasks_get` | `{id}` | Full task JSON |
| `{ns}_tasks_claim` | `{id}` | `"ok"` |
| `{ns}_tasks_update` | `{id, status?, description?, blocked_by?, metadata?}` | `"ok"` |
| `{ns}_tasks_watch` | `{id}` | Final task JSON |

Tool handlers read agent identity via `agentctx.AgentNameFromContext(ctx)` for `create` (sets `created_by`) and `claim` (sets `assignee`).

## Typical Workflow

```
1. Orchestrator creates tasks via shared_tasks_create
2. Orchestrator spawns workers via spawn_agents
3. Workers discover work via shared_tasks_list({status:"pending", blocked:false})
4. Worker claims a task via shared_tasks_claim (atomic, race-safe)
5. Worker completes via shared_tasks_update({id, status:"completed"})
6. Blocked tasks auto-unblock when dependencies complete
7. Orchestrator watches via shared_tasks_watch({id})
```

## Configuration

Enable in YAML config:

```yaml
tasks_enabled: true
```
