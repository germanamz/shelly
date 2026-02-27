package bridge

import (
	"context"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	"github.com/germanamz/shelly/pkg/engine"
)

// Start launches the event watcher and chat watcher goroutines.
// Both goroutines only call p.Send() — they never touch model state directly.
// Returns a cancel function that cancels the bridge context and waits for
// both goroutines to exit, ensuring no stale messages are sent after return.
func Start(ctx context.Context, p *tea.Program, c *chat.Chat, events *engine.EventBus) context.CancelFunc {
	bridgeCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	sub := events.Subscribe(64)

	// Event watcher: converts engine events to bubbletea messages.
	wg.Go(func() {
		defer events.Unsubscribe(sub)
		for {
			select {
			case <-bridgeCtx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				switch ev.Kind {
				case engine.EventAskUser:
					q, ok := ev.Data.(ask.Question)
					if !ok {
						continue
					}
					p.Send(msgs.AskUserMsg{Question: q, Agent: ev.Agent})

				case engine.EventAgentStart:
					var prefix, parent string
					if d, ok := ev.Data.(agent.AgentEventData); ok {
						prefix = d.Prefix
						parent = d.Parent
					}
					p.Send(msgs.AgentStartMsg{Agent: ev.Agent, Prefix: prefix, Parent: parent})

				case engine.EventAgentEnd:
					var parent string
					if d, ok := ev.Data.(agent.AgentEventData); ok {
						parent = d.Parent
					}
					p.Send(msgs.AgentEndMsg{Agent: ev.Agent, Parent: parent})
				}
			}
		}
	})

	// Chat watcher: detects new messages via Wait/Since and forwards them.
	wg.Go(func() {
		cursor := c.Len()
		for {
			_, err := c.Wait(bridgeCtx, cursor)

			// Always drain pending messages even when context is cancelled.
			chatMsgs := c.Since(cursor)
			for _, msg := range chatMsgs {
				p.Send(msgs.ChatMessageMsg{Msg: msg})
				cursor++
			}

			if err != nil {
				return
			}
		}
	})

	return func() {
		cancel()
		wg.Wait()
	}
}
