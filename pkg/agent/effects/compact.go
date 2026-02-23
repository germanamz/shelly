// Package effects provides reusable Effect implementations for the agent's
// ReAct loop.
package effects

import (
	"context"
	"fmt"
	"strings"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

const summarizationPrompt = `You are a conversation summarizer. Create a concise but thorough summary including:
1. The original user request or goal
2. What has been accomplished (key actions, files modified, commands run)
3. What is currently in progress
4. Important decisions, constraints, or context discovered
5. Pending tasks or planned next steps`

const (
	maxToolArgs   = 200
	maxToolResult = 500
)

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
	if ic.Phase != agent.PhaseBeforeComplete || ic.Iteration == 0 {
		return nil
	}

	if !e.shouldCompact(ic.Completer) {
		return nil
	}

	return e.compact(ctx, ic)
}

// shouldCompact returns true when the last LLM call's input tokens have
// reached or exceeded contextWindow * threshold.
func (e *CompactEffect) shouldCompact(completer modeladapter.Completer) bool {
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

// compact summarizes the conversation and replaces the chat with the summary.
func (e *CompactEffect) compact(ctx context.Context, ic agent.IterationContext) error {
	transcript := renderConversation(ic.Chat)

	tempChat := chat.New(
		message.NewText("", role.System, summarizationPrompt),
		message.NewText("", role.User, transcript),
	)

	summary, err := ic.Completer.Complete(ctx, tempChat, nil)
	if err != nil {
		return e.handleCompactError(ctx, ic, err)
	}

	sysPrompt := ic.Chat.SystemPrompt()
	compactedMsg := fmt.Sprintf("[Conversation compacted — previous context summarized below.]\n\n%s\n\n[Continue from where we left off.]", summary.TextContent())
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
	if e.cfg.AskFunc == nil {
		return nil
	}

	answer, err := e.cfg.AskFunc(ctx, fmt.Sprintf("Context compaction failed: %v. What should I do?", compactErr), []string{"Continue without compaction", "Retry compaction"})
	if err != nil {
		return nil
	}

	if answer == "Retry compaction" {
		transcript := renderConversation(ic.Chat)
		tempChat := chat.New(
			message.NewText("", role.System, summarizationPrompt),
			message.NewText("", role.User, transcript),
		)

		summary, retryErr := ic.Completer.Complete(ctx, tempChat, nil)
		if retryErr != nil {
			return nil // Give up silently on second failure.
		}

		sysPrompt := ic.Chat.SystemPrompt()
		compactedMsg := fmt.Sprintf("[Conversation compacted — previous context summarized below.]\n\n%s\n\n[Continue from where we left off.]", summary.TextContent())
		ic.Chat.Replace(
			message.NewText(ic.AgentName, role.System, sysPrompt),
			message.NewText("", role.User, compactedMsg),
		)

		if e.cfg.NotifyFunc != nil {
			e.cfg.NotifyFunc(ctx, "Context window compacted")
		}
	}

	return nil
}

// renderConversation converts chat messages into a compact text transcript,
// skipping system messages.
func renderConversation(c *chat.Chat) string {
	var b strings.Builder

	c.Each(func(_ int, m message.Message) bool {
		if m.Role == role.System {
			return true
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

		return true
	})

	return b.String()
}

// truncate returns s truncated to maxLen characters with "..." appended if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "\u2026"
}
