package engine

import (
	"context"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/agentctx"
)

// EventKind identifies the type of engine event.
type EventKind string

const (
	EventMessageAdded       EventKind = "message_added"
	EventToolCallStart      EventKind = "tool_call_start"
	EventToolCallEnd        EventKind = "tool_call_end"
	EventAgentStart         EventKind = "agent_start"
	EventAgentEnd           EventKind = "agent_end"
	EventAskUser            EventKind = "ask_user"
	EventFileChange         EventKind = "file_change"
	EventCompaction         EventKind = "compaction"
	EventError              EventKind = "error"
	EventBatchSubmitted     EventKind = "batch_submitted"
	EventBatchPolling       EventKind = "batch_polling"
	EventBatchCompleted     EventKind = "batch_completed"
	EventBatchFallback      EventKind = "batch_fallback"
	EventDelegationProgress EventKind = "delegation_progress"
)

// Event is an immutable notification of engine activity.
type Event struct {
	Kind      EventKind
	SessionID string
	Agent     string
	Timestamp time.Time
	Data      any
}

// Subscription receives events from an EventBus.
type Subscription struct {
	C  <-chan Event
	ch chan Event
}

// EventBus fans out events to all active subscribers. It is safe for
// concurrent use.
type EventBus struct {
	mu   sync.RWMutex
	subs map[*Subscription]struct{}
}

// NewEventBus creates an EventBus ready for use.
func NewEventBus() *EventBus {
	return &EventBus{
		subs: make(map[*Subscription]struct{}),
	}
}

// Subscribe creates a new subscription with the given channel buffer size.
// The caller should read from sub.C and eventually call Unsubscribe.
func (b *EventBus) Subscribe(bufSize int) *Subscription {
	ch := make(chan Event, bufSize)
	sub := &Subscription{C: ch, ch: ch}

	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()

	return sub
}

// Unsubscribe removes the subscription and closes its channel.
func (b *EventBus) Unsubscribe(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.subs[sub]; ok {
		delete(b.subs, sub)
		close(sub.ch)
	}
}

// Publish sends an event to all subscribers. If a subscriber's buffer is full
// the event is dropped for that subscriber to prevent slow consumers from
// stalling the agent loop.
func (b *EventBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subs {
		select {
		case sub.ch <- e:
		default:
		}
	}
}

// publish constructs an Event with time.Now() and publishes it.
func (b *EventBus) publish(kind EventKind, sessionID, agentName string, data any) {
	b.Publish(Event{
		Kind:      kind,
		SessionID: sessionID,
		Agent:     agentName,
		Timestamp: time.Now(),
		Data:      data,
	})
}

// publishFromContext extracts session ID and agent name from ctx and publishes.
func publishFromContext(bus *EventBus, ctx context.Context, kind EventKind, data any) {
	sid, _ := sessionIDFromContext(ctx)
	aname := agentctx.AgentNameFromContext(ctx)
	bus.publish(kind, sid, aname, data)
}

// --- context helpers for session ID ---

type sessionIDCtxKey struct{}

func withSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDCtxKey{}, id)
}

func sessionIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(sessionIDCtxKey{}).(string)
	return v, ok
}
