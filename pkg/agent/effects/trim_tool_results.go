package effects

import (
	"context"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

const (
	defaultMaxResultLength = 500
	defaultPreserveRecent  = 4
	trimmedMetaKey         = "trimmed"
	trimSuffix             = "… [trimmed]"
)

// TrimToolResultsConfig holds parameters for the TrimToolResultsEffect.
type TrimToolResultsConfig struct {
	MaxResultLength int // Max chars for tool result content (default: 500).
	PreserveRecent  int // Keep last N tool-role messages untrimmed (default: 4).
}

// TrimToolResultsEffect trims old tool result content to save tokens. It runs
// at PhaseAfterComplete (after the LLM has responded, before the next
// iteration) and preserves the most recent tool-role messages untrimmed.
type TrimToolResultsEffect struct {
	cfg TrimToolResultsConfig
}

// NewTrimToolResultsEffect creates a TrimToolResultsEffect with the given
// configuration, applying defaults for zero values.
func NewTrimToolResultsEffect(cfg TrimToolResultsConfig) *TrimToolResultsEffect {
	if cfg.MaxResultLength <= 0 {
		cfg.MaxResultLength = defaultMaxResultLength
	}
	if cfg.PreserveRecent < 0 {
		cfg.PreserveRecent = defaultPreserveRecent
	}

	return &TrimToolResultsEffect{cfg: cfg}
}

// Eval implements agent.Effect.
func (e *TrimToolResultsEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseAfterComplete || ic.Iteration == 0 {
		return nil
	}

	msgs := ic.Chat.Messages()

	// Find indices of all tool-role messages so we can identify which to preserve.
	var toolIndices []int
	for i, m := range msgs {
		if m.Role == role.Tool {
			toolIndices = append(toolIndices, i)
		}
	}

	// Build a set of indices that should be preserved (the last N tool messages).
	preserveSet := make(map[int]bool)
	preserveStart := max(len(toolIndices)-e.cfg.PreserveRecent, 0)

	for _, idx := range toolIndices[preserveStart:] {
		preserveSet[idx] = true
	}

	modified := false

	for i := range msgs {
		if msgs[i].Role != role.Tool {
			continue
		}

		if preserveSet[i] {
			continue
		}

		if _, ok := msgs[i].GetMeta(trimmedMetaKey); ok {
			continue
		}

		if e.trimMessage(&msgs[i]) {
			modified = true
		}
	}

	if modified {
		ic.Chat.Replace(msgs...)
	}

	return nil
}

// trimMessage trims ToolResult parts in a message that exceed MaxResultLength.
// It returns true if any part was modified.
func (e *TrimToolResultsEffect) trimMessage(m *message.Message) bool {
	trimmed := false

	for j, p := range m.Parts {
		tr, ok := p.(content.ToolResult)
		if !ok || tr.IsError || len(tr.Content) <= e.cfg.MaxResultLength {
			continue
		}

		tr.Content = tr.Content[:e.cfg.MaxResultLength] + trimSuffix
		m.Parts[j] = tr
		trimmed = true
	}

	if trimmed {
		m.SetMeta(trimmedMetaKey, true)
	}

	return trimmed
}
