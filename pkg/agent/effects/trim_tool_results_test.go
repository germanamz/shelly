package effects

import (
	"context"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrimToolResultsEffect_SkipsBeforeComplete(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{})

	longContent := strings.Repeat("x", 1000)
	c := chat.New(
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c1", Content: longContent}),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseBeforeComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	tr := c.At(0).Parts[0].(content.ToolResult)
	assert.Equal(t, longContent, tr.Content)
}

func TestTrimToolResultsEffect_SkipsIteration0(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{})

	longContent := strings.Repeat("x", 1000)
	c := chat.New(
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c1", Content: longContent}),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 0,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	tr := c.At(0).Parts[0].(content.ToolResult)
	assert.Equal(t, longContent, tr.Content)
}

func TestTrimToolResultsEffect_TrimsLongResults(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{
		MaxResultLength: 10,
		PreserveRecent:  0,
	})

	c := chat.New(
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c1", Content: "this is a very long tool result"}),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	tr := c.At(0).Parts[0].(content.ToolResult)
	assert.Equal(t, "this is a "+trimSuffix, tr.Content)
}

func TestTrimToolResultsEffect_PreservesRecentMessages(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{
		MaxResultLength: 10,
		PreserveRecent:  2,
	})

	longContent := strings.Repeat("x", 100)
	c := chat.New(
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c1", Content: longContent}),
		message.NewText("bot", role.Assistant, "thinking"),
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c2", Content: longContent}),
		message.NewText("bot", role.Assistant, "more thinking"),
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c3", Content: longContent}),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	// First tool message should be trimmed.
	tr0 := c.At(0).Parts[0].(content.ToolResult)
	assert.Contains(t, tr0.Content, trimSuffix)

	// Last two tool messages should be preserved.
	tr2 := c.At(2).Parts[0].(content.ToolResult)
	assert.Equal(t, longContent, tr2.Content)

	tr4 := c.At(4).Parts[0].(content.ToolResult)
	assert.Equal(t, longContent, tr4.Content)
}

func TestTrimToolResultsEffect_DoesNotTrimErrors(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{
		MaxResultLength: 10,
		PreserveRecent:  0,
	})

	longError := strings.Repeat("e", 100)
	c := chat.New(
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c1", Content: longError, IsError: true}),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	tr := c.At(0).Parts[0].(content.ToolResult)
	assert.Equal(t, longError, tr.Content)
}

func TestTrimToolResultsEffect_DoesNotTrimShortResults(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{
		MaxResultLength: 100,
		PreserveRecent:  0,
	})

	c := chat.New(
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c1", Content: "ok"}),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	tr := c.At(0).Parts[0].(content.ToolResult)
	assert.Equal(t, "ok", tr.Content)
}

func TestTrimToolResultsEffect_MetadataPreventsDuplicateTrimming(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{
		MaxResultLength: 10,
		PreserveRecent:  0,
	})

	c := chat.New(
		message.New("", role.Tool, content.ToolResult{ToolCallID: "c1", Content: "this is a very long tool result"}),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	firstTrim := c.At(0).Parts[0].(content.ToolResult).Content

	// Second eval should not re-trim.
	ic.Iteration = 2
	err = e.Eval(context.Background(), ic)
	require.NoError(t, err)

	secondTrim := c.At(0).Parts[0].(content.ToolResult).Content
	assert.Equal(t, firstTrim, secondTrim)
}

func TestTrimToolResultsEffect_DefaultConfig(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{PreserveRecent: -1})
	assert.Equal(t, defaultMaxResultLength, e.cfg.MaxResultLength)
	assert.Equal(t, defaultPreserveRecent, e.cfg.PreserveRecent)
}

func TestTrimToolResultsEffect_ZeroPreserveRecent(t *testing.T) {
	// 0 is a valid value meaning "preserve nothing".
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{PreserveRecent: 0})
	assert.Equal(t, 0, e.cfg.PreserveRecent)
}

func TestTrimToolResultsEffect_MixedParts(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{
		MaxResultLength: 10,
		PreserveRecent:  0,
	})

	longContent := strings.Repeat("z", 100)
	c := chat.New(
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: longContent},
			content.ToolResult{ToolCallID: "c2", Content: "short"},
			content.ToolResult{ToolCallID: "c3", Content: longContent, IsError: true},
		),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	parts := c.At(0).Parts

	// Long non-error: trimmed.
	tr0 := parts[0].(content.ToolResult)
	assert.Contains(t, tr0.Content, trimSuffix)

	// Short: untouched.
	tr1 := parts[1].(content.ToolResult)
	assert.Equal(t, "short", tr1.Content)

	// Long error: untouched.
	tr2 := parts[2].(content.ToolResult)
	assert.Equal(t, longContent, tr2.Content)
}

func TestTrimToolResultsEffect_NoToolMessages(t *testing.T) {
	e := NewTrimToolResultsEffect(TrimToolResultsConfig{
		MaxResultLength: 10,
		PreserveRecent:  0,
	})

	c := chat.New(
		message.NewText("", role.System, "system prompt"),
		message.NewText("user", role.User, "hello"),
		message.NewText("bot", role.Assistant, "hi"),
	)

	ic := agent.IterationContext{
		Phase:     agent.PhaseAfterComplete,
		Iteration: 1,
		Chat:      c,
	}

	err := e.Eval(context.Background(), ic)
	require.NoError(t, err)

	assert.Equal(t, 3, c.Len())
}
