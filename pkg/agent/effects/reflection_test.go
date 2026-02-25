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

func TestReflectionEffect_SkipsAfterComplete(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	c := chat.New(
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "error", IsError: true},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "error", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// No message injected since wrong phase.
	assert.Equal(t, 2, c.Len())
}

func TestReflectionEffect_SkipsIteration0(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	c := chat.New(
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "error", IsError: true},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "error", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 0,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 2, c.Len())
}

func TestReflectionEffect_InjectsAfterConsecutiveFailures(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	c := chat.New(
		message.NewText("", role.System, "sys"),
		message.NewText("user", role.User, "do something"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "exec", Arguments: `{"cmd":"bad"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "command failed", IsError: true},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "exec", Arguments: `{"cmd":"bad2"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "command failed again", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 3,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// Reflection prompt should be injected.
	assert.Equal(t, 7, c.Len())
	last := c.At(6)
	assert.Equal(t, role.User, last.Role)
	assert.Contains(t, last.TextContent(), "2 consecutive tool failures")
	assert.Contains(t, last.TextContent(), "root cause")
}

func TestReflectionEffect_NoInjectionBelowThreshold(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 3})

	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "exec", Arguments: `{"cmd":"bad"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "failed", IsError: true},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "exec", Arguments: `{"cmd":"bad2"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "failed", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 2,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// No injection — only 2 failures, threshold is 3.
	assert.Equal(t, 4, c.Len())
}

func TestReflectionEffect_ResetsAfterSuccess(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	c := chat.New(
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "failed", IsError: true},
		),
		// Success breaks the streak.
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "ok"},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c3", Name: "exec", Arguments: `{"cmd":"bad"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c3", Content: "failed", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 3,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// Only 1 consecutive failure at the end, below threshold.
	assert.Equal(t, 4, c.Len())
}

func TestReflectionEffect_StopsAtUserMessage(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	c := chat.New(
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "failed", IsError: true},
		),
		// User message breaks scanning.
		message.NewText("user", role.User, "try something else"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "exec", Arguments: `{"cmd":"bad"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "failed", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 2,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// Only 1 consecutive failure at end (user msg breaks chain).
	assert.Equal(t, 4, c.Len())
}

func TestReflectionEffect_SkipsAssistantMessages(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	c := chat.New(
		message.NewText("user", role.User, "do something"),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "exec", Arguments: `{"cmd":"bad"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "failed", IsError: true},
		),
		// Assistant message between tool results should be skipped.
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "exec", Arguments: `{"cmd":"bad2"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "failed again", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 3,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// Both failures counted (assistant msgs skipped), so reflection injected.
	assert.Equal(t, 6, c.Len())
	assert.Contains(t, c.At(5).TextContent(), "2 consecutive tool failures")
}

func TestReflectionEffect_Defaults(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{})
	assert.Equal(t, defaultFailureThreshold, e.cfg.FailureThreshold)
}

func TestReflectionEffect_ToolMessageWithNoToolResultParts(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	// A tool-role message with only a Text part (no ToolResult) should not
	// be counted as a failure.
	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "exec", Arguments: `{"cmd":"bad"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "failed", IsError: true},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "exec", Arguments: `{"cmd":"bad2"}`},
		),
		// Tool-role message with only text, no ToolResult parts.
		message.NewText("", role.Tool, "some text without a tool result"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 2,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	// The text-only tool message breaks the streak — no injection.
	assert.Equal(t, 4, c.Len())
}

func TestReflectionEffect_ReInjectionGuard(t *testing.T) {
	e := NewReflectionEffect(ReflectionConfig{FailureThreshold: 2})

	c := chat.New(
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c1", Name: "exec", Arguments: `{"cmd":"bad"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "failed", IsError: true},
		),
		message.New("bot", role.Assistant,
			content.ToolCall{ID: "c2", Name: "exec", Arguments: `{"cmd":"bad2"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c2", Content: "failed again", IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 2,
		Chat:      c,
	}

	// First eval: should inject.
	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 5, c.Len())

	// Second eval with same count: should NOT re-inject.
	ic.Iteration = 3
	err = e.Eval(context.Background(), ic)
	require.NoError(t, err)
	assert.Equal(t, 5, c.Len())
}
