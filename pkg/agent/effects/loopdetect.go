package effects

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

const (
	defaultLoopThreshold  = 3
	defaultLoopWindowSize = 10
)

// LoopDetectConfig holds parameters for the LoopDetectEffect.
type LoopDetectConfig struct {
	Threshold  int // Consecutive identical calls before intervention (default: 3).
	WindowSize int // Sliding window of tool calls to track (default: 10).
}

// LoopDetectEffect detects when an agent is stuck calling the same tool with
// the same arguments repeatedly and injects an intervention message. It runs
// only at PhaseBeforeComplete when Iteration > 0.
type LoopDetectEffect struct {
	cfg LoopDetectConfig
}

// NewLoopDetectEffect creates a LoopDetectEffect with the given configuration,
// applying defaults for zero or negative values.
func NewLoopDetectEffect(cfg LoopDetectConfig) *LoopDetectEffect {
	if cfg.Threshold <= 0 {
		cfg.Threshold = defaultLoopThreshold
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = defaultLoopWindowSize
	}

	return &LoopDetectEffect{cfg: cfg}
}

// Eval implements agent.Effect.
func (e *LoopDetectEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseBeforeComplete || ic.Iteration == 0 {
		return nil
	}

	toolName, count := e.detectLoop(ic)
	if count >= e.cfg.Threshold {
		ic.Chat.Append(message.NewText("", role.User,
			fmt.Sprintf("You have called %s with the same arguments %d times. This is not making progress. Try a different approach or tool.", toolName, count),
		))
	}

	return nil
}

// detectLoop scans the last WindowSize assistant-role messages from the end of
// the chat for ToolCall parts. It returns the tool name and the count of
// consecutive identical calls at the tail of the window. The key used for
// comparison is toolName + "\x00" + arguments.
func (e *LoopDetectEffect) detectLoop(ic agent.IterationContext) (string, int) {
	msgs := ic.Chat.Messages()

	// Collect tool call keys from assistant messages, scanning from the end,
	// up to WindowSize entries.
	var keys []string

	for i := len(msgs) - 1; i >= 0 && len(keys) < e.cfg.WindowSize; i-- {
		if msgs[i].Role != role.Assistant {
			continue
		}

		for _, p := range msgs[i].Parts {
			tc, ok := p.(content.ToolCall)
			if !ok {
				continue
			}

			keys = append(keys, tc.Name+"\x00"+tc.Arguments)
		}
	}

	if len(keys) == 0 {
		return "", 0
	}

	// keys[0] is the most recent call. Count consecutive identical entries.
	latest := keys[0]
	count := 0

	for _, k := range keys {
		if k != latest {
			break
		}
		count++
	}

	// Extract just the tool name from the key for the intervention message.
	toolName := latest
	for i, b := range latest {
		if b == '\x00' {
			toolName = latest[:i]
			break
		}
	}

	return toolName, count
}
