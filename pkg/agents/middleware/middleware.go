// Package middleware provides composable middleware for agents.Agent.
// Each middleware wraps an Agent's Run method, and the wrapped value is itself
// an Agent, so middleware composes naturally via Chain or Apply.
//
// If the inner agent implements reactor.NamedAgent, every middleware wrapper
// preserves AgentName() and AgentChat() by delegating to the inner agent.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/germanamz/shelly/pkg/agents"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/reactor"
)

// Middleware wraps an Agent, returning a new Agent with added behaviour.
type Middleware func(next agents.Agent) agents.Agent

// Chain composes multiple middleware into a single Middleware.
// The first middleware in the list is the outermost (runs first).
func Chain(mws ...Middleware) Middleware {
	return func(next agents.Agent) agents.Agent {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// Apply wraps an agent with the given middleware. The first middleware
// in the list is the outermost (runs first).
func Apply(agent agents.Agent, mws ...Middleware) agents.Agent {
	return Chain(mws...)(agent)
}

// --- NamedAgent helper ---

// namedAgentBase provides NamedAgent delegation for middleware wrappers.
// If the inner agent implements reactor.NamedAgent, the wrapper delegates
// AgentName and AgentChat. Otherwise it returns zero values.
type namedAgentBase struct {
	next agents.Agent
}

func (n *namedAgentBase) AgentName() string {
	if na, ok := n.next.(reactor.NamedAgent); ok {
		return na.AgentName()
	}
	return ""
}

func (n *namedAgentBase) AgentChat() *chat.Chat {
	if na, ok := n.next.(reactor.NamedAgent); ok {
		return na.AgentChat()
	}
	return nil
}

// --- Timeout middleware ---

type timeoutAgent struct {
	namedAgentBase
	timeout time.Duration
}

func (a *timeoutAgent) Run(ctx context.Context) (message.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	return a.next.Run(ctx)
}

// Timeout returns a Middleware that wraps the agent's context with a deadline.
func Timeout(d time.Duration) Middleware {
	return func(next agents.Agent) agents.Agent {
		return &timeoutAgent{
			namedAgentBase: namedAgentBase{next: next},
			timeout:        d,
		}
	}
}

// --- Recovery middleware ---

type recoveryAgent struct {
	namedAgentBase
}

func (a *recoveryAgent) Run(ctx context.Context) (msg message.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("agent panicked: %v", r)
		}
	}()

	return a.next.Run(ctx)
}

// Recovery returns a Middleware that catches panics and converts them to errors.
func Recovery() Middleware {
	return func(next agents.Agent) agents.Agent {
		return &recoveryAgent{
			namedAgentBase: namedAgentBase{next: next},
		}
	}
}

// --- Logger middleware ---

type loggerAgent struct {
	namedAgentBase
	log *slog.Logger
}

func (a *loggerAgent) Run(ctx context.Context) (message.Message, error) {
	name := a.AgentName()
	a.log.InfoContext(ctx, "agent started", "agent", name)

	start := time.Now()

	msg, err := a.next.Run(ctx)

	duration := time.Since(start)

	if err != nil {
		a.log.ErrorContext(ctx, "agent finished with error",
			"agent", name,
			"duration", duration,
			"error", err,
		)
	} else {
		a.log.InfoContext(ctx, "agent finished",
			"agent", name,
			"duration", duration,
		)
	}

	return msg, err
}

// Logger returns a Middleware that logs agent start, duration, and error.
// If the inner agent implements reactor.NamedAgent, the agent's name is
// included in the log attributes.
func Logger(log *slog.Logger) Middleware {
	return func(next agents.Agent) agents.Agent {
		return &loggerAgent{
			namedAgentBase: namedAgentBase{next: next},
			log:            log,
		}
	}
}

// --- OutputGuardrail middleware ---

type guardrailAgent struct {
	namedAgentBase
	check func(message.Message) error
}

func (a *guardrailAgent) Run(ctx context.Context) (message.Message, error) {
	msg, err := a.next.Run(ctx)
	if err != nil {
		return msg, err
	}

	if checkErr := a.check(msg); checkErr != nil {
		return message.Message{}, checkErr
	}

	return msg, nil
}

// OutputGuardrail returns a Middleware that validates the final message from
// the agent. If check returns an error, that error is returned instead of
// the message.
func OutputGuardrail(check func(message.Message) error) Middleware {
	return func(next agents.Agent) agents.Agent {
		return &guardrailAgent{
			namedAgentBase: namedAgentBase{next: next},
			check:          check,
		}
	}
}
