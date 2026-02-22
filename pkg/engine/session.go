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
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	"github.com/germanamz/shelly/pkg/codingtoolbox/filesystem"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

// Session represents one interactive conversation. It owns a chat and an agent
// instance. Only one Send call may be active at a time.
type Session struct {
	id           string
	agent        *agent.Agent
	events       *EventBus
	responder    *ask.Responder
	sessionTrust *filesystem.SessionTrust

	mu     sync.Mutex
	active bool
}

// newSession creates a session with the given ID, agent, event bus, and responder.
func newSession(id string, a *agent.Agent, events *EventBus, responder *ask.Responder) *Session {
	return &Session{
		id:           id,
		agent:        a,
		events:       events,
		responder:    responder,
		sessionTrust: &filesystem.SessionTrust{},
	}
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// Chat returns the underlying chat for direct observation (e.g., Wait/Since).
func (s *Session) Chat() *chat.Chat { return s.agent.Chat() }

// Completer returns the session's underlying completer for usage reporting.
func (s *Session) Completer() modeladapter.Completer { return s.agent.Completer() }

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

	ctx = withSessionID(ctx, s.id)
	ctx = withAgentName(ctx, s.agent.Name())
	ctx = filesystem.WithSessionTrust(ctx, s.sessionTrust)

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

// Respond delivers a user response to a pending ask_user question.
func (s *Session) Respond(questionID, response string) error {
	return s.responder.Respond(questionID, response)
}

// --- context helpers ---

type (
	sessionIDCtxKey struct{}
	agentNameCtxKey struct{}
)

func withSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDCtxKey{}, id)
}

func sessionIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(sessionIDCtxKey{}).(string)
	return v, ok
}

func withAgentName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, agentNameCtxKey{}, name)
}

func agentNameFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(agentNameCtxKey{}).(string)
	return v, ok
}
