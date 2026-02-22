// Package agentctx provides shared context key helpers for propagating agent
// identity across package boundaries. It is intentionally zero-dependency so
// both pkg/agent and pkg/engine can import it without creating cycles.
package agentctx

import "context"

type agentNameCtxKey struct{}

// WithAgentName returns a new context carrying the given agent name.
func WithAgentName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, agentNameCtxKey{}, name)
}

// AgentNameFromContext extracts the agent name from the context.
// Returns "" if no agent name is present.
func AgentNameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(agentNameCtxKey{}).(string)
	return v
}
