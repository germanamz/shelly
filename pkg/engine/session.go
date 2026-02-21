package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// Session represents one interactive conversation. It owns a chat and an agent
// instance. Only one Send call may be active at a time.
type Session struct {
	id     string
	agent  *agent.Agent
	events *EventBus

	mu     sync.Mutex
	active bool
}

// newSession creates a session with the given ID, agent, and event bus.
func newSession(id string, a *agent.Agent, events *EventBus) *Session {
	return &Session{
		id:     id,
		agent:  a,
		events: events,
	}
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// Chat returns the underlying chat for direct observation (e.g., Wait/Since).
func (s *Session) Chat() *chat.Chat { return s.agent.Chat() }

// Send appends a text message from the user and runs the agent's ReAct loop.
// It returns the agent's reply. Only one Send may be active per session.
func (s *Session) Send(ctx context.Context, text string) (message.Message, error) {
	return s.SendParts(ctx, content.Text{Text: text})
}

// SendParts appends a user message with the given parts and runs the agent's
// ReAct loop. Only one Send may be active per session.
func (s *Session) SendParts(ctx context.Context, parts ...content.Part) (message.Message, error) {
	if err := s.acquire(); err != nil {
		return message.Message{}, err
	}
	defer s.release()

	s.events.Publish(Event{
		Kind:      EventAgentStart,
		SessionID: s.id,
		Agent:     s.agent.Name(),
		Timestamp: time.Now(),
	})

	s.agent.Chat().Append(message.New("user", role.User, parts...))

	reply, err := s.agent.Run(ctx)
	if err != nil {
		s.events.Publish(Event{
			Kind:      EventError,
			SessionID: s.id,
			Agent:     s.agent.Name(),
			Timestamp: time.Now(),
			Data:      err,
		})
		s.events.Publish(Event{
			Kind:      EventAgentEnd,
			SessionID: s.id,
			Agent:     s.agent.Name(),
			Timestamp: time.Now(),
		})
		return message.Message{}, err
	}

	s.events.Publish(Event{
		Kind:      EventAgentEnd,
		SessionID: s.id,
		Agent:     s.agent.Name(),
		Timestamp: time.Now(),
	})

	return reply, nil
}

func (s *Session) acquire() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return fmt.Errorf("engine: session %s: another Send is already active", s.id)
	}
	s.active = true
	return nil
}

func (s *Session) release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.active = false
}
