package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Question represents a question from a child agent to its parent.
type Question struct {
	ID      string `json:"id"`
	Agent   string `json:"agent"`
	Content string `json:"content"`
}

// InteractionChannel provides bidirectional communication between a child
// agent and its parent during delegation. The child sends questions via
// request_input, and the parent (or an auto-answer mechanism) responds.
type InteractionChannel struct {
	questionCh chan Question
	answerCh   chan string
	idCounter  atomic.Int64
}

// NewInteractionChannel creates a new InteractionChannel with buffered
// channels for question/answer exchange.
func NewInteractionChannel() *InteractionChannel {
	return &InteractionChannel{
		questionCh: make(chan Question, 1),
		answerCh:   make(chan string, 1),
	}
}

// Questions returns the receive-only channel from which the parent reads
// questions sent by the child via request_input.
func (ic *InteractionChannel) Questions() <-chan Question {
	return ic.questionCh
}

// Answer sends a response back to the child's pending request_input call.
func (ic *InteractionChannel) Answer(answer string) {
	ic.answerCh <- answer
}

// requestInputTool creates a tool that allows a child agent to ask its parent
// (or auto-answer mechanism) a question and wait for the response.
func requestInputTool(a *Agent, ic *InteractionChannel) toolbox.Tool {
	return toolbox.Tool{
		Name:        "request_input",
		Description: "Ask the parent agent or orchestrator a question and wait for a response. Use this when you need clarification, a decision, or information that only the parent context can provide.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"question":{"type":"string","description":"The question to ask the parent"}},"required":["question"]}`),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var ri requestInputInput
			if err := json.Unmarshal(input, &ri); err != nil {
				return "", fmt.Errorf("request_input: invalid input: %w", err)
			}

			if ri.Question == "" {
				return "", fmt.Errorf("request_input: question is required")
			}

			q := Question{
				ID:      fmt.Sprintf("q-%d", ic.idCounter.Add(1)),
				Agent:   a.name,
				Content: ri.Question,
			}

			// Send question to parent.
			select {
			case ic.questionCh <- q:
			case <-ctx.Done():
				return "", ctx.Err()
			}

			// Wait for answer.
			select {
			case answer := <-ic.answerCh:
				return answer, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	}
}

type requestInputInput struct {
	Question string `json:"question"`
}

// autoAnswer runs a goroutine that reads questions from the InteractionChannel
// and responds using the delegation context. It stops when ctx is canceled.
func autoAnswer(ctx context.Context, ic *InteractionChannel, delegationContext string) {
	go func() {
		for {
			select {
			case q := <-ic.questionCh:
				answer := fmt.Sprintf(
					"Based on the delegation context for your question %q:\n\n%s",
					q.Content, delegationContext,
				)
				select {
				case ic.answerCh <- answer:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
