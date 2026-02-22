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

// agentEndMsg signals that the named agent finished its ReAct loop.
type agentEndMsg struct {
	agent string
}

// askUserMsg delivers a pending question from the ask responder.
type askUserMsg struct {
	question ask.Question
	agent    string
}

// askAnsweredMsg is sent after the user answers an ask prompt.
type askAnsweredMsg struct {
	questionID string
	response   string
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

// tickMsg drives spinner animation in active reasoning chains.
type tickMsg time.Time
