package agent

import (
	"context"
	"unicode/utf8"
)

// DelegationEventKind identifies the type of delegation streaming event.
type DelegationEventKind string

const (
	// DelegationStatus represents a lifecycle transition (started, ended).
	DelegationStatus DelegationEventKind = "status"
	// DelegationProgress represents a partial output from a child agent.
	DelegationProgress DelegationEventKind = "progress"
	// DelegationResult represents the final result of a delegation.
	DelegationResult DelegationEventKind = "result"
)

// DelegationEvent carries information about delegation activity. It is used
// as the Data payload for "delegation_progress" events published through the
// EventNotifier / EventBus.
type DelegationEvent struct {
	Kind    DelegationEventKind `json:"kind"`
	Agent   string              `json:"agent"`
	Parent  string              `json:"parent"`
	Message string              `json:"message,omitempty"`
	Result  *delegateResult     `json:"result,omitempty"`
}

// maxProgressMessageRunes caps progress message text to keep events concise.
const maxProgressMessageRunes = 500

// delegationProgressFunc wraps a parent's EventFunc to additionally emit
// DelegationProgress events when the child agent adds assistant messages.
// The original EventFunc continues to receive all events unchanged.
func delegationProgressFunc(parent EventFunc, notifier EventNotifier, childName, parentName string) EventFunc {
	return func(ctx context.Context, kind string, data any) {
		// Forward the original event unchanged.
		if parent != nil {
			parent(ctx, kind, data)
		}

		// Emit delegation progress for assistant messages only.
		if kind != "message_added" || notifier == nil {
			return
		}
		mad, ok := data.(MessageAddedEventData)
		if !ok || mad.Role != "assistant" {
			return
		}
		text := mad.Message.TextContent()
		if text == "" {
			return
		}
		if utf8.RuneCountInString(text) > maxProgressMessageRunes {
			text = string([]rune(text)[:maxProgressMessageRunes]) + "..."
		}
		notifier(ctx, "delegation_progress", childName, DelegationEvent{
			Kind:    DelegationProgress,
			Agent:   childName,
			Parent:  parentName,
			Message: text,
		})
	}
}

// emitDelegationResult publishes a DelegationResult event via the notifier.
func emitDelegationResult(notifier EventNotifier, ctx context.Context, childName, parentName string, dr delegateResult) {
	if notifier == nil {
		return
	}
	notifier(ctx, "delegation_progress", childName, DelegationEvent{
		Kind:   DelegationResult,
		Agent:  childName,
		Parent: parentName,
		Result: &dr,
	})
}
