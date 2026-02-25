// Package ask provides a tool that lets agents ask the user a question and
// block until a response is received. Questions can include multiple-choice
// options or be free-form. The Responder manages pending questions and their
// response channels.
package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Question represents a question posed to the user by an agent.
type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options,omitempty"`
}

// OnAskFunc is called when a new question is posed. Implementations should
// notify the frontend so it can present the question and collect a response.
type OnAskFunc func(ctx context.Context, q Question)

// Responder manages pending user questions and their responses. It provides a
// toolbox.Tool that blocks until the user responds or the context is cancelled.
type Responder struct {
	mu      sync.Mutex
	pending map[string]chan string
	onAsk   OnAskFunc
	nextID  atomic.Int64
}

// NewResponder creates a Responder. The onAsk callback is invoked every time
// the agent asks a question, giving the frontend an opportunity to display it.
// If onAsk is nil, questions are still registered but no notification is sent.
func NewResponder(onAsk OnAskFunc) *Responder {
	return &Responder{
		pending: make(map[string]chan string),
		onAsk:   onAsk,
	}
}

// Respond delivers a user response to a pending question. It returns an error
// if the question ID is not found or the receiver is no longer listening.
func (r *Responder) Respond(questionID, response string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, ok := r.pending[questionID]
	if !ok {
		return fmt.Errorf("ask: question %q not found", questionID)
	}

	// The channel is buffered (size 1), so this send will not block as long
	// as the entry is still in pending (meaning no one else has sent yet).
	select {
	case ch <- response:
		delete(r.pending, questionID)
		return nil
	default:
		return fmt.Errorf("ask: question %q is no longer awaiting a response", questionID)
	}
}

// Tools returns a ToolBox containing the ask_user tool.
func (r *Responder) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(r.askUserTool())

	return tb
}

type askInput struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

func (r *Responder) askUserTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "ask_user",
		Description: "Ask the user a question. Optionally provide multiple-choice options. The user may select an option or provide a custom response.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"question":{"type":"string","description":"The question to ask the user"},"options":{"type":"array","items":{"type":"string"},"description":"Optional list of choices for the user to select from"}},"required":["question"]}`),
		Handler:     r.handleAsk,
	}
}

// Ask programmatically poses a question to the user and blocks until a
// response is received or the context is cancelled. Other packages (e.g.
// filesystem) can use this to request user input without going through the
// tool handler JSON layer.
func (r *Responder) Ask(ctx context.Context, text string, options []string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("ask: question is required")
	}

	id := fmt.Sprintf("q-%d", r.nextID.Add(1))
	ch := make(chan string, 1)

	r.mu.Lock()
	r.pending[id] = ch
	r.mu.Unlock()

	q := Question{
		ID:      id,
		Text:    text,
		Options: options,
	}

	if r.onAsk != nil {
		r.onAsk(ctx, q)
	}

	select {
	case <-ctx.Done():
		// When both ctx.Done() and ch are ready, Go's select picks
		// non-deterministically. Drain the channel before giving up so a
		// response that arrived just before cancellation is not lost.
		select {
		case resp := <-ch:
			return resp, nil
		default:
		}

		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()

		return "", ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
}

func (r *Responder) handleAsk(ctx context.Context, input json.RawMessage) (string, error) {
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("ask_user: invalid input: %w", err)
	}

	resp, err := r.Ask(ctx, in.Question, in.Options)
	if err != nil {
		return "", fmt.Errorf("ask_user: %w", err)
	}

	return resp, nil
}
