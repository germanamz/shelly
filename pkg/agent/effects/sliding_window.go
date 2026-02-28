package effects

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

const (
	defaultRecentZone     = 10
	defaultMediumZone     = 10
	defaultSlidingTrimLen = 200

	// incrementalSummarizationPrompt asks the LLM to update a running summary
	// with new information from old messages being evicted.
	incrementalSummarizationPrompt = `You are a conversation summarizer. You have an existing summary and new messages that are about to be evicted from context. Update the summary to incorporate the new information while keeping it concise. Preserve exact file paths, error messages, and decisions.

Existing summary:
%s

New messages to incorporate:
%s

Output ONLY the updated summary in this format:

## Goal
[The original user request]

## Completed Work
- [Each completed action with file paths and outcomes]

## Files Touched
- [path]: [what was done]

## Key Decisions
- [Decision]: [rationale]

## Errors & Blockers
- [Any errors encountered and resolution status]`
)

// SlidingWindowConfig holds parameters for the SlidingWindowEffect.
type SlidingWindowConfig struct {
	ContextWindow int     // Provider's context window size.
	Threshold     float64 // Fraction triggering window management (e.g. 0.7).

	// RecentZone is the number of recent messages kept at full fidelity.
	RecentZone int
	// MediumZone is the number of messages (before recent) where tool results
	// are trimmed but text is preserved.
	MediumZone int
	// TrimLength is the max tool result length in the medium zone.
	TrimLength int

	// NotifyFunc optionally emits events.
	NotifyFunc func(ctx context.Context, message string)
}

// SlidingWindowEffect implements a three-zone context management strategy:
//   - Zone 1 (recent): Last N messages — full fidelity
//   - Zone 2 (medium): Messages N to N+M — tool results trimmed, text preserved
//   - Zone 3 (old): Messages older than N+M — incrementally summarized into a
//     running summary block
//
// It runs at PhaseBeforeComplete when Iteration > 0.
type SlidingWindowEffect struct {
	cfg            SlidingWindowConfig
	mu             sync.Mutex
	runningSummary string // Accumulated summary of evicted messages.
}

// NewSlidingWindowEffect creates a SlidingWindowEffect with the given
// configuration, applying defaults for zero values.
func NewSlidingWindowEffect(cfg SlidingWindowConfig) *SlidingWindowEffect {
	if cfg.RecentZone <= 0 {
		cfg.RecentZone = defaultRecentZone
	}
	if cfg.MediumZone <= 0 {
		cfg.MediumZone = defaultMediumZone
	}
	if cfg.TrimLength <= 0 {
		cfg.TrimLength = defaultSlidingTrimLen
	}

	return &SlidingWindowEffect{cfg: cfg}
}

// Reset implements agent.Resetter.
func (e *SlidingWindowEffect) Reset() {
	e.mu.Lock()
	e.runningSummary = ""
	e.mu.Unlock()
}

// Eval implements agent.Effect.
func (e *SlidingWindowEffect) Eval(ctx context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseBeforeComplete || ic.Iteration == 0 {
		return nil
	}

	if !e.shouldManage(ic.Completer) {
		return nil
	}

	return e.manage(ctx, ic)
}

