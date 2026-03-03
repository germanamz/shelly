# batch

A batching decorator for the `Completer` interface. The batch package collects concurrent `Complete()` calls and submits them as a single batch to provider-specific batch APIs for cost reduction (typically 50% discount on all tokens).

## Architecture

```
batch/
├── batch.go          Submitter interface, Request/Result types, pendingRequest
└── collector.go      Collector — shared request accumulator, flush + poll loop,
                      sync fallback, event notifications, UsageReporter
```

### `Submitter` — Provider-Specific Batch Interface

Each LLM provider implements `Submitter` to translate requests into their native batch API format:

```go
type Submitter interface {
    SubmitBatch(ctx context.Context, reqs []Request) (batchID string, err error)
    PollBatch(ctx context.Context, batchID string) (results map[string]Result, done bool, err error)
    CancelBatch(ctx context.Context, batchID string) error
}
```

- `SubmitBatch` sends a batch of requests and returns a batch ID for polling.
- `PollBatch` checks batch status. Returns `done=true` with results when complete.
- `CancelBatch` attempts to cancel an in-progress batch.

### `Request` and `Result`

```go
type Request struct {
    ID    string           // UUID for correlation (maps to provider's custom_id).
    Chat  *chat.Chat
    Tools []toolbox.Tool
}

type Result struct {
    Message message.Message
    Usage   usage.TokenCount
    Err     error
}
```

### `Collector` — Batching Completer Decorator

`Collector` implements `modeladapter.Completer` and `modeladapter.UsageReporter`. It is the main entry point for batch support.

When `Complete()` is called:
1. A `Request` is created with a UUID and enqueued.
2. If the collect window timer is not running, it starts.
3. When the timer fires OR the queue hits `MaxBatchSize`, all pending requests are flushed as one batch.
4. A background goroutine polls for results and delivers them to each request's channel.
5. On timeout or error, all requests fall back to synchronous `inner.Complete()`.

`Collector` is **shared per provider** so concurrent sub-agents using the same provider naturally batch together.

```go
c := batch.NewCollector(innerCompleter, submitter, batch.CollectorOpts{
    CollectWindow: 500 * time.Millisecond,
    PollInterval:  5 * time.Second,
    Timeout:       time.Hour,
    MaxBatchSize:  100,
})

// Use c anywhere a Completer is expected.
msg, err := c.Complete(ctx, chat, tools)
```

### `CollectorOpts`

| Field           | Default  | Description                                      |
|-----------------|----------|--------------------------------------------------|
| `CollectWindow` | 500ms    | How long to wait for more requests before flush   |
| `PollInterval`  | 5s       | How often to poll batch status                    |
| `Timeout`       | 1h       | Maximum time to wait for batch results            |
| `MaxBatchSize`  | 100      | Auto-flush threshold for pending requests         |

### Event Notifications

Register a handler to receive batch lifecycle events:

```go
c.SetEventHandler(func(kind batch.EventKind, data map[string]any) {
    log.Printf("batch event: %s %v", kind, data)
})
```

| Event Kind        | Data                        |
|-------------------|-----------------------------|
| `batch_submitted` | `count` (number of requests)|
| `batch_polling`   | `batch_id`, `count`         |
| `batch_completed` | `batch_id`, `count`         |

### Fallback Behavior

On any batch error (submit failure, poll failure, timeout), the Collector automatically falls back to synchronous completion using the inner completer. This ensures reliability — batch mode degrades gracefully to normal operation.

### Test Hooks

For deterministic testing: `SetNowFunc`, `SetSleepFunc`, `SetUUIDFunc`.

## Dependencies

- `pkg/modeladapter` — `Completer`, `UsageReporter` interfaces
- `pkg/chats/chat` — `Chat` type
- `pkg/chats/message` — `Message` type
- `pkg/modeladapter/usage` — `TokenCount`, `Tracker`
- `pkg/tools/toolbox` — `Tool` type
- `github.com/google/uuid` — UUID generation for request correlation
