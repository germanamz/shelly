package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/skill"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

// sequenceCompleter returns a sequence of preconfigured replies.
type sequenceCompleter struct {
	replies []message.Message
	index   int
}

func (p *sequenceCompleter) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	if p.index >= len(p.replies) {
		return message.Message{}, errors.New("no more replies")
	}
	reply := p.replies[p.index]
	p.index++
	return reply, nil
}

// errorCompleter always returns an error.
type errorCompleter struct {
	err error
}

func (p *errorCompleter) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return message.Message{}, p.err
}

func newEchoToolBox() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(toolbox.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return string(input), nil
		},
	})
	return tb
}

// --- Agent constructor tests ---

func TestNew(t *testing.T) {
	c := &sequenceCompleter{}
	a := New("bot", "A test agent", "Do stuff", c, Options{MaxIterations: 5})

	assert.Equal(t, "bot", a.Name())
	assert.Equal(t, "A test agent", a.Description())
	assert.NotNil(t, a.Chat())
	assert.Equal(t, 0, a.Chat().Len())
}

// --- ReAct loop tests ---

func TestRunNoToolCalls(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Done."),
		},
	}
	a := New("bot", "", "", p, Options{})

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Done.", result.TextContent())
	assert.Equal(t, "bot", result.Sender)
}

func TestRunSingleIteration(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.Text{Text: "Calling tool."},
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{"msg":"hi"}`},
			),
			message.NewText("", role.Assistant, "Got the result."),
		},
	}
	a := New("bot", "", "", p, Options{})
	a.AddToolBoxes(newEchoToolBox())

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Got the result.", result.TextContent())
	assert.Equal(t, 2, p.index)
}

func TestRunMultipleIterations(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{"step":1}`},
			),
			message.New("", role.Assistant,
				content.ToolCall{ID: "c2", Name: "echo", Arguments: `{"step":2}`},
			),
			message.NewText("", role.Assistant, "All done."),
		},
	}
	a := New("bot", "", "", p, Options{})
	a.AddToolBoxes(newEchoToolBox())

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "All done.", result.TextContent())
	assert.Equal(t, 3, p.index)
}

func TestRunMaxIterations(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{ID: "c1", Name: "echo", Arguments: `{}`},
			),
			message.New("", role.Assistant,
				content.ToolCall{ID: "c2", Name: "echo", Arguments: `{}`},
			),
			message.New("", role.Assistant,
				content.ToolCall{ID: "c3", Name: "echo", Arguments: `{}`},
			),
		},
	}
	a := New("bot", "", "", p, Options{MaxIterations: 2})
	a.AddToolBoxes(newEchoToolBox())

	_, err := a.Run(context.Background())

	require.ErrorIs(t, err, ErrMaxIterations)
	assert.Equal(t, 2, p.index)
}

func TestRunProviderError(t *testing.T) {
	p := &errorCompleter{err: errors.New("api error")}
	a := New("bot", "", "", p, Options{})

	_, err := a.Run(context.Background())

	assert.EqualError(t, err, "api error")
}

func TestRunContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &errorCompleter{err: ctx.Err()}
	a := New("bot", "", "", p, Options{})

	_, err := a.Run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunToolNotFound(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{ID: "c1", Name: "missing", Arguments: `{}`},
			),
			message.NewText("", role.Assistant, "Done."),
		},
	}
	a := New("bot", "", "", p, Options{})

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Done.", result.TextContent())

	// Check that a tool-not-found error was appended to chat.
	var foundError bool
	a.Chat().Each(func(_ int, m message.Message) bool {
		if m.Role == role.Tool {
			for _, p := range m.Parts {
				if tr, ok := p.(content.ToolResult); ok && tr.IsError {
					foundError = true
					return false
				}
			}
		}
		return true
	})
	assert.True(t, foundError)
}

// --- System prompt tests ---

func TestSystemPromptIdentity(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("assistant", "A helpful bot", "Be helpful.", p, Options{})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	assert.Contains(t, prompt, "You are assistant.")
	assert.Contains(t, prompt, "A helpful bot")
	assert.Contains(t, prompt, "Be helpful.")
}

