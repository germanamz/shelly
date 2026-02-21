package reactor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

// ErrMaxRounds is returned when a coordinator exceeds its round limit.
var ErrMaxRounds = errors.New("reactor: max rounds reached")

// SequenceCoordinator runs each member exactly once in order, then signals done.
type SequenceCoordinator struct {
	next int
}

// NewSequence creates a SequenceCoordinator.
func NewSequence() *SequenceCoordinator {
	return &SequenceCoordinator{}
}

func (s *SequenceCoordinator) Next(_ context.Context, _ *chat.Chat, members []TeamMember) (Selection, error) {
	if s.next >= len(members) {
		return Selection{Done: true}, nil
	}

	idx := s.next
	s.next++

	return Selection{Members: []int{idx}}, nil
}

// LoopCoordinator cycles through members in round-robin fashion with an optional
// max rounds limit. Zero maxRounds means unlimited.
type LoopCoordinator struct {
	maxRounds int
	step      int
}

// NewLoop creates a LoopCoordinator.
func NewLoop(maxRounds int) *LoopCoordinator {
	return &LoopCoordinator{maxRounds: maxRounds}
}

func (l *LoopCoordinator) Next(_ context.Context, _ *chat.Chat, members []TeamMember) (Selection, error) {
	if l.maxRounds > 0 && l.step >= l.maxRounds*len(members) {
		return Selection{}, ErrMaxRounds
	}

	idx := l.step % len(members)
	l.step++

	return Selection{Members: []int{idx}}, nil
}

// RoundRobinUntilCoordinator cycles through members until a predicate returns
// true or maxRounds is exceeded. Zero maxRounds means no round limit.
type RoundRobinUntilCoordinator struct {
	maxRounds int
	predicate func(*chat.Chat) bool
	step      int
}

// NewRoundRobinUntil creates a RoundRobinUntilCoordinator.
func NewRoundRobinUntil(maxRounds int, predicate func(*chat.Chat) bool) *RoundRobinUntilCoordinator {
	return &RoundRobinUntilCoordinator{
		maxRounds: maxRounds,
		predicate: predicate,
	}
}

func (r *RoundRobinUntilCoordinator) Next(_ context.Context, shared *chat.Chat, members []TeamMember) (Selection, error) {
	if r.predicate(shared) {
		return Selection{Done: true}, nil
	}

	if r.maxRounds > 0 && r.step >= r.maxRounds*len(members) {
		return Selection{}, ErrMaxRounds
	}

	idx := r.step % len(members)
	r.step++

	return Selection{Members: []int{idx}}, nil
}

// RoleRoundRobin cycles through an ordered list of roles. On each step it
// selects all members matching the current role, enabling concurrent execution
// when multiple members share the same role.
type RoleRoundRobin struct {
	maxRounds int
	order     []TeamRole
	step      int
}

// NewRoleRoundRobin creates a RoleRoundRobin coordinator. order defines the
// sequence of roles to cycle through. maxRounds limits the total number of full
// cycles through the role order (zero means unlimited).
func NewRoleRoundRobin(maxRounds int, order ...TeamRole) *RoleRoundRobin {
	return &RoleRoundRobin{
		maxRounds: maxRounds,
		order:     order,
	}
}

func (rr *RoleRoundRobin) Next(_ context.Context, _ *chat.Chat, members []TeamMember) (Selection, error) {
	if len(rr.order) == 0 {
		return Selection{Done: true}, nil
	}

	// Guard against infinite skipping when no roles match any member.
	maxSkips := len(rr.order)

	for range maxSkips {
		if rr.maxRounds > 0 && rr.step/len(rr.order) >= rr.maxRounds {
			return Selection{}, ErrMaxRounds
		}

		targetRole := rr.order[rr.step%len(rr.order)]
		rr.step++

		var indices []int
		for i, m := range members {
			if m.Role == targetRole {
				indices = append(indices, i)
			}
		}

		if len(indices) > 0 {
			return Selection{Members: indices}, nil
		}
	}

	return Selection{Done: true}, nil
}

// MemberDescriptor provides optional metadata about a team member for the
// LLM coordinator's system prompt.
type MemberDescriptor struct {
	Description string
}

// LLMCoordinator uses an LLM to decide which agent(s) should act next. It
// maintains its own private chat for reasoning and includes a sliding window
// of recent shared messages as context for the LLM.
type LLMCoordinator struct {
	completer   modeladapter.Completer
	maxRounds   int
	step        int
	chat        *chat.Chat
	descriptors []MemberDescriptor
	windowSize  int
}

