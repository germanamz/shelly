package msgs

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
)

// --- Bridge → TUI messages ---

// ChatMessageMsg delivers a new chat message from the bridge goroutine.
type ChatMessageMsg struct {
	Msg message.Message
}

// AgentStartMsg signals that the named agent started its ReAct loop.
type AgentStartMsg struct {
	Agent  string
	Prefix string // display prefix (e.g. "🤖", "📝")
	Parent string // parent agent name (empty for top-level)
}

// AgentEndMsg signals that the named agent finished its ReAct loop.
type AgentEndMsg struct {
	Agent  string
	Parent string // parent agent name (empty for top-level)
}

// AskUserMsg delivers a pending question from the ask responder.
type AskUserMsg struct {
	Question ask.Question
	Agent    string
}

// --- Internal messages ---

// InputSubmitMsg carries the text the user submitted from the input box.
type InputSubmitMsg struct {
	Text string
}

// SendCompleteMsg is returned by the tea.Cmd that calls sess.Send.
type SendCompleteMsg struct {
	Err        error
	Duration   time.Duration
	Generation uint64
}

// ProgramReadyMsg passes the *tea.Program to the model so it can start bridge goroutines.
type ProgramReadyMsg struct {
	Program *tea.Program
}

// InitDrainMsg fires after a short delay so that stale terminal responses
// (e.g. OSC 11 background-color replies) are discarded before focusing input.
type InitDrainMsg struct{}

// TickMsg drives spinner animation in active reasoning chains.
type TickMsg time.Time

// AskBatchReadyMsg signals that the batching window has closed.
type AskBatchReadyMsg struct{}

// AskBatchAnsweredMsg is sent after the user answers all batched questions.
type AskBatchAnsweredMsg struct {
	Answers []AskAnswer
}

// AskAnswer holds the response for a single question in a batch.
type AskAnswer struct {
	QuestionID string
	Response   string
}

// RespondErrorMsg is sent when a sess.Respond call fails asynchronously.
type RespondErrorMsg struct {
	Err error
}

// FilePickerEntriesMsg delivers the discovered file list.
type FilePickerEntriesMsg struct {
	Entries []string
}
