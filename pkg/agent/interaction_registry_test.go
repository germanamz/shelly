package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelegationRegistryRegisterAndGet(t *testing.T) {
	reg := NewDelegationRegistry()

	pd := &PendingDelegation{
		ID:       "d-1",
		Agent:    "worker",
		AnswerCh: make(chan string, 1),
		Cancel:   func() {},
	}

	require.NoError(t, reg.Register(pd))

	got, ok := reg.Get("d-1")
	assert.True(t, ok)
	assert.Equal(t, "worker", got.Agent)
}

func TestDelegationRegistryDoubleRegisterRejected(t *testing.T) {
	reg := NewDelegationRegistry()

	pd := &PendingDelegation{ID: "d-1", Cancel: func() {}}
	require.NoError(t, reg.Register(pd))

	err := reg.Register(&PendingDelegation{ID: "d-1", Cancel: func() {}})
	assert.ErrorContains(t, err, "already registered")
}

func TestDelegationRegistryRemove(t *testing.T) {
	reg := NewDelegationRegistry()

	pd := &PendingDelegation{ID: "d-1", Cancel: func() {}}
	require.NoError(t, reg.Register(pd))

	reg.Remove("d-1")

	_, ok := reg.Get("d-1")
	assert.False(t, ok)
}

func TestDelegationRegistryClosesCancelsAll(t *testing.T) {
	reg := NewDelegationRegistry()

	var canceled1, canceled2 bool
	require.NoError(t, reg.Register(&PendingDelegation{ID: "d-1", Cancel: func() { canceled1 = true }}))
	require.NoError(t, reg.Register(&PendingDelegation{ID: "d-2", Cancel: func() { canceled2 = true }}))

	reg.Close()

	assert.True(t, canceled1)
	assert.True(t, canceled2)

	_, ok := reg.Get("d-1")
	assert.False(t, ok)
}

func TestDelegationRegistryNextID(t *testing.T) {
	reg := NewDelegationRegistry()

	id1 := reg.NextDelegationID()
	id2 := reg.NextDelegationID()

	assert.Equal(t, "d-1", id1)
	assert.Equal(t, "d-2", id2)
}

func TestSharedInteractionChannel(t *testing.T) {
	sharedQueue := make(chan PendingQuestion, 4)
	ic := NewSharedInteractionChannel("d-1", sharedQueue)

	a := &Agent{name: "child-1", interaction: ic}
	tool := requestInputTool(a, ic)

	// Run the tool in a goroutine since it blocks.
	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"question":"What API version?"}`))
		ch <- result{out, err}
	}()

	// Read from the shared queue.
	select {
	case pq := <-sharedQueue:
		assert.Equal(t, "d-1", pq.DelegationID)
		assert.Equal(t, "child-1", pq.Question.Agent)
		assert.Equal(t, "What API version?", pq.Question.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question on shared queue")
	}

	// Answer via the IC's answer channel.
	ic.answerCh <- "v2"

	select {
	case r := <-ch:
		require.NoError(t, r.err)
		assert.Equal(t, "v2", r.output)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool result")
	}
}

func TestAnswerDelegationQuestionsToolBasic(t *testing.T) {
	reg := NewDelegationRegistry()
	doneCh := make(chan delegateResult, 1)
	answerCh := make(chan string, 1)

	pd := &PendingDelegation{
		ID:       "d-1",
		Agent:    "worker",
		AnswerCh: answerCh,
		DoneCh:   doneCh,
		Cancel:   func() {},
	}
	require.NoError(t, reg.Register(pd))

	a := &Agent{
		interactiveDelegations: reg,
	}
	tool := answerDelegationQuestionsTool(a)

	// Answer in a goroutine since the tool blocks until child responds.
	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"answers":[{"delegation_id":"d-1","answer":"use REST"}]}`))
		ch <- result{out, err}
	}()

	// Child receives answer.
	select {
	case ans := <-answerCh:
		assert.Equal(t, "use REST", ans)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for answer delivery")
	}

	// Child completes.
	doneCh <- delegateResult{Agent: "worker", Result: "done"}

	select {
	case r := <-ch:
		require.NoError(t, r.err)

		var results []interactiveDelegateResult
		require.NoError(t, json.Unmarshal([]byte(r.output), &results))
		require.Len(t, results, 1)
		assert.Equal(t, "worker", results[0].Agent)
		assert.Equal(t, "done", results[0].Result)
		assert.Empty(t, results[0].DelegationID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool result")
	}
}

