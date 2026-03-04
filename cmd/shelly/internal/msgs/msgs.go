package msgs

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	"github.com/germanamz/shelly/pkg/tasks"
)

// --- Bridge → TUI messages ---

// ChatMessageMsg delivers a new chat message from the bridge goroutine.
type ChatMessageMsg struct {
	Msg message.Message
}

// AgentStartMsg signals that the named agent started its ReAct loop.
type AgentStartMsg struct {
	Agent         string
	Prefix        string // display prefix (e.g. "🤖", "📝")
	Parent        string // parent agent name (empty for top-level)
	ProviderLabel string // provider display label (e.g. "anthropic/claude-sonnet-4")
	Task          string // delegation task description (empty for top-level)
}

// AgentEndMsg signals that the named agent finished its ReAct loop.
type AgentEndMsg struct {
	Agent   string
	Parent  string // parent agent name (empty for top-level)
	Summary string // completion summary (from CompletionResult or final text)
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

// TasksChangedMsg is sent by the bridge when the task store is mutated.
type TasksChangedMsg struct {
	Tasks []tasks.Task
}

// --- Picker messages ---

// FilePickerActivateMsg opens the file picker at the given '@' rune position.
type FilePickerActivateMsg struct {
	AtPos int
}

// FilePickerDismissMsg closes the file picker.
type FilePickerDismissMsg struct{}

// FilePickerQueryMsg updates the file picker filter query.
type FilePickerQueryMsg struct {
	Query string
}

// FilePickerSelectionMsg carries the selected file path from the file picker.
type FilePickerSelectionMsg struct {
	Path string
}

// CmdPickerActivateMsg opens the command picker at the given '/' rune position.
type CmdPickerActivateMsg struct {
	SlashPos int
}

// CmdPickerDismissMsg closes the command picker.
type CmdPickerDismissMsg struct{}

// CmdPickerQueryMsg updates the command picker filter query.
type CmdPickerQueryMsg struct {
	Query string
}

// CmdPickerSelectionMsg carries the selected command from the command picker.
type CmdPickerSelectionMsg struct {
	Command string
}

// --- ChatView messages ---

// ChatViewSetWidthMsg sets the render width.
type ChatViewSetWidthMsg struct {
	Width int
}

// ChatViewSetProcessingMsg sets the processing state.
type ChatViewSetProcessingMsg struct {
	Processing bool
}

// ChatViewAdvanceSpinnersMsg increments spinner frames.
type ChatViewAdvanceSpinnersMsg struct{}

// ChatViewClearMsg resets the chat view state.
type ChatViewClearMsg struct{}

// ChatViewFlushAllMsg ends all remaining agents and emits their summaries.
type ChatViewFlushAllMsg struct{}

// ChatViewMarkSentMsg records that content has been displayed.
type ChatViewMarkSentMsg struct{}

// ChatViewCommitUserMsg renders a user message into the viewport.
type ChatViewCommitUserMsg struct {
	Text string
}

// ChatViewAppendMsg appends arbitrary text (logo, help, errors) to the viewport.
type ChatViewAppendMsg struct {
	Content string
}

// ChatViewSetHeightMsg sets the viewport height.
type ChatViewSetHeightMsg struct {
	Height int
}

// --- TaskPanel messages ---

// TaskPanelSetWidthMsg sets the task panel render width.
type TaskPanelSetWidthMsg struct {
	Width int
}

// --- Input messages ---

// InputEnableMsg enables the input box and focuses the textarea.
type InputEnableMsg struct{}

// InputResetMsg resets the input box (clears text, re-enables, dismisses pickers).
type InputResetMsg struct{}

// InputSetWidthMsg sets the input box width.
type InputSetWidthMsg struct {
	Width int
}

// InputSetTokenCountMsg updates the token counter display.
type InputSetTokenCountMsg struct {
	TokenCount string
}
