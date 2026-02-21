package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventBus_SubscribePublish(t *testing.T) {
	bus := NewEventBus()
	sub := bus.Subscribe(8)
	defer bus.Unsubscribe(sub)

	e := Event{
		Kind:      EventAgentStart,
		SessionID: "s1",
		Agent:     "bot",
		Timestamp: time.Now(),
	}

	bus.Publish(e)

	select {
	case got := <-sub.C:
		assert.Equal(t, EventAgentStart, got.Kind)
		assert.Equal(t, "s1", got.SessionID)
		assert.Equal(t, "bot", got.Agent)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventBus_FanOut(t *testing.T) {
	bus := NewEventBus()
	sub1 := bus.Subscribe(4)
	sub2 := bus.Subscribe(4)
	defer bus.Unsubscribe(sub1)
	defer bus.Unsubscribe(sub2)

	bus.Publish(Event{Kind: EventMessageAdded})

	select {
	case <-sub1.C:
	case <-time.After(time.Second):
		t.Fatal("sub1 did not receive event")
	}

	select {
	case <-sub2.C:
	case <-time.After(time.Second):
		t.Fatal("sub2 did not receive event")
	}
}

func TestEventBus_NonBlockingDrop(t *testing.T) {
	bus := NewEventBus()
	sub := bus.Subscribe(1) // buffer of 1
	defer bus.Unsubscribe(sub)

	// Fill the buffer.
	bus.Publish(Event{Kind: EventAgentStart})
	// This should not block — event is dropped.
	bus.Publish(Event{Kind: EventAgentEnd})

	got := <-sub.C
	assert.Equal(t, EventAgentStart, got.Kind)

	select {
	case <-sub.C:
		t.Fatal("expected channel to be empty after drop")
	default:
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()
	sub := bus.Subscribe(4)

	bus.Unsubscribe(sub)

	// Channel should be closed.
	_, ok := <-sub.C
	assert.False(t, ok, "channel should be closed after unsubscribe")

	// Double unsubscribe should not panic.
	bus.Unsubscribe(sub)
}

func TestEventBus_PublishNoSubscribers(t *testing.T) {
	bus := NewEventBus()
	// Should not panic.
	bus.Publish(Event{Kind: EventError})
}