func TestAnswerDelegationQuestionsToolFollowUp(t *testing.T) {
	reg := NewDelegationRegistry()
	doneCh := make(chan delegateResult, 1)
	answerCh := make(chan string, 1)

	pd := &PendingDelegation{
		ID:       "d-1",
		Agent:    "worker",
		AnswerCh: answerCh,
		DoneCh:   doneCh,
		Cancel:   func() {},
	}
	require.NoError(t, reg.Register(pd))

	a := &Agent{
		interactiveDelegations: reg,
	}
	tool := answerDelegationQuestionsTool(a)

	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"answers":[{"delegation_id":"d-1","answer":"v2"}]}`))
		ch <- result{out, err}
	}()

	// Child receives answer.
	select {
	case <-answerCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}

	// Child asks a follow-up via shared queue.
	reg.questions <- PendingQuestion{
		DelegationID: "d-1",
		Question:     Question{ID: "q-2", Agent: "worker", Content: "what format?"},
	}

	select {
	case r := <-ch:
		require.NoError(t, r.err)

		var results []interactiveDelegateResult
		require.NoError(t, json.Unmarshal([]byte(r.output), &results))
		require.Len(t, results, 1)
		assert.Equal(t, "d-1", results[0].DelegationID)
		assert.Equal(t, "what format?", results[0].PendingQuestion.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool result")
	}
}

func TestAnswerDelegationQuestionsInvalidID(t *testing.T) {
	reg := NewDelegationRegistry()
	a := &Agent{
		interactiveDelegations: reg,
	}
	tool := answerDelegationQuestionsTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"answers":[{"delegation_id":"d-999","answer":"test"}]}`))
	assert.ErrorContains(t, err, "not found")
}

func TestAnswerDelegationQuestionsNilRegistry(t *testing.T) {
	a := &Agent{}
	tool := answerDelegationQuestionsTool(a)

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"answers":[{"delegation_id":"d-1","answer":"test"}]}`))
	assert.ErrorContains(t, err, "not enabled")
}

func TestInteractiveDelegateFlow(t *testing.T) {
	// Set up a parent agent with interactive delegation enabled.
	reg := NewRegistry()

	// Worker that asks a question, gets an answer, then completes.
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &interactiveCompleter{}, Options{})
	})

	parent := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{
			maxDepth:        2,
			interactionMode: "interactive",
		},
		interactiveDelegations: NewDelegationRegistry(),
	}

	tool := delegateTool(parent)

	// Start interactive delegation.
	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"worker","task":"do stuff","context":"ctx","mode":"interactive"}]}`))
		ch <- result{out, err}
	}()

	// The delegate tool should return with a pending question.
	select {
	case r := <-ch:
		require.NoError(t, r.err)

		var results []interactiveDelegateResult
		require.NoError(t, json.Unmarshal([]byte(r.output), &results))
		require.Len(t, results, 1)
		assert.NotEmpty(t, results[0].DelegationID)
		assert.NotNil(t, results[0].PendingQuestion)
		assert.Equal(t, "What API?", results[0].PendingQuestion.Content)

		// Now answer the question.
		answerTool := answerDelegationQuestionsTool(parent)
		answerInput := fmt.Sprintf(`{"answers":[{"delegation_id":"%s","answer":"REST"}]}`, results[0].DelegationID)

		answerResult, err := answerTool.Handler(context.Background(), json.RawMessage(answerInput))
		require.NoError(t, err)

		var answerResults []interactiveDelegateResult
		require.NoError(t, json.Unmarshal([]byte(answerResult), &answerResults))
		require.Len(t, answerResults, 1)
		assert.Equal(t, "worker", answerResults[0].Agent)
		// Child should have completed.
		assert.NotEmpty(t, answerResults[0].Result)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for interactive delegation result")
	}
}

func TestInteractiveDelegateChildCompletesWithoutAsking(t *testing.T) {
	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done without asking"),
			},
		}, Options{})
	})

	parent := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{
			maxDepth:        2,
			interactionMode: "interactive",
		},
		interactiveDelegations: NewDelegationRegistry(),
	}

	tool := delegateTool(parent)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"worker","task":"quick task","context":"ctx","mode":"interactive"}]}`))
	require.NoError(t, err)

	var results []interactiveDelegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Empty(t, results[0].DelegationID)
	assert.NotEmpty(t, results[0].Result)
}

func TestInteractiveDelegateMultiChild(t *testing.T) {
	reg := NewRegistry()

	// Worker A asks a question.
	reg.Register("asker", "Asks questions", func() *Agent {
		return New("asker", "", "", &interactiveCompleter{}, Options{})
	})

	// Worker B completes immediately.
	reg.Register("quick", "Completes fast", func() *Agent {
		return New("quick", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done fast"),
			},
		}, Options{})
	})

	parent := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{
			maxDepth:        2,
			interactionMode: "interactive",
		},
		interactiveDelegations: NewDelegationRegistry(),
	}

	tool := delegateTool(parent)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"asker","task":"ask stuff","context":"ctx","mode":"interactive"},{"agent":"quick","task":"fast task","context":"ctx","mode":"interactive"}]}`))
	require.NoError(t, err)

	var results []interactiveDelegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 2)

	// One should have a pending question, the other should be complete.
	var hasQuestion, hasResult bool
	for _, r := range results {
		if r.DelegationID != "" && r.PendingQuestion != nil {
			hasQuestion = true
		}
		if r.Result != "" {
			hasResult = true
		}
	}
	assert.True(t, hasQuestion, "expected one child with pending question")
	assert.True(t, hasResult, "expected one child completed")
}

