package effects

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoopDetectEffect_SkipsAfterComplete(t *testing.T) {
	e := NewLoopDetectEffect(LoopDetectConfig{Threshold: 1})

	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "file contents"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// No intervention message should be appended.
	assert.Equal(t, 2, c.Len())
}

func TestLoopDetectEffect_SkipsIteration0(t *testing.T) {
	e := NewLoopDetectEffect(LoopDetectConfig{Threshold: 1})

	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "file contents"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 0,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// No intervention message should be appended.
	assert.Equal(t, 2, c.Len())
}

func TestLoopDetectEffect_NoLoopDetected(t *testing.T) {
	e := NewLoopDetectEffect(LoopDetectConfig{})

	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "contents of foo"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "fs_read", Arguments: `{"path":"/bar"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "contents of bar"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c3", Name: "exec", Arguments: `{"cmd":"ls"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c3", Content: "file list"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 3,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// No intervention — all calls are different.
	assert.Equal(t, 6, c.Len())
}

func TestLoopDetectEffect_DetectsLoop(t *testing.T) {
	e := NewLoopDetectEffect(LoopDetectConfig{})

	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "file contents"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "file contents"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c3", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c3", Content: "file contents"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 3,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Intervention message should be appended.
	assert.Equal(t, 7, c.Len())

	last := c.At(6)
	assert.Equal(t, role.User, last.Role)
	assert.Contains(t, last.TextContent(), "fs_read")
	assert.Contains(t, last.TextContent(), "3 times")
	assert.Contains(t, last.TextContent(), "different approach")
}

func TestLoopDetectEffect_CustomThreshold(t *testing.T) {
	e := NewLoopDetectEffect(LoopDetectConfig{Threshold: 2})

	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "exec", Arguments: `{"cmd":"make"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "error"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "exec", Arguments: `{"cmd":"make"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "error"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 2,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Threshold is 2, and we have 2 identical calls — should intervene.
	assert.Equal(t, 5, c.Len())

	last := c.At(4)
	assert.Equal(t, role.User, last.Role)
	assert.Contains(t, last.TextContent(), "exec")
	assert.Contains(t, last.TextContent(), "2 times")
}

func TestLoopDetectEffect_MultiPartKeyOrder(t *testing.T) {
	// An assistant message with multiple tool calls: the last part is the most
	// recent call. After the fix, keys[0] should be the last part, not the first.
	e := NewLoopDetectEffect(LoopDetectConfig{Threshold: 2})

	c := chat.New(
		// Message with two calls: first is "exec", last is "fs_read".
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "exec", Arguments: `{"cmd":"ls"}`},
			content.ToolCall{ID: "c2", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "ok"},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "ok"},
		),
		// Second message: only "fs_read" with same args.
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c3", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c3", Content: "ok"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 2,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// With correct key order, keys[0] is the most recent (fs_read /foo from c3),
	// keys[1] is fs_read /foo from c2, which makes 2 consecutive identical calls.
	// Threshold is 2, so an intervention message should be appended.
	assert.Equal(t, 6, c.Len(), "expected intervention message")

	last := c.At(5)
	assert.Equal(t, role.User, last.Role)
	assert.Contains(t, last.TextContent(), "fs_read")
	assert.Contains(t, last.TextContent(), "2 times")
}

func TestLoopDetectEffect_DefaultConfig(t *testing.T) {
	e := NewLoopDetectEffect(LoopDetectConfig{})
	assert.Equal(t, defaultLoopThreshold, e.cfg.Threshold)
	assert.Equal(t, defaultLoopWindowSize, e.cfg.WindowSize)
}

func TestLoopDetectEffect_ResetClearsState(t *testing.T) {
	e := NewLoopDetectEffect(LoopDetectConfig{Threshold: 2})

	// Simulate a loop detection that sets lastInjectedCount.
	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "ok"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "fs_read", Arguments: `{"path":"/foo"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "ok"},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 2,
		Chat:      c,
	}

	// First eval should inject.
	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 5, c.Len())

	// Reset and verify fresh state.
	e.Reset()

	// After reset, with a fresh chat showing a loop, it should inject again
	// (because lastInjectedCount was cleared).
	c2 := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c3", Name: "exec", Arguments: `{"cmd":"ls"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c3", Content: "ok"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c4", Name: "exec", Arguments: `{"cmd":"ls"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c4", Content: "ok"},
		),
	)

	ic2 := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c2,
	}

	err = e.Eval(context.Background(), ic2)
	require.NoError(t, err)
	assert.Equal(t, 5, c2.Len(), "should inject again after reset")
}
