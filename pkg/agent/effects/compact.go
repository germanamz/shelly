// Package effects provides reusable Effect implementations for the agent's
// ReAct loop.
package effects

import (
	"context"
	"errors"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

const summarizationPrompt = `You are a conversation summarizer. Summarize this conversation into the following structured format. Be precise — preserve exact file paths, error messages, and technical details.

## Goal
[The original user request — copy verbatim if short, otherwise paraphrase precisely]

## Completed Work
- [Each completed action with file paths and outcomes]

## Files Touched
- [path]: [created/modified/read — and what was done]

## Key Decisions
- [Decision]: [rationale]

## Errors & Blockers
- [Any errors encountered and resolution status]

## Current State
[What the agent is currently working on or just finished]

## Next Steps
1. [Specific next action with enough detail to execute without prior context]
2. [Following action]`

// CompactConfig holds parameters for the CompactEffect.
type CompactConfig struct {
	ContextWindow int     // Provider's context window size.
	Threshold     float64 // Fraction triggering compaction (e.g. 0.8).

	// AskFunc optionally asks the user a question on compaction failure.
	AskFunc func(ctx context.Context, text string, options []string) (string, error)
	// NotifyFunc optionally emits events (e.g. compaction notifications).
	NotifyFunc func(ctx context.Context, message string)
}

// CompactEffect summarizes the conversation when token usage approaches the
// context window limit. It runs only at PhaseBeforeComplete when Iteration > 0.
type CompactEffect struct {
	cfg CompactConfig
}

// NewCompactEffect creates a CompactEffect with the given configuration.
func NewCompactEffect(cfg CompactConfig) *CompactEffect {
	return &CompactEffect{cfg: cfg}
}

// Eval implements agent.Effect.
func (e *CompactEffect) Eval(ctx context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseBeforeComplete {
		return nil
	}

	if !e.shouldCompact(ic.Completer, ic.EstimatedTokens) {
		return nil
	}

	return e.summarize(ctx, ic)
}

// shouldCompact returns true when the context is estimated or measured to
// have reached or exceeded contextWindow * threshold.
func (e *CompactEffect) shouldCompact(completer modeladapter.Completer, estimatedTokens int) bool {
	if e.cfg.ContextWindow <= 0 || e.cfg.Threshold <= 0 {
		return false
	}

	limit := int(float64(e.cfg.ContextWindow) * e.cfg.Threshold)

	// Use pre-call estimate when available (enables firing on iteration 0).
	if estimatedTokens > 0 && estimatedTokens >= limit {
		return true
	}

	reporter, ok := completer.(modeladapter.UsageReporter)
	if !ok {
		return false
	}

	last, ok := reporter.UsageTracker().Last()
	if !ok {
		return false
	}

	return last.InputTokens >= limit
}

// summarize performs full conversation summarization, keeping the system prompt
// and a compacted summary. Tool-result trimming is handled separately by the
// standalone TrimToolResultsEffect which runs at PhaseAfterComplete before
// this effect's PhaseBeforeComplete on the next iteration.
func (e *CompactEffect) summarize(ctx context.Context, ic agent.IterationContext) error {
	transcript := renderMessages(ic.Chat.Messages())

	tempChat := chat.New(
		message.NewText("", role.System, summarizationPrompt),
		message.NewText("", role.User, transcript),
	)

	summary, err := ic.Completer.Complete(ctx, tempChat, nil)
	if err != nil {
		return e.handleCompactError(ctx, ic, err)
	}

	sysPrompt := ic.Chat.SystemPrompt()
	compactedMsg := fmt.Sprintf("[Conversation compacted — previous context summarized below.]\n\n%s\n\n[Continue executing the next steps listed above. Do not re-read files or repeat work already marked as completed.]", summary.TextContent())
	ic.Chat.Replace(
		message.NewText(ic.AgentName, role.System, sysPrompt),
		message.NewText("", role.User, compactedMsg),
	)

	if e.cfg.NotifyFunc != nil {
		e.cfg.NotifyFunc(ctx, "Context window compacted")
	}

	return nil
}

// handleCompactError asks the user what to do on compaction failure if AskFunc
// is available. Otherwise it returns nil to continue silently.
func (e *CompactEffect) handleCompactError(ctx context.Context, ic agent.IterationContext, compactErr error) error {
	// Propagate context errors (cancellation, deadline) regardless of AskFunc.
	if errors.Is(compactErr, context.Canceled) || errors.Is(compactErr, context.DeadlineExceeded) || ctx.Err() != nil {
		return ctx.Err()
	}

	if e.cfg.AskFunc == nil {
		return nil
	}

	answer, err := e.cfg.AskFunc(ctx, fmt.Sprintf("Context compaction failed: %v. What should I do?", compactErr), []string{"Continue without compaction", "Retry compaction"})
	if err != nil {
		// Propagate context errors from AskFunc; swallow other errors.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}

	if answer == "Retry compaction" {
		if retryErr := e.summarize(ctx, ic); retryErr != nil {
			if errors.Is(retryErr, context.Canceled) || errors.Is(retryErr, context.DeadlineExceeded) || ctx.Err() != nil {
				return ctx.Err()
			}
			if e.cfg.NotifyFunc != nil {
				e.cfg.NotifyFunc(ctx, fmt.Sprintf("Retry compaction failed: %v", retryErr))
			}
			return nil
		}
	}

	return nil
}
