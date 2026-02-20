// Package chat provides a mutable conversation container for LLM interactions.
package chat

import (
	"context"
	"sync"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// Chat is a mutable conversation container. The zero value is ready to use.
// All methods are safe for concurrent use.
type Chat struct {
	mu       sync.RWMutex
	once     sync.Once
	signal   chan struct{}
	messages []message.Message
}

// init ensures the signal channel is allocated. Safe to call multiple times.
func (c *Chat) init() {
	c.once.Do(func() {
		c.signal = make(chan struct{})
	})
}

// New creates a Chat pre-populated with the given messages.
func New(msgs ...message.Message) *Chat {
	c := &Chat{messages: msgs}
	c.init()

	return c
}

// Append adds one or more messages to the conversation and notifies any
// goroutines blocked in Wait.
func (c *Chat) Append(msgs ...message.Message) {
	c.init()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, msgs...)
	close(c.signal)
	c.signal = make(chan struct{})
}

// Len returns the number of messages in the conversation.
func (c *Chat) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.messages)
}

// At returns the message at the given index.
// It panics if the index is out of range.
func (c *Chat) At(index int) message.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.messages[index]
}

// Last returns the most recent message and true, or a zero Message and false
// if the conversation is empty.
func (c *Chat) Last() (message.Message, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.messages) == 0 {
		return message.Message{}, false
	}

	return c.messages[len(c.messages)-1], true
}

// Messages returns a copy of all messages in the conversation.
func (c *Chat) Messages() []message.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cp := make([]message.Message, len(c.messages))
	copy(cp, c.messages)

	return cp
}

// Each iterates over messages, calling fn for each one. If fn returns false,
// iteration stops early. The read lock is held for the entire iteration; fn
// must not call other Chat methods or a deadlock will occur.
func (c *Chat) Each(fn func(int, message.Message) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i, m := range c.messages {
		if !fn(i, m) {
			return
		}
	}
}

// BySender returns all messages from the given sender.
func (c *Chat) BySender(sender string) []message.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var out []message.Message
	for _, m := range c.messages {
		if m.Sender == sender {
			out = append(out, m)
		}
	}

	return out
}

// SystemPrompt returns the text content of the first system message, or an
// empty string if there is none.
func (c *Chat) SystemPrompt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, m := range c.messages {
		if m.Role == role.System {
			return m.TextContent()
		}
	}

	return ""
}

// Since returns a copy of messages starting from offset. Returns nil if offset
// is out of range.
func (c *Chat) Since(offset int) []message.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if offset >= len(c.messages) || offset < 0 {
		return nil
	}

	cp := make([]message.Message, len(c.messages)-offset)
	copy(cp, c.messages[offset:])

	return cp
}

// Wait blocks until the chat contains more than n messages or ctx is done.
// It returns the current message count. Typical usage with Since:
//
//	cursor := 0
//	for {
//	    cursor, err = chat.Wait(ctx, cursor)
//	    if err != nil { break }
//	    msgs := chat.Since(cursor)
//	    // process msgs …
//	    cursor += len(msgs)
//	}
func (c *Chat) Wait(ctx context.Context, n int) (int, error) {
	c.init()

	for {
		c.mu.RLock()
		cur := len(c.messages)
		sig := c.signal
		c.mu.RUnlock()

		if cur > n {
			return cur, nil
		}

		select {
		case <-ctx.Done():
			return cur, ctx.Err()
		case <-sig:
		}
	}
}
