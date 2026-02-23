package agent

import (
	"context"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

// IterationPhase indicates when an effect runs within a single ReAct iteration.
type IterationPhase int

const (
	// PhaseBeforeComplete runs before the LLM call.
	PhaseBeforeComplete IterationPhase = iota
	// PhaseAfterComplete runs after the LLM reply, before tool dispatch.
	PhaseAfterComplete
)

// IterationContext provides per-iteration state to effects without exposing the
// full Agent.
type IterationContext struct {
	Phase     IterationPhase
	Iteration int
	Chat      *chat.Chat
	Completer modeladapter.Completer
	AgentName string
}

// Effect is a dynamic, per-iteration hook that runs inside the ReAct loop.
// Effects run synchronously in registration order. Returning an error aborts
// the loop.
type Effect interface {
	Eval(ctx context.Context, ic IterationContext) error
}

// EffectFunc is an adapter that lets ordinary functions implement Effect.
type EffectFunc func(ctx context.Context, ic IterationContext) error

// Eval calls f(ctx, ic).
func (f EffectFunc) Eval(ctx context.Context, ic IterationContext) error { return f(ctx, ic) }
