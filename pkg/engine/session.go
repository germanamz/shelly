package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/agentctx"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	"github.com/germanamz/shelly/pkg/codingtoolbox/filesystem"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

// ProviderInfo holds the provider kind and model for display purposes.
type ProviderInfo struct {
	Kind  string // Provider type (anthropic, openai, grok, gemini).
	Model string // Model identifier.
}

// Label returns a display string like "anthropic/claude-sonnet-4".
func (p ProviderInfo) Label() string {
	if p.Kind == "" {
		return ""
	}
	return p.Kind + "/" + p.Model
}

// Session represents one interactive conversation. It owns a chat and an agent
// instance. Only one Send call may be active at a time.
type Session struct {
	id           string
	agent        *agent.Agent
	providerInfo ProviderInfo
	lifecycle    sessionLifecycle
	events       *EventBus
	responder    *ask.Responder
	sessionTrust *filesystem.SessionTrust

	mu     sync.Mutex
	active bool
}

// newSession creates a session with the given ID, agent, lifecycle coordinator,
// event bus, and responder.
func newSession(id string, a *agent.Agent, lc sessionLifecycle, events *EventBus, responder *ask.Responder) *Session {
	return &Session{
		id:           id,
		agent:        a,
		lifecycle:    lc,
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

// ProviderInfo returns the session's provider metadata.
func (s *Session) ProviderInfo() ProviderInfo { return s.providerInfo }

// Send appends a text message from the user and runs the agent's ReAct loop.
// It returns the agent's reply. Only one Send may be active per session.
func (s *Session) Send(ctx context.Context, text string) (message.Message, error) {
	return s.SendParts(ctx, content.Text{Text: text})
}

// SendParts appends a user message with the given parts and runs the agent's
// ReAct loop. Only one Send may be active per session.
func (s *Session) SendParts(ctx context.Context, parts ...content.Part) (message.Message, error) {
	// Check whether the engine has been closed before starting work.
	if err := s.lifecycle.acquireSend(); err != nil {
		return message.Message{}, err
	}
	defer s.lifecycle.releaseSend()

	if err := s.acquire(); err != nil {
		return message.Message{}, err
	}
	defer s.release()

	s.events.publish(EventAgentStart, s.id, s.agent.Name(), agent.AgentEventData{Prefix: s.agent.Prefix(), ProviderLabel: s.agent.ProviderLabel()})

	ctx = withSessionID(ctx, s.id)
	ctx = agentctx.WithAgentName(ctx, s.agent.Name())
	ctx = filesystem.WithSessionTrust(ctx, s.sessionTrust)

	s.agent.Chat().Append(message.New("user", role.User, parts...))

	reply, err := s.agent.Run(ctx)
	if err != nil {
		s.events.publish(EventError, s.id, s.agent.Name(), err)
		s.events.publish(EventAgentEnd, s.id, s.agent.Name(), agent.AgentEventData{Prefix: s.agent.Prefix(), ProviderLabel: s.agent.ProviderLabel()})
		return message.Message{}, err
	}

	s.events.publish(EventAgentEnd, s.id, s.agent.Name(), agent.AgentEventData{Prefix: s.agent.Prefix(), ProviderLabel: s.agent.ProviderLabel(), Summary: reply.TextContent()})

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

// AgentName returns the name of the session's agent.
func (s *Session) AgentName() string { return s.agent.Name() }

// Respond delivers a user response to a pending ask_user question.
func (s *Session) Respond(questionID, response string) error {
	return s.responder.Respond(questionID, response)
}
