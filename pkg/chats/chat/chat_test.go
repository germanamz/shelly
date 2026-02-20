package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m1 := message.NewText("alice", role.User, "hello")
	m2 := message.NewText("bot", role.Assistant, "hi")
	c := New(m1, m2)

	assert.Equal(t, 2, c.Len())
}

func TestChat_ZeroValue(t *testing.T) {
	var c Chat

	assert.Equal(t, 0, c.Len())

	_, ok := c.Last()
	assert.False(t, ok)
	assert.Empty(t, c.Messages())
}

func TestChat_Append(t *testing.T) {
	c := New()
	c.Append(message.NewText("alice", role.User, "one"))
	c.Append(
		message.NewText("bot", role.Assistant, "two"),
		message.NewText("alice", role.User, "three"),
	)

	assert.Equal(t, 3, c.Len())
}

func TestChat_At(t *testing.T) {
	m := message.NewText("alice", role.User, "hello")
	c := New(m)

	got := c.At(0)
	assert.Equal(t, role.User, got.Role)
	assert.Equal(t, "hello", got.TextContent())
}

func TestChat_At_Panics(t *testing.T) {
	c := New()
	assert.Panics(t, func() { c.At(0) })
}

func TestChat_Last(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "first"),
		message.NewText("bot", role.Assistant, "second"),
	)

	msg, ok := c.Last()
	assert.True(t, ok)
	assert.Equal(t, "second", msg.TextContent())
}

func TestChat_Last_Empty(t *testing.T) {
	c := New()

	_, ok := c.Last()
	assert.False(t, ok)
}

func TestChat_Messages_ReturnsCopy(t *testing.T) {
	c := New(message.NewText("alice", role.User, "hello"))

	msgs := c.Messages()
	msgs[0] = message.NewText("bot", role.Assistant, "modified")

	assert.Equal(t, "hello", c.At(0).TextContent())
}

func TestChat_Each(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "a"),
		message.NewText("bot", role.Assistant, "b"),
		message.NewText("alice", role.User, "c"),
	)

	var visited []string
	c.Each(func(_ int, m message.Message) bool {
		visited = append(visited, m.TextContent())
		return true
	})

	assert.Equal(t, []string{"a", "b", "c"}, visited)
}

func TestChat_Each_EarlyStop(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "a"),
		message.NewText("bot", role.Assistant, "b"),
		message.NewText("alice", role.User, "c"),
	)

	var visited []string
	c.Each(func(_ int, m message.Message) bool {
		visited = append(visited, m.TextContent())
		return len(visited) < 2
	})

	assert.Equal(t, []string{"a", "b"}, visited)
}

func TestChat_BySender(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "hello"),
		message.NewText("bot", role.Assistant, "hi"),
		message.NewText("alice", role.User, "how are you?"),
		message.NewText("bot", role.Assistant, "great!"),
	)

	msgs := c.BySender("alice")

	assert.Len(t, msgs, 2)
	assert.Equal(t, "hello", msgs[0].TextContent())
	assert.Equal(t, "how are you?", msgs[1].TextContent())
}

func TestChat_BySender_NoMatch(t *testing.T) {
	c := New(message.NewText("alice", role.User, "hello"))

	assert.Empty(t, c.BySender("bob"))
}

func TestChat_BySender_Empty(t *testing.T) {
	c := New()

	assert.Empty(t, c.BySender("alice"))
}

func TestChat_SystemPrompt(t *testing.T) {
	c := New(
		message.NewText("", role.System, "you are helpful"),
		message.NewText("alice", role.User, "hello"),
	)

	assert.Equal(t, "you are helpful", c.SystemPrompt())
}

func TestChat_SystemPrompt_None(t *testing.T) {
	c := New(message.NewText("alice", role.User, "hello"))

	assert.Empty(t, c.SystemPrompt())
}

func TestChat_SystemPrompt_NotFirst(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "hello"),
		message.NewText("", role.System, "system msg"),
	)

	assert.Equal(t, "system msg", c.SystemPrompt())
}

