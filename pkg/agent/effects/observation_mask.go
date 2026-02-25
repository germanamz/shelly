package effects

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

const (
	defaultObsMaskRecentWindow = 10
	defaultObsMaskThreshold    = 0.6
	obsMaskMetaKey             = "obs_masked"
	obsMaskMaxPreview          = 80
)

// ObservationMaskConfig holds parameters for the ObservationMaskEffect.
type ObservationMaskConfig struct {
	ContextWindow int     // Provider's context window size.
	Threshold     float64 // Fraction triggering masking (e.g. 0.6).
	RecentWindow  int     // Messages to keep at full fidelity (default: 10).
}

// ObservationMaskEffect replaces old tool result content with brief placeholders
// while keeping reasoning (assistant text) and actions (tool calls) intact. It
// runs at PhaseBeforeComplete when Iteration > 0, acting as a lightweight first
// tier before heavier compaction effects.
type ObservationMaskEffect struct {
	cfg ObservationMaskConfig
}

// NewObservationMaskEffect creates an ObservationMaskEffect with the given
// configuration, applying defaults for zero values.
func NewObservationMaskEffect(cfg ObservationMaskConfig) *ObservationMaskEffect {
	if cfg.RecentWindow <= 0 {
		cfg.RecentWindow = defaultObsMaskRecentWindow
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = defaultObsMaskThreshold
	}

	return &ObservationMaskEffect{cfg: cfg}
}

// Eval implements agent.Effect.
func (e *ObservationMaskEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseBeforeComplete || ic.Iteration == 0 {
		return nil
	}

	if !e.shouldMask(ic.Completer) {
		return nil
	}

	e.mask(ic)

	return nil
}

// shouldMask returns true when token usage exceeds the configured threshold.
func (e *ObservationMaskEffect) shouldMask(completer modeladapter.Completer) bool {
	if e.cfg.ContextWindow <= 0 || e.cfg.Threshold <= 0 {
		return false
	}

	reporter, ok := completer.(modeladapter.UsageReporter)
	if !ok {
		return false
	}

	last, ok := reporter.UsageTracker().Last()
	if !ok {
		return false
	}

	limit := int(float64(e.cfg.ContextWindow) * e.cfg.Threshold)

	return last.InputTokens >= limit
}

// mask replaces tool result content in older messages with brief placeholders.
func (e *ObservationMaskEffect) mask(ic agent.IterationContext) {
	msgs := ic.Chat.Messages()

	// Determine the boundary: messages before this index are eligible for masking.
	boundary := len(msgs) - e.cfg.RecentWindow
	if boundary <= 0 {
		return
	}

	modified := false

	for i := range msgs {
		if i >= boundary {
			break
		}

		if msgs[i].Role != role.Tool {
			continue
		}

		// Skip already-masked messages.
		if _, ok := msgs[i].GetMeta(obsMaskMetaKey); ok {
			continue
		}

		masked := false

		for j, p := range msgs[i].Parts {
			tr, ok := p.(content.ToolResult)
			if !ok || tr.IsError {
				continue
			}

			preview := truncate(tr.Content, obsMaskMaxPreview)

			tr.Content = fmt.Sprintf("[tool result for %s: %s]", toolNameForResult(msgs, i, tr.ToolCallID), preview)
			msgs[i].Parts[j] = tr
			masked = true
		}

		if masked {
			msgs[i].SetMeta(obsMaskMetaKey, true)
			modified = true
		}
	}

	if modified {
		ic.Chat.Replace(msgs...)
	}
}

// toolNameForResult finds the tool name for a given tool call ID by scanning
// preceding assistant messages.
func toolNameForResult(msgs []message.Message, resultIdx int, callID string) string {
	for i := resultIdx - 1; i >= 0; i-- {
		if msgs[i].Role != role.Assistant {
			continue
		}

		for _, p := range msgs[i].Parts {
			tc, ok := p.(content.ToolCall)
			if !ok {
				continue
			}

			if tc.ID == callID {
				return tc.Name
			}
		}
	}

	return "unknown"
}
