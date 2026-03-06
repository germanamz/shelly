package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// PendingQuestion pairs a question with its delegation handle so the parent
// can route answers back to the correct child.
type PendingQuestion struct {
	DelegationID string   `json:"delegation_id"`
	Question     Question `json:"question"`
}

// PendingDelegation represents one in-flight interactive child.
type PendingDelegation struct {
	ID         string
	Agent      string
	Task       string
	QuestionCh chan PendingQuestion  // per-delegation question intake
	AnswerCh   chan string           // route answers back to this child
	DoneCh     <-chan delegateResult // receives final result
	Cancel     context.CancelFunc    // cancel the child
}

// DelegationRegistry tracks active interactive delegations for a parent agent.
// Each child has its own QuestionCh and AnswerCh for 1:1 routing.
type DelegationRegistry struct {
	mu      sync.Mutex
	pending map[string]*PendingDelegation
	counter atomic.Int64
}

// NewDelegationRegistry creates a DelegationRegistry.
func NewDelegationRegistry() *DelegationRegistry {
	return &DelegationRegistry{
		pending: make(map[string]*PendingDelegation),
	}
}

// NextDelegationID returns a unique delegation ID.
func (dr *DelegationRegistry) NextDelegationID() string {
	return fmt.Sprintf("d-%d", dr.counter.Add(1))
}

// Register adds a pending delegation. Returns an error if the ID is already
// registered.
func (dr *DelegationRegistry) Register(pd *PendingDelegation) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	if _, exists := dr.pending[pd.ID]; exists {
		return fmt.Errorf("delegation %q already registered", pd.ID)
	}
	dr.pending[pd.ID] = pd
	return nil
}

// Get returns a pending delegation by ID.
func (dr *DelegationRegistry) Get(id string) (*PendingDelegation, bool) {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	pd, ok := dr.pending[id]
	return pd, ok
}

// Remove removes and returns a pending delegation.
func (dr *DelegationRegistry) Remove(id string) {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	delete(dr.pending, id)
}

// Close cancels all pending children.
func (dr *DelegationRegistry) Close() {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	for id, pd := range dr.pending {
		pd.Cancel()
		delete(dr.pending, id)
	}
}

// interactiveDelegateResult is the return type for interactive mode delegate
// and answer_delegation_questions calls.
type interactiveDelegateResult struct {
	Agent string `json:"agent"`

	// Set for completed children (mutually exclusive with DelegationID).
	Result     string            `json:"result,omitempty"`
	Completion *CompletionResult `json:"completion,omitempty"`
	Error      string            `json:"error,omitempty"`
	Warning    string            `json:"warning,omitempty"`

	// Set for children with pending questions (mutually exclusive with Result).
	DelegationID    string    `json:"delegation_id,omitempty"`
	PendingQuestion *Question `json:"pending_question,omitempty"`
}

// answerDelegationQuestionsTool creates the tool for batched question answering
// in interactive delegation mode.
func answerDelegationQuestionsTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "answer_delegation_questions",
		Description: "Answer pending questions from interactive delegations. Provide answers for all pending questions. The tool blocks until every answered child either asks a follow-up question or completes.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"answers":{"type":"array","items":{"type":"object","properties":{"delegation_id":{"type":"string","description":"The delegation ID from the pending question"},"answer":{"type":"string","description":"Your answer to the child's question"}},"required":["delegation_id","answer"]},"description":"List of answers to pending delegation questions"}},"required":["answers"]}`),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var ai answerInput
			if err := json.Unmarshal(input, &ai); err != nil {
				return "", fmt.Errorf("answer_delegation_questions: invalid input: %w", err)
			}

			if len(ai.Answers) == 0 {
				return "[]", nil
			}

			reg := a.interactiveDelegations
			if reg == nil {
				return "", fmt.Errorf("answer_delegation_questions: interactive delegation not enabled")
			}

			// Validate all delegation IDs exist before sending any answers.
			pds := make([]*PendingDelegation, len(ai.Answers))
			for i, ans := range ai.Answers {
				pd, ok := reg.Get(ans.DelegationID)
				if !ok {
					return "", fmt.Errorf("answer_delegation_questions: delegation %q not found", ans.DelegationID)
				}
				pds[i] = pd
			}

			// Send all answers concurrently.
			for i, ans := range ai.Answers {
				pd := pds[i]
				select {
				case pd.AnswerCh <- ans.Answer:
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}

			// Wait for each child to either ask a follow-up or complete.
			results := make([]interactiveDelegateResult, len(ai.Answers))
			var wg sync.WaitGroup
			for i, ans := range ai.Answers {
				pd := pds[i]
				wg.Go(func() {
					results[i] = waitForChildResponse(ctx, reg, pd, ans.DelegationID, a.delegation.questionTimeout)
				})
			}
			wg.Wait()

			data, err := json.Marshal(results)
			if err != nil {
				return "", fmt.Errorf("answer_delegation_questions: %w", err)
			}
			return string(data), nil
		},
	}
}

// waitForChildResponse waits for a child to either complete or ask another
// question after receiving an answer. Returns the appropriate result.
func waitForChildResponse(ctx context.Context, reg *DelegationRegistry, pd *PendingDelegation, delegationID string, timeout time.Duration) interactiveDelegateResult {
	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}

	for {
		select {
		case pq := <-pd.QuestionCh:
			return interactiveDelegateResult{
				Agent:           pd.Agent,
				DelegationID:    delegationID,
				PendingQuestion: &pq.Question,
			}
		case dr := <-pd.DoneCh:
			reg.Remove(delegationID)
			return completedInteractiveResult(pd.Agent, dr)
		case <-timer:
			pd.Cancel()
			reg.Remove(delegationID)
			return interactiveDelegateResult{
				Agent: pd.Agent,
				Error: fmt.Sprintf("question timeout (%s) exceeded for delegation %s", timeout, delegationID),
			}
		case <-ctx.Done():
			return interactiveDelegateResult{
				Agent: pd.Agent,
				Error: ctx.Err().Error(),
			}
		}
	}
}

// completedInteractiveResult converts a delegateResult into an interactive result.
func completedInteractiveResult(agentName string, dr delegateResult) interactiveDelegateResult {
	return interactiveDelegateResult{
		Agent:      agentName,
		Result:     dr.Result,
		Completion: dr.Completion,
		Error:      dr.Error,
		Warning:    dr.Warning,
	}
}

type answerInput struct {
	Answers []answerEntry `json:"answers"`
}

type answerEntry struct {
	DelegationID string `json:"delegation_id"`
	Answer       string `json:"answer"`
}
