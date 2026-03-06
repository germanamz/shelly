package effects

import (
	"context"
	"fmt"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addToolRound appends an assistant tool-call message and a tool result message.
func addToolRound(c *chat.Chat, callID, toolName, args, result string, isError bool) {
	c.Append(message.New("", role.Assistant,
		content.ToolCall{ID: callID, Name: toolName, Arguments: args},
	))
	c.Append(message.New("", role.Tool,
		content.ToolResult{ToolCallID: callID, Content: result, IsError: isError},
	))
}

func newStallIC(iteration int, c *chat.Chat) agent.IterationContext {
	return agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: iteration,
		Chat:      c,
		Completer: &sequenceCompleter{},
	}
}

func TestStallDetectEffect_SkipsAfterComplete(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 3})

	c := chat.New(message.NewText("", role.System, "sys"))
	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 5,
		Chat:      c,
		Completer: &sequenceCompleter{},
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)
}

func TestStallDetectEffect_SkipsIteration0(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 3})

	c := chat.New(message.NewText("", role.System, "sys"))
	err := e.Eval(context.Background(), newStallIC(0, c))
	require.NoError(t, err)
}

func TestStallDetectEffect_SkipsBelowWindowSize(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 6})

	c := chat.New(message.NewText("", role.System, "sys"))
	// Add only 3 tool rounds — below window of 6.
	for i := range 3 {
		addToolRound(c, fmt.Sprintf("c%d", i), "read_file", `{"path":"a.go"}`, "same content", false)
	}

	err := e.Eval(context.Background(), newStallIC(4, c))
	require.NoError(t, err)
	// No message injected beyond system + 3*(assistant+tool) = 7.
	assert.Equal(t, 7, c.Len())
}

func TestStallDetectEffect_NoStallWithDiverseResults(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 4, SimilarityThreshold: 0.8})

	c := chat.New(message.NewText("", role.System, "sys"))
	// 4 rounds with unique results.
	for i := range 4 {
		addToolRound(c, fmt.Sprintf("c%d", i), "read_file",
			fmt.Sprintf(`{"path":"file%d.go"}`, i),
			fmt.Sprintf("unique content %d", i), false)
	}

	err := e.Eval(context.Background(), newStallIC(5, c))
	require.NoError(t, err)
	assert.Equal(t, 9, c.Len()) // no nudge injected
}

func TestStallDetectEffect_InjectsNudgeOnStall(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 4, SimilarityThreshold: 0.5})

	c := chat.New(message.NewText("", role.System, "sys"))
	// 4 rounds: same tool, same error result → all fingerprints identical → 3/4 = 75% duplicates.
	for i := range 4 {
		addToolRound(c, fmt.Sprintf("c%d", i), "exec",
			`{"cmd":"make build"}`, "exit code 1: build failed", true)
	}

	err := e.Eval(context.Background(), newStallIC(5, c))
	require.NoError(t, err)

	// Should have injected a nudge message.
	last := c.At(c.Len() - 1)
	assert.Equal(t, role.User, last.Role)
	assert.Contains(t, last.TextContent(), "stalled")
}

func TestStallDetectEffect_NudgeOnlyOnce(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 4, SimilarityThreshold: 0.5})

	c := chat.New(message.NewText("", role.System, "sys"))
	for i := range 4 {
		addToolRound(c, fmt.Sprintf("c%d", i), "exec",
			`{"cmd":"make build"}`, "exit code 1: build failed", true)
	}

	// First eval — nudge.
	err := e.Eval(context.Background(), newStallIC(5, c))
	require.NoError(t, err)
	lenAfterNudge := c.Len()

	// Second eval immediately after — should NOT nudge again (within same window).
	err = e.Eval(context.Background(), newStallIC(6, c))
	require.NoError(t, err)
	assert.Equal(t, lenAfterNudge, c.Len())
}