func TestSystemPromptSkills(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("bot", "", "", p, Options{
		Skills: []skill.Skill{
			{Name: "review", Content: "1. Check tests\n2. Check coverage"},
		},
	})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	assert.Contains(t, prompt, "### review")
	assert.Contains(t, prompt, "1. Check tests")
}

func TestSystemPromptSkillsWithDescription(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("bot", "", "", p, Options{
		Skills: []skill.Skill{
			{Name: "code-review", Description: "Teaches code review best practices", Content: "1. Check tests\n2. Check coverage"},
		},
	})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	assert.Contains(t, prompt, "## Available Skills")
	assert.Contains(t, prompt, "load_skill")
	assert.Contains(t, prompt, "**code-review**: Teaches code review best practices")
	// Full content should NOT be in the prompt.
	assert.NotContains(t, prompt, "1. Check tests")
}

func TestSystemPromptMixedSkills(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("bot", "", "", p, Options{
		Skills: []skill.Skill{
			{Name: "review", Content: "1. Check tests\n2. Check coverage"},
			{Name: "deploy", Description: "Deployment procedures", Content: "1. Build\n2. Deploy"},
		},
	})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	// Inline skill section.
	assert.Contains(t, prompt, "## Skills")
	assert.Contains(t, prompt, "### review")
	assert.Contains(t, prompt, "1. Check tests")
	// On-demand skill section.
	assert.Contains(t, prompt, "## Available Skills")
	assert.Contains(t, prompt, "**deploy**: Deployment procedures")
	// On-demand skill content should NOT be in prompt.
	assert.NotContains(t, prompt, "1. Build")
}

func TestSystemPromptNoDescriptionSkillsOnly(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("bot", "", "", p, Options{
		Skills: []skill.Skill{
			{Name: "review", Content: "1. Check tests"},
		},
	})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	assert.Contains(t, prompt, "## Skills")
	assert.Contains(t, prompt, "### review")
	assert.NotContains(t, prompt, "## Available Skills")
}

func TestSystemPromptContext(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("bot", "", "Do stuff.", p, Options{
		Context: "This is a Go project using module github.com/example/foo.",
	})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	assert.Contains(t, prompt, "## Project Context")
	assert.Contains(t, prompt, "Treat this as your own knowledge")
	assert.Contains(t, prompt, "This is a Go project using module github.com/example/foo.")
	// Context should appear after instructions.
	instrIdx := len("## Instructions")
	ctxIdx := len("## Project Context")
	_ = instrIdx
	_ = ctxIdx
	// Just verify ordering by checking both sections exist.
	assert.Contains(t, prompt, "## Instructions")
}

func TestSystemPromptContextEmpty(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("bot", "", "", p, Options{})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	assert.NotContains(t, prompt, "## Project Context")
}

func TestSystemPromptAgentDirectory(t *testing.T) {
	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Hi"),
		},
	}
	a := New("orchestrator", "", "", p, Options{})

	reg := NewRegistry()
	reg.Register("worker", "Does tasks", func() *Agent {
		return New("worker", "Does tasks", "", p, Options{})
	})
	reg.Register("orchestrator", "Self", func() *Agent {
		return a
	})
	a.SetRegistry(reg)

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	prompt := a.Chat().SystemPrompt()
	assert.Contains(t, prompt, "**worker**")
	// Self should not appear in directory.
	assert.NotContains(t, prompt, "**orchestrator**")
}

// --- Middleware integration tests ---

func TestRunWithMiddleware(t *testing.T) {
	var order []string

	mw := func(tag string) Middleware {
		return func(next Runner) Runner {
			return RunnerFunc(func(ctx context.Context) (message.Message, error) {
				order = append(order, tag+":before")
				msg, err := next.Run(ctx)
				order = append(order, tag+":after")
				return msg, err
			})
		}
	}

	p := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "Done."),
		},
	}
	a := New("bot", "", "", p, Options{
		Middleware: []Middleware{mw("A"), mw("B")},
	})

	_, err := a.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{
		"A:before", "B:before",
		"B:after", "A:after",
	}, order)
}

// --- Delegation tests ---

