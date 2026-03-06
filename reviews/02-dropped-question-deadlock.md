# Dropped Question Causes Child Agent Deadlock

## Severity: Critical

## Location

- `pkg/agent/delegation.go:256-260` — `waitForInitialChildResponse`
- `pkg/agent/interaction_registry.go:197-201` — `waitForChildResponse`

## Description

Both `waitForInitialChildResponse` and `waitForChildResponse` consume from the shared `reg.questions` channel. When they receive a question belonging to a different delegation, they attempt a non-blocking put-back:

```go
select {
case reg.questions <- pq:
default:
    // Channel full; drop and let the child retry.
}
```

The comment says "let the child retry", but `request_input` in `interaction.go` does not retry. After sending the question to `sharedQueue`, the tool blocks indefinitely on `ic.answerCh`. If the question is dropped because the 16-slot buffer is full, the child agent hangs until its context is cancelled.

This path is reachable when multiple concurrent children are asking questions and the buffer fills up.

## Fix

Options:

- **Per-child channel routing**: Use a separate buffered channel per delegation instead of the shared queue, eliminating cross-delegation interference.
- **Blocking re-enqueue**: Never drop questions — block on re-enqueue while holding nothing, so the question is guaranteed to be re-consumed by the correct waiter.
- **Retry in `request_input`**: Add retry logic in the `request_input` tool handler so dropped questions are re-sent. This is the least desirable option since it adds complexity to the tool side.