func TestInteractiveDelegateModeRequiresRegistry(t *testing.T) {
	reg := NewRegistry()
	reg.Register("worker", "Does work", func() *Agent {
		return New("worker", "", "", &sequenceCompleter{
			replies: []message.Message{
				message.NewText("", role.Assistant, "done"),
			},
		}, Options{})
	})

	parent := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{maxDepth: 2},
		// No interactiveDelegations set.
	}

	tool := delegateTool(parent)
	_, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"worker","task":"do stuff","context":"ctx","mode":"interactive"}]}`))
	assert.ErrorContains(t, err, "interactive mode requires")
}

func TestInteractiveDelegateQuestionTimeout(t *testing.T) {
	reg := NewRegistry()
	// Agent that takes forever (blocks until context cancel).
	reg.Register("slow", "Slow agent", func() *Agent {
		return New("slow", "", "", &contextCompleter{}, Options{})
	})

	parent := &Agent{
		name:       "orch",
		configName: "orch",
		registry:   reg,
		chat:       chat.New(),
		delegation: delegationConfig{
			maxDepth:        2,
			interactionMode: "interactive",
			questionTimeout: 100 * time.Millisecond,
		},
		interactiveDelegations: NewDelegationRegistry(),
	}

	tool := delegateTool(parent)
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"tasks":[{"agent":"slow","task":"slow task","context":"ctx","mode":"interactive"}]}`))
	require.NoError(t, err)

	var results []interactiveDelegateResult
	require.NoError(t, json.Unmarshal([]byte(result), &results))
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "timeout")
}

func TestOrchestrationToolBoxIncludesAnswerTool(t *testing.T) {
	a := &Agent{
		registry:               NewRegistry(),
		delegation:             delegationConfig{maxDepth: 2},
		interactiveDelegations: NewDelegationRegistry(),
	}

	tb := orchestrationToolBox(a)
	_, ok := tb.Get("answer_delegation_questions")
	assert.True(t, ok, "answer_delegation_questions tool should be registered")
}

func TestOrchestrationToolBoxExcludesAnswerToolWhenNoRegistry(t *testing.T) {
	a := &Agent{
		registry:   NewRegistry(),
		delegation: delegationConfig{maxDepth: 2},
	}

	tb := orchestrationToolBox(a)
	_, ok := tb.Get("answer_delegation_questions")
	assert.False(t, ok, "answer_delegation_questions should not be registered without interactive delegation")
}

func TestDelegationRegistryCloseIdempotent(t *testing.T) {
	reg := NewDelegationRegistry()
	callCount := 0
	require.NoError(t, reg.Register(&PendingDelegation{ID: "d-1", Cancel: func() { callCount++ }}))

	reg.Close()
	reg.Close() // second close should be safe

	assert.Equal(t, 1, callCount, "cancel should only be called once")
}

// interactiveCompleter simulates a child agent that:
// 1. First call: makes a tool call to request_input with "What API?"
// 2. Second call: produces a final text reply using the answer.
type interactiveCompleter struct {
	mu    sync.Mutex
	calls int
}

func (c *interactiveCompleter) Complete(_ context.Context, _ *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	c.mu.Lock()
	call := c.calls
	c.calls++
	c.mu.Unlock()

	if call == 0 {
		// Find request_input tool and make a tool call.
		for _, t := range tools {
			if t.Name == "request_input" {
				return message.New("", role.Assistant, content.ToolCall{
					ID:        "tc-1",
					Name:      "request_input",
					Arguments: `{"question":"What API?"}`,
				}), nil
			}
		}
		return message.NewText("", role.Assistant, "no request_input tool found"), nil
	}

	// Second call: we should have the answer in the chat. Just produce a final reply.
	return message.NewText("", role.Assistant, "completed with answer"), nil
}
