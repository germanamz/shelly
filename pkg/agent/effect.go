package agent

import (
	"context"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
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
	Phase           IterationPhase
	Iteration       int
	Chat            *chat.Chat
	Completer       modeladapter.Completer
	AgentName       string
	EstimatedTokens int // Pre-call token estimate (chat + tools). 0 = not computed.
	ToolTokens      int // Cached token cost of tool definitions. 0 = not computed.
}

// Effect is a dynamic, per-iteration hook that runs inside the ReAct loop.
// Effects run synchronously in registration order. Returning an error aborts
// the loop.
type Effect interface {
	Eval(ctx context.Context, ic IterationContext) error
}

// Resetter is an optional interface that effects can implement to reset
// their internal state between agent runs. Effects that track per-run
// state (e.g. injection guards) should implement this.
type Resetter interface {
	Reset()
}

// ToolFilter is an optional interface that effects can implement to filter
// which tools are sent to the LLM on each iteration. Multiple filters are
// applied sequentially (intersection semantics).
type ToolFilter interface {
	FilterTools(ctx context.Context, ic IterationContext, tools []toolbox.Tool) []toolbox.Tool
}

// ToolProvider is an optional interface that effects can implement to provide
// additional tools that should be added to the agent's toolbox.
type ToolProvider interface {
	ProvidedTools() *toolbox.ToolBox
}

// EffectFunc is an adapter that lets ordinary functions implement Effect.
type EffectFunc func(ctx context.Context, ic IterationContext) error

// Eval calls f(ctx, ic).
func (f EffectFunc) Eval(ctx context.Context, ic IterationContext) error { return f(ctx, ic) }