func TestChat_Since(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "a"),
		message.NewText("bot", role.Assistant, "b"),
		message.NewText("alice", role.User, "c"),
	)

	msgs := c.Since(1)
	assert.Len(t, msgs, 2)
	assert.Equal(t, "b", msgs[0].TextContent())
	assert.Equal(t, "c", msgs[1].TextContent())
}

func TestChat_Since_Zero(t *testing.T) {
	c := New(message.NewText("alice", role.User, "a"))

	msgs := c.Since(0)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "a", msgs[0].TextContent())
}

func TestChat_Since_AtEnd(t *testing.T) {
	c := New(message.NewText("alice", role.User, "a"))

	assert.Nil(t, c.Since(1))
}

func TestChat_Since_BeyondEnd(t *testing.T) {
	c := New(message.NewText("alice", role.User, "a"))

	assert.Nil(t, c.Since(5))
}

func TestChat_Since_Negative(t *testing.T) {
	c := New(message.NewText("alice", role.User, "a"))

	assert.Nil(t, c.Since(-1))
}

func TestChat_Since_ReturnsCopy(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "a"),
		message.NewText("bot", role.Assistant, "b"),
	)

	msgs := c.Since(0)
	msgs[0] = message.NewText("eve", role.User, "modified")

	assert.Equal(t, "a", c.At(0).TextContent())
}

func TestChat_Wait_ImmediateReturn(t *testing.T) {
	c := New(
		message.NewText("alice", role.User, "a"),
		message.NewText("bot", role.Assistant, "b"),
	)

	n, err := c.Wait(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestChat_Wait_BlocksUntilAppend(t *testing.T) {
	c := New()

	done := make(chan struct{})
	var got int
	var gotErr error

	go func() {
		got, gotErr = c.Wait(context.Background(), 0)
		close(done)
	}()

	// Give the goroutine time to block.
	time.Sleep(20 * time.Millisecond)

	c.Append(message.NewText("alice", role.User, "hello"))

	select {
	case <-done:
		require.NoError(t, gotErr)
		assert.Equal(t, 1, got)
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after Append")
	}
}

func TestChat_Wait_ContextCancelled(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	var gotErr error

	go func() {
		_, gotErr = c.Wait(ctx, 0)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		require.ErrorIs(t, gotErr, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after context cancel")
	}
}

func TestChat_Wait_ZeroValue(t *testing.T) {
	var c Chat

	done := make(chan struct{})
	var got int
	var gotErr error

	go func() {
		got, gotErr = c.Wait(context.Background(), 0)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	c.Append(message.NewText("alice", role.User, "hello"))

	select {
	case <-done:
		require.NoError(t, gotErr)
		assert.Equal(t, 1, got)
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after Append on zero-value Chat")
	}
}

func TestChat_Wait_MultipleWaiters(t *testing.T) {
	c := New()
	const waiters = 5

	var wg sync.WaitGroup
	results := make([]int, waiters)

	for i := range waiters {
		wg.Go(func() {
			n, err := c.Wait(context.Background(), 0)
			assert.NoError(t, err)
			results[i] = n
		})
	}

	time.Sleep(20 * time.Millisecond)
	c.Append(message.NewText("alice", role.User, "hello"))
	wg.Wait()

	for i, n := range results {
		assert.GreaterOrEqual(t, n, 1, "waiter %d got n=%d", i, n)
	}
}

func TestChat_ConcurrentAccess(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())

	var writers sync.WaitGroup
	var waiters sync.WaitGroup

	// Writers.
	for range 10 {
		writers.Go(func() {
			for range 100 {
				c.Append(message.NewText("alice", role.User, "hello"))
			}
		})
	}

	// Readers.
	for range 10 {
		writers.Go(func() {
			for range 100 {
				c.Len()
				c.Last()
				c.Messages()
				c.BySender("alice")
				c.SystemPrompt()
				c.Since(0)
			}
		})
	}

	// Waiters.
	for range 5 {
		waiters.Go(func() {
			cursor := 0
			for {
				n, err := c.Wait(ctx, cursor)
				if err != nil {
					return
				}
				cursor = n
			}
		})
	}

	writers.Wait()
	cancel()
	waiters.Wait()

	assert.Equal(t, 1000, c.Len())
}
