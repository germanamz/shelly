// Package reactor orchestrates one or more agents over a shared conversation.
// Each agent maintains its own private chat; the reactor bridges messages
// between the shared chat and each agent's private chat using per-agent cursors.
// Agents are organized as team members with roles, and a Coordinator decides
// which members to run next — including concurrent execution when multiple
// members are selected.
package reactor

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/germanamz/shelly/pkg/agents"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// Compile-time interface checks.
var (
	_ agents.Agent = (*Reactor)(nil)
	_ NamedAgent   = (*Reactor)(nil)
)

// ErrNoMembers is returned when a Reactor is created with an empty members list.
var ErrNoMembers = errors.New("reactor: at least one member is required")

// NamedAgent extends agents.Agent with name and chat accessors, allowing the
// reactor to inspect each agent's identity and private conversation.
type NamedAgent interface {
	agents.Agent
	AgentName() string
	AgentChat() *chat.Chat
}

// TeamRole identifies an agent's function within a team.
type TeamRole string

// TeamMember pairs a NamedAgent with its team role.
type TeamMember struct {
	Agent NamedAgent
	Role  TeamRole
}

// Selection represents the coordinator's decision.
type Selection struct {
	Members []int // Indices of members to run. Empty when Done is true.
	Done    bool
}

// Coordinator decides which team member(s) should act next.
type Coordinator interface {
	Next(ctx context.Context, shared *chat.Chat, members []TeamMember) (Selection, error)
}

// Options configures a Reactor.
type Options struct {
	Coordinator Coordinator
}

// Reactor orchestrates multiple agents over a shared conversation. It
// implements both agents.Agent and NamedAgent, making it composable/nestable.
type Reactor struct {
	name        string
	members     []TeamMember
	shared      *chat.Chat
	cursors     []int
	coordinator Coordinator
}

// New creates a Reactor with the given name, shared chat, members, and options.
// It returns ErrNoMembers if the members slice is empty.
func New(name string, shared *chat.Chat, members []TeamMember, opts Options) (*Reactor, error) {
	if len(members) == 0 {
		return nil, ErrNoMembers
	}

	return &Reactor{
		name:        name,
		members:     members,
		shared:      shared,
		cursors:     make([]int, len(members)),
		coordinator: opts.Coordinator,
	}, nil
}

// AgentName returns the reactor's name.
func (r *Reactor) AgentName() string { return r.name }

// AgentChat returns the reactor's shared chat.
func (r *Reactor) AgentChat() *chat.Chat { return r.shared }

// Run executes the orchestration loop. On each iteration the coordinator picks
// one or more members, new shared messages are synced to each member's private
// chat, the members run (concurrently if multiple), and replies are appended to
// the shared chat. The loop ends when the coordinator signals done or returns
// an error.
func (r *Reactor) Run(ctx context.Context) (message.Message, error) {
	for {
		sel, err := r.coordinator.Next(ctx, r.shared, r.members)
		if err != nil {
			return message.Message{}, fmt.Errorf("reactor: coordinator: %w", err)
		}

		if sel.Done {
			if last, ok := r.shared.Last(); ok {
				return last, nil
			}

			return message.Message{}, nil
		}

		for _, idx := range sel.Members {
			if idx < 0 || idx >= len(r.members) {
				return message.Message{}, fmt.Errorf("reactor: coordinator returned invalid member index %d", idx)
			}
		}

		if len(sel.Members) == 1 {
			idx := sel.Members[0]
			r.syncToAgent(idx)

			reply, err := r.members[idx].Agent.Run(ctx)
			if err != nil {
				return message.Message{}, fmt.Errorf("reactor: agent %q: %w", r.members[idx].Agent.AgentName(), err)
			}

			r.shared.Append(reply)
		} else {
			if err := r.runConcurrent(ctx, sel.Members); err != nil {
				return message.Message{}, err
			}
		}
	}
}

// runConcurrent syncs and runs multiple agents concurrently. Replies are
// appended to the shared chat in selection order for deterministic output.
func (r *Reactor) runConcurrent(ctx context.Context, indices []int) error {
	// Sync all selected agents before launching goroutines.
	for _, idx := range indices {
		r.syncToAgent(idx)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		reply message.Message
		err   error
	}

	results := make([]result, len(indices))

	var wg sync.WaitGroup
	wg.Add(len(indices))

	for i, idx := range indices {
		go func() {
			defer wg.Done()

			reply, err := r.members[idx].Agent.Run(ctx)
			results[i] = result{reply: reply, err: err}

			if err != nil {
				cancel()
			}
		}()
	}

	wg.Wait()

	// Append replies in selection order and return first error.
	var firstErr error
	for i, idx := range indices {
		if results[i].err != nil && firstErr == nil {
			firstErr = fmt.Errorf("reactor: agent %q: %w", r.members[idx].Agent.AgentName(), results[i].err)
		}

		if results[i].err == nil {
			r.shared.Append(results[i].reply)
		}
	}

	return firstErr
}

// syncToAgent copies new messages from the shared chat into the agent's private
// chat. Messages from the agent itself are skipped. All synced messages have
// their role remapped to User while preserving Sender, Parts, and Metadata.
func (r *Reactor) syncToAgent(idx int) {
	agent := r.members[idx].Agent
	newMsgs := r.shared.Since(r.cursors[idx])

	for _, msg := range newMsgs {
		if msg.Sender == agent.AgentName() {
			continue
		}

		synced := message.Message{
			Sender:   msg.Sender,
			Role:     role.User,
			Parts:    msg.Parts,
			Metadata: msg.Metadata,
		}
		agent.AgentChat().Append(synced)
	}

	r.cursors[idx] += len(newMsgs)
}