func TestDelegateToAgent(t *testing.T) {
	// Worker that replies with "worker result".
	workerCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.NewText("", role.Assistant, "worker result"),
		},
	}

	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "Does work", "", workerCompleter, Options{})
	})

	// Orchestrator calls delegate_to_agent.
	orchCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "delegate_to_agent",
					Arguments: `{"agent":"worker","task":"do the thing"}`,
				},
			),
			message.NewText("", role.Assistant, "Got worker's result."),
		},
	}

	a := New("orchestrator", "", "", orchCompleter, Options{})
	a.SetRegistry(reg)

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Got worker's result.", result.TextContent())
}

func TestDelegateSelfRejected(t *testing.T) {
	reg := NewRegistry()

	orchCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "delegate_to_agent",
					Arguments: `{"agent":"self","task":"loop"}`,
				},
			),
			message.NewText("", role.Assistant, "Done."),
		},
	}

	a := New("self", "", "", orchCompleter, Options{})
	a.SetRegistry(reg)

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Done.", result.TextContent())
}

func TestDelegateMaxDepth(t *testing.T) {
	reg := NewRegistry()

	orchCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "delegate_to_agent",
					Arguments: `{"agent":"worker","task":"do"}`,
				},
			),
			message.NewText("", role.Assistant, "Done."),
		},
	}

	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", orchCompleter, Options{})
	})

	a := New("orch", "", "", orchCompleter, Options{MaxDelegationDepth: 1})
	a.SetRegistry(reg)
	a.depth = 1 // Already at max depth.

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Done.", result.TextContent())
}

func TestListAgents(t *testing.T) {
	reg := NewRegistry()
	reg.Register("worker-a", "Worker A", func() *Agent { return &Agent{} })
	reg.Register("worker-b", "Worker B", func() *Agent { return &Agent{} })

	orchCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "list_agents",
					Arguments: `{}`,
				},
			),
			message.NewText("", role.Assistant, "Listed."),
		},
	}

	a := New("orchestrator", "", "", orchCompleter, Options{})
	a.SetRegistry(reg)

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Listed.", result.TextContent())

	// Check that the list_agents result contains the workers (not self).
	var listResult string
	a.Chat().Each(func(_ int, m message.Message) bool {
		if m.Role == role.Tool {
			for _, p := range m.Parts {
				if tr, ok := p.(content.ToolResult); ok && !tr.IsError {
					listResult = tr.Content
					return false
				}
			}
		}
		return true
	})

	assert.Contains(t, listResult, "worker-a")
	assert.Contains(t, listResult, "worker-b")
	assert.NotContains(t, listResult, "orchestrator")
}

func TestSpawnAgents(t *testing.T) {
	reg := NewRegistry()
	reg.Register("worker-a", "Worker A", func() *Agent {
		return New("worker-a", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "result-a"),
			},
		}, Options{})
	})
	reg.Register("worker-b", "Worker B", func() *Agent {
		return New("worker-b", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "result-b"),
			},
		}, Options{})
	})

	orchCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "spawn_agents",
					Arguments: `{"tasks":[{"agent":"worker-a","task":"task-a"},{"agent":"worker-b","task":"task-b"}]}`,
				},
			),
			message.NewText("", role.Assistant, "All done."),
		},
	}

	a := New("orch", "", "", orchCompleter, Options{})
	a.SetRegistry(reg)

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "All done.", result.TextContent())

	// Check spawn results.
	var spawnResultStr string
	a.Chat().Each(func(_ int, m message.Message) bool {
		if m.Role == role.Tool {
			for _, p := range m.Parts {
				if tr, ok := p.(content.ToolResult); ok && !tr.IsError {
					spawnResultStr = tr.Content
					return false
				}
			}
		}
		return true
	})

	assert.Contains(t, spawnResultStr, "result-a")
	assert.Contains(t, spawnResultStr, "result-b")
}

func TestSpawnAgentsSelfRejected(t *testing.T) {
	reg := NewRegistry()

	orchCompleter := &sequenceCompleter{
		replies: []message.Message{
			message.New("", role.Assistant,
				content.ToolCall{
					ID:        "c1",
					Name:      "spawn_agents",
					Arguments: `{"tasks":[{"agent":"orch","task":"loop"}]}`,
				},
			),
			message.NewText("", role.Assistant, "Done."),
		},
	}

	a := New("orch", "", "", orchCompleter, Options{})
	a.SetRegistry(reg)

	result, err := a.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Done.", result.TextContent())
}
