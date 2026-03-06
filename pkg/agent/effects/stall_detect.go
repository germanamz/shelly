package effects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

const (
	defaultStallWindow              = 6
	defaultStallSimilarityThreshold = 0.8
	stallFingerprintTruncate        = 200
)

// StallDetectConfig holds parameters for the StallDetectEffect.
type StallDetectConfig struct {
	Window              int     // Number of recent iterations to examine (default: 6).
	SimilarityThreshold float64 // Fraction of duplicate fingerprints to trigger (default: 0.8).
}

// StallDetectEffect detects when an agent is active but not progressing —
// calling different tools but getting the same errors, reading the same files,
// or producing no meaningful output. It complements LoopDetectEffect which
// catches exact consecutive repetition.
//
// It runs at PhaseBeforeComplete when Iteration > 0. On first trigger it
// injects a nudge message. If stall continues for another window, it returns
// ErrStallDetected.
type StallDetectEffect struct {
	cfg            StallDetectConfig
	nudged         bool
	nudgeIteration int // iteration at which the nudge was injected
}

// NewStallDetectEffect creates a StallDetectEffect with the given configuration,
// applying defaults for zero or invalid values.
func NewStallDetectEffect(cfg StallDetectConfig) *StallDetectEffect {
	if cfg.Window <= 0 {
		cfg.Window = defaultStallWindow
	}
	if cfg.SimilarityThreshold <= 0 || cfg.SimilarityThreshold > 1 {
		cfg.SimilarityThreshold = defaultStallSimilarityThreshold
	}
	return &StallDetectEffect{cfg: cfg}
}

// Reset implements agent.Resetter.
func (e *StallDetectEffect) Reset() {
	e.nudged = false
	e.nudgeIteration = 0
}

// Eval implements agent.Effect.
func (e *StallDetectEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseBeforeComplete || ic.Iteration == 0 {
		return nil
	}

	fps := e.collectFingerprints(ic)
	if len(fps) < e.cfg.Window {
		return nil
	}

	// Count how many fingerprints in the window are duplicates of others.
	seen := make(map[string]struct{})
	duplicates := 0
	for _, fp := range fps {
		if _, ok := seen[fp]; ok {
			duplicates++
		} else {
			seen[fp] = struct{}{}
		}
	}

	ratio := float64(duplicates) / float64(len(fps))
	if ratio < e.cfg.SimilarityThreshold {
		return nil
	}

	// Escalation: first time → nudge, second time (after another window) → error.
	if e.nudged && ic.Iteration >= e.nudgeIteration+e.cfg.Window {
		return agent.ErrStallDetected
	}

	if !e.nudged {
		e.nudged = true
		e.nudgeIteration = ic.Iteration
		ic.Chat.Append(message.NewText("", role.User,
			fmt.Sprintf(
				"You appear stalled. The last %d iterations produced similar results. "+
					"Step back and reconsider your approach.",
				len(fps),
			),
		))
	}

	return nil
}

// collectFingerprints scans the chat and builds fingerprints for the most
// recent tool-call/tool-result pairs within the configured window. A
// fingerprint is: toolName + isError + hash(first N chars of result).
func (e *StallDetectEffect) collectFingerprints(ic agent.IterationContext) []string {
	msgs := ic.Chat.Messages()

	// First pass: index all tool-call IDs → tool name.
	calls := make(map[string]string) // callID → toolName
	for _, msg := range msgs {
		if msg.Role != role.Assistant {
			continue
		}
		for _, p := range msg.Parts {
			if tc, ok := p.(content.ToolCall); ok {
				calls[tc.ID] = tc.Name
			}
		}
	}

	// Second pass: collect tool results from the end, up to Window entries.
	type resultEntry struct {
		toolName string
		isError  bool
		content  string
	}
	var results []resultEntry

	for i := len(msgs) - 1; i >= 0 && len(results) < e.cfg.Window; i-- {
		msg := msgs[i]
		if msg.Role != role.Tool {
			continue
		}
		for j := len(msg.Parts) - 1; j >= 0 && len(results) < e.cfg.Window; j-- {
			tr, ok := msg.Parts[j].(content.ToolResult)
			if !ok {
				continue
			}
			toolName, found := calls[tr.ToolCallID]
			if !found {
				continue
			}
			results = append(results, resultEntry{
				toolName: toolName,
				isError:  tr.IsError,
				content:  tr.Content,
			})
		}
	}

	// Convert to fingerprint strings.
	fps := make([]string, len(results))
	for i, r := range results {
		truncated := r.content
		if len(truncated) > stallFingerprintTruncate {
			truncated = truncated[:stallFingerprintTruncate]
		}
		h := sha256.Sum256([]byte(truncated))
		errFlag := "0"
		if r.isError {
			errFlag = "1"
		}
		fps[i] = r.toolName + "\x00" + errFlag + "\x00" + hex.EncodeToString(h[:8])
	}

	return fps
}