// shouldManage returns true when token usage exceeds the configured threshold.
func (e *SlidingWindowEffect) shouldManage(completer modeladapter.Completer) bool {
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

// manage applies the three-zone sliding window to the chat.
func (e *SlidingWindowEffect) manage(ctx context.Context, ic agent.IterationContext) error {
	msgs := ic.Chat.Messages()

	// Find non-system messages (system prompt is always index 0).
	startIdx := len(msgs)
	for i, m := range msgs {
		if m.Role != role.System {
			startIdx = i
			break
		}
	}

	// Skip if there's an existing summary message (starts with [Context summary]).
	if startIdx < len(msgs) && strings.HasPrefix(msgs[startIdx].TextContent(), "[Context summary") {
		startIdx++
	}

	nonSystem := msgs[startIdx:]
	total := len(nonSystem)

	// Not enough messages to create zones.
	if total <= e.cfg.RecentZone {
		return nil
	}

	recentStart := total - e.cfg.RecentZone
	mediumStart := max(recentStart-e.cfg.MediumZone, 0)

	// Zone 3: old messages (0 to mediumStart) — summarize and remove.
	oldMsgs := nonSystem[:mediumStart]
	// Zone 2: medium messages (mediumStart to recentStart) — trim tool results.
	mediumMsgs := nonSystem[mediumStart:recentStart]
	// Zone 1: recent messages (recentStart to end) — full fidelity.
	recentMsgs := nonSystem[recentStart:]

	// Summarize old messages incrementally.
	// The LLM call is done outside the mutex to avoid blocking concurrent agents.
	summarized := true
	if len(oldMsgs) > 0 {
		transcript := renderMessages(oldMsgs)

		// Read current summary under lock.
		e.mu.Lock()
		existing := e.runningSummary
		e.mu.Unlock()

		// Perform the blocking LLM call without holding the lock.
		newSummary, err := e.computeSummary(ctx, ic, existing, transcript)
		if err != nil {
			// Non-fatal: retain old messages instead of dropping them.
			summarized = false
			if e.cfg.NotifyFunc != nil {
				e.cfg.NotifyFunc(ctx, "Sliding window summarization failed, retaining old messages")
			}
		} else {
			// Write the updated summary under lock.
			e.mu.Lock()
			e.runningSummary = newSummary
			e.mu.Unlock()
		}
	}

	// Reconstruct the chat: system prompt + optional summary + medium + recent.
	e.mu.Lock()
	currentSummary := e.runningSummary
	e.mu.Unlock()

	var newMsgs []message.Message
	newMsgs = append(newMsgs, msgs[:startIdx]...) // System prompt(s).

	if !summarized {
		// Summarization failed — keep old messages to avoid data loss.
		// Skip medium-zone trimming as well since zone boundaries are invalid.
		newMsgs = append(newMsgs, oldMsgs...)
	} else {
		// Trim tool results in medium zone only when summarization succeeded,
		// since zone boundaries are only valid after old messages are evicted.
		for i := range mediumMsgs {
			if mediumMsgs[i].Role != role.Tool {
				continue
			}

			if _, ok := mediumMsgs[i].GetMeta("sw_trimmed"); ok {
				continue
			}

			trimmed := false

			for j, p := range mediumMsgs[i].Parts {
				tr, ok := p.(content.ToolResult)
				if !ok || tr.IsError || utf8.RuneCountInString(tr.Content) <= e.cfg.TrimLength {
					continue
				}

				tr.Content = string([]rune(tr.Content)[:e.cfg.TrimLength]) + "… [trimmed]"
				mediumMsgs[i].Parts[j] = tr
				trimmed = true
			}

			if trimmed {
				mediumMsgs[i].SetMeta("sw_trimmed", true)
			}
		}

		if currentSummary != "" {
			summaryMsg := fmt.Sprintf("[Context summary — earlier conversation condensed below.]\n\n%s", currentSummary)
			newMsgs = append(newMsgs, message.NewText("", role.User, summaryMsg))
		}
	}

	newMsgs = append(newMsgs, mediumMsgs...)
	newMsgs = append(newMsgs, recentMsgs...)

	ic.Chat.Replace(newMsgs...)

	if e.cfg.NotifyFunc != nil {
		e.cfg.NotifyFunc(ctx, "Context window managed via sliding window")
	}

	return nil
}

// computeSummary asks the LLM to incrementally update the running summary with
// information from newly evicted messages. It returns the new summary text
// without modifying any struct state, so callers can invoke it without holding
// the mutex.
func (e *SlidingWindowEffect) computeSummary(ctx context.Context, ic agent.IterationContext, existing, newTranscript string) (string, error) {
	if existing == "" {
		existing = "(no prior summary)"
	}

	prompt := fmt.Sprintf(incrementalSummarizationPrompt, existing, newTranscript)

	tempChat := chat.New(
		message.NewText("", role.System, "You are a conversation summarizer."),
		message.NewText("", role.User, prompt),
	)

	reply, err := ic.Completer.Complete(ctx, tempChat, nil)
	if err != nil {
		return "", err
	}

	return reply.TextContent(), nil
}

// renderMessages converts a slice of messages into a compact text transcript.
func renderMessages(msgs []message.Message) string {
	var b strings.Builder

	for _, m := range msgs {
		if m.Role == role.System {
			continue
		}

		for _, p := range m.Parts {
			switch v := p.(type) {
			case content.Text:
				fmt.Fprintf(&b, "[%s] %s\n", m.Role, v.Text)
			case content.ToolCall:
				args := truncate(v.Arguments, maxToolArgs)
				fmt.Fprintf(&b, "[%s] Called tool %s(%s)\n", m.Role, v.Name, args)
			case content.ToolResult:
				body := truncate(v.Content, maxToolResult)
				if v.IsError {
					fmt.Fprintf(&b, "[tool error] %s\n", body)
				} else {
					fmt.Fprintf(&b, "[tool result] %s\n", body)
				}
			}
		}
	}

	return b.String()
}
