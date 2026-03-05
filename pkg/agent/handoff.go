package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// HandoffResult carries peer handoff data from a sub-agent. Set by the
// handoff tool, read by the delegation machinery to spawn a peer agent.
type HandoffResult struct {
	TargetAgent string `json:"target_agent"`
	Reason      string `json:"reason"`
	Context     string `json:"context"` // Context to pass to the peer.
}

// handoffHandler manages the handoff tool state for sub-agents.
// It wraps a HandoffResult with a sync.Once guard to ensure at-most-once
// semantics, similar to completionHandler.
type handoffHandler struct {
	result *HandoffResult
	once   sync.Once
}

// tool returns the handoff tool definition.
func (hh *handoffHandler) tool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "handoff",
		Description: "Transfer control to a peer agent. Use this when the task requires expertise outside your capabilities and another agent is better suited. The current agent's loop stops and the peer continues with the provided context.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"target_agent":{"type":"string","description":"Name of the agent to hand off to"},"reason":{"type":"string","description":"Why you are handing off to this agent"},"context":{"type":"string","description":"Summary of relevant state and progress to pass to the peer agent"}},"required":["target_agent","reason","context"]}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var hi handoffInput
			if err := json.Unmarshal(input, &hi); err != nil {
				return "", fmt.Errorf("handoff: invalid input: %w", err)
			}

			if hi.TargetAgent == "" {
				return "", fmt.Errorf("handoff: target_agent is required")
			}
			if hi.Reason == "" {
				return "", fmt.Errorf("handoff: reason is required")
			}
			if hi.Context == "" {
				return "", fmt.Errorf("handoff: context is required")
			}

			alreadySet := true
			hh.once.Do(func() {
				alreadySet = false
				hh.result = &HandoffResult{
					TargetAgent: hi.TargetAgent,
					Reason:      hi.Reason,
					Context:     hi.Context,
				}
			})

			if alreadySet {
				return "Handoff already initiated — duplicate call ignored.", nil
			}

			return fmt.Sprintf("Handing off to %s.", hi.TargetAgent), nil
		},
	}
}

// Result returns the handoff data, or nil if not yet set.
func (hh *handoffHandler) Result() *HandoffResult { return hh.result }

// IsHandoff returns true when the handoff tool has been called.
func (hh *handoffHandler) IsHandoff() bool { return hh.result != nil }

type handoffInput struct {
	TargetAgent string `json:"target_agent"`
	Reason      string `json:"reason"`
	Context     string `json:"context"`
}