// NewLLMCoordinator creates an LLMCoordinator. descriptors is optional and
// provides human-readable descriptions for each team member.
func NewLLMCoordinator(completer modeladapter.Completer, maxRounds int, descriptors ...MemberDescriptor) *LLMCoordinator {
	return &LLMCoordinator{
		completer:   completer,
		maxRounds:   maxRounds,
		chat:        chat.New(),
		descriptors: descriptors,
		windowSize:  20,
	}
}

// llmDecision is the JSON structure expected from the LLM.
type llmDecision struct {
	Members []int `json:"members"`
	Done    bool  `json:"done"`
}

func (lc *LLMCoordinator) Next(ctx context.Context, shared *chat.Chat, members []TeamMember) (Selection, error) {
	if lc.maxRounds > 0 && lc.step >= lc.maxRounds*len(members) {
		return Selection{}, ErrMaxRounds
	}

	// Build system prompt on first call.
	if lc.chat.Len() == 0 {
		lc.chat.Append(message.NewText("", role.System, lc.buildSystemPrompt(members)))
	}

	// Add recent shared messages as context.
	lc.appendContext(shared)

	// First attempt.
	decision, err := lc.complete(ctx, members)
	if err == nil {
		lc.step++
		return Selection(decision), nil
	}

	// On parse failure, provide feedback and retry once.
	lc.chat.Append(message.NewText("", role.User, fmt.Sprintf("Invalid response: %v. Please respond with ONLY a JSON object.", err)))

	decision, err = lc.complete(ctx, members)
	if err != nil {
		return Selection{}, fmt.Errorf("reactor: llm coordinator: %w", err)
	}

	lc.step++

	return Selection(decision), nil
}

// complete calls the LLM and parses its JSON response into an llmDecision.
func (lc *LLMCoordinator) complete(ctx context.Context, members []TeamMember) (llmDecision, error) {
	reply, err := lc.completer.Complete(ctx, lc.chat)
	if err != nil {
		return llmDecision{}, err
	}

	lc.chat.Append(reply)

	text := stripCodeFences(reply.TextContent())

	var decision llmDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		return llmDecision{}, fmt.Errorf("invalid JSON: %w", err)
	}

	if !decision.Done {
		for _, idx := range decision.Members {
			if idx < 0 || idx >= len(members) {
				return llmDecision{}, fmt.Errorf("member index %d out of range [0, %d)", idx, len(members))
			}
		}
	}

	return decision, nil
}

// buildSystemPrompt constructs the coordinator's system prompt from the member list.
func (lc *LLMCoordinator) buildSystemPrompt(members []TeamMember) string {
	var b strings.Builder

	b.WriteString("You are a team coordinator. Based on the conversation, decide which team member(s) should act next.\n\nTeam members:\n")

	for i, m := range members {
		desc := ""
		if i < len(lc.descriptors) && lc.descriptors[i].Description != "" {
			desc = " — " + lc.descriptors[i].Description
		}

		fmt.Fprintf(&b, "- [%d] %q (role: %s)%s\n", i, m.Agent.AgentName(), m.Role, desc)
	}

	b.WriteString("\nRespond with ONLY a JSON object (no markdown, no explanation):\n")
	b.WriteString(`{"members": [0], "done": false}`)
	b.WriteString("\n\n- \"members\": array of member indices to run next (can be multiple for concurrent execution)\n")
	b.WriteString("- \"done\": true when the overall task is complete\n")

	return b.String()
}

// appendContext takes the last windowSize messages from the shared chat and
// appends them as a user message summarizing recent activity.
func (lc *LLMCoordinator) appendContext(shared *chat.Chat) {
	total := shared.Len()
	if total == 0 {
		return
	}

	start := max(total-lc.windowSize, 0)

	msgs := shared.Since(start)
	if len(msgs) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("Recent conversation:\n")

	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s] %s: %s\n", m.Role, m.Sender, m.TextContent())
	}

	lc.chat.Append(message.NewText("", role.User, b.String()))
}

// stripCodeFences removes markdown code fences from the LLM response.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "```") {
		// Remove opening fence (with optional language tag).
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}

		// Remove closing fence.
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}

		s = strings.TrimSpace(s)
	}

	return s
}
