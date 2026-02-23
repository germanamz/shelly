package agent

import (
	"context"
	"fmt"
	"strings"

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

// shouldCompact returns true when the last LLM call's input tokens have
// reached or exceeded contextWindow * threshold.
func (a *Agent) shouldCompact() bool {
	if a.options.ContextWindow <= 0 || a.options.ContextThreshold <= 0 {
		return false
	}

	reporter, ok := a.completer.(modeladapter.UsageReporter)
	if !ok {
		return false
	}

	last, ok := reporter.UsageTracker().Last()
	if !ok {
		return false
	}

	limit := int(float64(a.options.ContextWindow) * a.options.ContextThreshold)

	return last.InputTokens >= limit
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

// compact summarizes the conversation and replaces the chat with the summary.
func (a *Agent) compact(ctx context.Context) error {
	transcript := renderConversation(a.chat)

	// Build a temporary chat for summarization.
	tempChat := chat.New(
		message.NewText("", role.System, summarizationPrompt),
		message.NewText("", role.User, transcript),
	)

	// Call the completer with no tools — pure summarization.
	summary, err := a.completer.Complete(ctx, tempChat, nil)
	if err != nil {
		return a.handleCompactError(ctx, err)
	}

	// Preserve the original system prompt and replace the rest.
	sysPrompt := a.chat.SystemPrompt()
	compactedMsg := fmt.Sprintf("[Conversation compacted — previous context summarized below.]\n\n%s\n\n[Continue from where we left off.]", summary.TextContent())
	a.chat.Replace(
		message.NewText(a.name, role.System, sysPrompt),
		message.NewText("", role.User, compactedMsg),
	)

	if a.options.NotifyFunc != nil {
		a.options.NotifyFunc(ctx, "Context window compacted")
	}

	return nil
}

// handleCompactError asks the user what to do on compaction failure if AskFunc
// is available. Otherwise it returns nil to continue silently.
func (a *Agent) handleCompactError(ctx context.Context, compactErr error) error {
	if a.options.AskFunc == nil {
		return nil
	}

	answer, err := a.options.AskFunc(ctx, fmt.Sprintf("Context compaction failed: %v. What should I do?", compactErr), []string{"Continue without compaction", "Retry compaction"})
	if err != nil {
		return nil
	}

	if answer == "Retry compaction" {
		transcript := renderConversation(a.chat)
		tempChat := chat.New(
			message.NewText("", role.System, summarizationPrompt),
			message.NewText("", role.User, transcript),
		)

		summary, retryErr := a.completer.Complete(ctx, tempChat, nil)
		if retryErr != nil {
			return nil // Give up silently on second failure.
		}

		sysPrompt := a.chat.SystemPrompt()
		compactedMsg := fmt.Sprintf("[Conversation compacted — previous context summarized below.]\n\n%s\n\n[Continue from where we left off.]", summary.TextContent())
		a.chat.Replace(
			message.NewText(a.name, role.System, sysPrompt),
			message.NewText("", role.User, compactedMsg),
		)

		if a.options.NotifyFunc != nil {
			a.options.NotifyFunc(ctx, "Context window compacted")
		}
	}

	return nil
}

// truncate returns s truncated to maxLen characters with "…" appended if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "…"
}
