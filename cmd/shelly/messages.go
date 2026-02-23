package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
)

// chatMessageMsg delivers a new chat message from the bridge goroutine.
type chatMessageMsg struct {
	msg message.Message
}

// agentStartMsg signals that the named agent started its ReAct loop.
type agentStartMsg struct {
	agent  string
	prefix string // display prefix (e.g. "🤖", "📝")
}

// agentEndMsg signals that the named agent finished its ReAct loop.
type agentEndMsg struct {
	agent string
}

// askUserMsg delivers a pending question from the ask responder.
type askUserMsg struct {
	question ask.Question
	agent    string
}

// askAnswer holds the response for a single question in a batch.
type askAnswer struct {
	questionID string
	response   string
}

// askBatchReadyMsg signals that the batching window has closed.
type askBatchReadyMsg struct{}

// askBatchAnsweredMsg is sent after the user answers all batched questions.
type askBatchAnsweredMsg struct {
	answers []askAnswer
}

// inputSubmitMsg carries the text the user submitted from the input box.
type inputSubmitMsg struct {
	text string
}

// sendCompleteMsg is returned by the tea.Cmd that calls sess.Send.
type sendCompleteMsg struct {
	err      error
	duration time.Duration
}

// programReadyMsg passes the *tea.Program to the model so it can start bridge goroutines.
type programReadyMsg struct {
	program *tea.Program
}

// initDrainMsg fires after a short delay so that stale terminal responses
// (e.g. OSC 11 background-color replies) are discarded before focusing input.
type initDrainMsg struct{}

// tickMsg drives spinner animation in active reasoning chains.
type tickMsg time.Time