func TestStallDetectEffect_EscalatesToError(t *testing.T) {
	window := 4
	e := NewStallDetectEffect(StallDetectConfig{Window: window, SimilarityThreshold: 0.5})

	c := chat.New(message.NewText("", role.System, "sys"))
	for i := range 4 {
		addToolRound(c, fmt.Sprintf("c%d", i), "exec",
			`{"cmd":"make build"}`, "exit code 1: build failed", true)
	}

	// First eval at iteration 5 — nudge.
	err := e.Eval(context.Background(), newStallIC(5, c))
	require.NoError(t, err)
	assert.True(t, e.nudged)

	// Add more stalled rounds.
	for i := range window {
		addToolRound(c, fmt.Sprintf("d%d", i), "exec",
			`{"cmd":"make build"}`, "exit code 1: build failed", true)
	}

	// Second eval after another window — should return error.
	err = e.Eval(context.Background(), newStallIC(5+window, c))
	assert.ErrorIs(t, err, agent.ErrStallDetected)
}

func TestStallDetectEffect_Reset(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 4, SimilarityThreshold: 0.5})

	c := chat.New(message.NewText("", role.System, "sys"))
	for i := range 4 {
		addToolRound(c, fmt.Sprintf("c%d", i), "exec",
			`{"cmd":"make"}`, "error", true)
	}

	_ = e.Eval(context.Background(), newStallIC(5, c))
	assert.True(t, e.nudged)

	e.Reset()
	assert.False(t, e.nudged)
	assert.Zero(t, e.nudgeIteration)
}

func TestStallDetectEffect_DefaultConfig(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{})
	assert.Equal(t, defaultStallWindow, e.cfg.Window)
	assert.InEpsilon(t, defaultStallSimilarityThreshold, e.cfg.SimilarityThreshold, 1e-9)
}

func TestStallDetectEffect_InvalidThresholdDefaulted(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{SimilarityThreshold: 1.5})
	assert.InEpsilon(t, defaultStallSimilarityThreshold, e.cfg.SimilarityThreshold, 1e-9)

	e2 := NewStallDetectEffect(StallDetectConfig{SimilarityThreshold: -0.5})
	assert.InEpsilon(t, defaultStallSimilarityThreshold, e2.cfg.SimilarityThreshold, 1e-9)
}

func TestStallDetectEffect_MixedToolsDifferentErrors(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 4, SimilarityThreshold: 0.8})

	c := chat.New(message.NewText("", role.System, "sys"))
	// Different tools but same error content — fingerprints differ because tool name differs.
	addToolRound(c, "c0", "read_file", `{"path":"a.go"}`, "permission denied", true)
	addToolRound(c, "c1", "write_file", `{"path":"a.go"}`, "permission denied", true)
	addToolRound(c, "c2", "exec", `{"cmd":"cat a.go"}`, "permission denied", true)
	addToolRound(c, "c3", "list_dir", `{"path":"."}`, "permission denied", true)

	err := e.Eval(context.Background(), newStallIC(5, c))
	require.NoError(t, err)
	// All fingerprints unique (different tool names) → no stall.
	assert.Equal(t, 9, c.Len())
}

func TestStallDetectEffect_SameToolSameErrorTriggers(t *testing.T) {
	e := NewStallDetectEffect(StallDetectConfig{Window: 4, SimilarityThreshold: 0.5})

	c := chat.New(message.NewText("", role.System, "sys"))
	// Same tool, same error → identical fingerprints.
	addToolRound(c, "c0", "read_file", `{"path":"a.go"}`, "file not found", true)
	addToolRound(c, "c1", "read_file", `{"path":"b.go"}`, "file not found", true)
	addToolRound(c, "c2", "read_file", `{"path":"c.go"}`, "file not found", true)
	addToolRound(c, "c3", "read_file", `{"path":"d.go"}`, "file not found", true)

	err := e.Eval(context.Background(), newStallIC(5, c))
	require.NoError(t, err)
	// Should have injected a nudge.
	last := c.At(c.Len() - 1)
	assert.Equal(t, role.User, last.Role)
	assert.Contains(t, last.TextContent(), "stalled")
}
