package main

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

// appState represents the application state machine.
type appState int

const (
	stateIdle appState = iota
	stateProcessing
	stateAskUser
)

// appModel is the root bubbletea v2 model.
type appModel struct {
	ctx          context.Context
	sess         *engine.Session
	eng          *engine.Engine
	events       *engine.EventBus
	program      *tea.Program
	chatView     chatViewModel
	inputBox     inputModel
	askQueue     []askUserMsg
	askActive    *askBatchModel
	askBatching  bool
	state        appState
	cancelBridge context.CancelFunc
	cancelSend   context.CancelFunc // cancels the current Send when Escape is pressed
	width        int
	height       int
	sendStart    time.Time
}

func newAppModel(ctx context.Context, sess *engine.Session, eng *engine.Engine) appModel {
	return appModel{
		ctx:      ctx,
		sess:     sess,
		eng:      eng,
		events:   eng.Events(),
		chatView: newChatView(),
		inputBox: newInput(),
		state:    stateIdle,
	}
}

func (m appModel) Init() tea.Cmd {
	// Delay focusing the input so that stale terminal escape-sequence
	// responses (e.g. OSC 11 background-color) are drained first.
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return initDrainMsg{}
	})
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case initDrainMsg:
		m.inputBox.enabled = true
		cmd := m.inputBox.enable()
		return m, cmd

	case programReadyMsg:
		m.program = msg.program
		m.cancelBridge = startBridge(m.ctx, msg.program, m.sess.Chat(), m.eng.Events())
		return m, nil

	case filePickerEntriesMsg:
		m.inputBox.filePicker.setEntries(msg.entries)
		return m, nil

	case inputSubmitMsg:
		return m.handleSubmit(msg)

	case chatMessageMsg:
		cmd := m.chatView.addMessage(msg.msg)
		return m, cmd

	case agentStartMsg:
		m.chatView.startAgent(msg.agent, msg.prefix, msg.parent)
		return m, nil

	case agentEndMsg:
		m.chatView.endAgent(msg.agent, msg.parent)
		return m, nil

	case sendCompleteMsg:
		m.state = stateIdle
		m.cancelSend = nil
		m.chatView.setProcessing(false)
		m.updateTokenCounter()

		if msg.err != nil && m.ctx.Err() == nil {
			errLine := errorBlockStyle.Render(
				lipgloss.NewStyle().Foreground(colorError).Render("error: " + msg.err.Error()),
			)
			m.chatView.committed.WriteString("\n" + errLine + "\n")
		}

		return m, nil

	case askUserMsg:
		return m.handleAskUser(msg)

	case askBatchReadyMsg:
		return m.drainAskBatch()

	case askBatchAnsweredMsg:
		return m.handleBatchAnswered(msg)

	case respondErrorMsg:
		errLine := errorBlockStyle.Render(
			lipgloss.NewStyle().Foreground(colorError).Render("error responding: " + msg.err.Error()),
		)
		m.chatView.committed.WriteString("\n" + errLine + "\n")
		return m, nil

	case tickMsg:
		if m.state == stateProcessing || m.chatView.hasActiveChains() {
			m.chatView.advanceSpinners()
			m.updateTokenCounter()
			return m, tickCmd()
		}
		return m, nil
	}

	// Delegate to active sub-component.
	switch {
	case m.askActive != nil:
		updated, cmd := m.askActive.Update(msg)
		m.askActive = &updated
		return m, cmd
	default:
		var cmd tea.Cmd
		m.inputBox, cmd = m.inputBox.Update(msg)
		return m, cmd
	}
}

func (m appModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var parts []string

	// Chat viewport.
	chatContent := m.chatView.View()
	if chatContent != "" {
		parts = append(parts, chatContent)
	}

	// Input area or ask prompt. Always show the input box (disabled state
	// renders a dimmed border) so the user sees the full UI during init.
	if m.askActive != nil {
		parts = append(parts, m.askActive.View())
	} else {
		parts = append(parts, m.inputBox.View())
	}

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (m *appModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	initMarkdownRenderer(m.width - 4)
	m.inputBox.setWidth(m.width)

	// Calculate chat area height: total minus input area.
	inputHeight := m.inputBox.viewHeight()
	chatHeight := max(m.height-inputHeight, 4)
	m.chatView.setSize(m.width, chatHeight)

	return m, nil
}

func (m *appModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()

	// Ctrl+C always quits.
	if k.Code == 'c' && k.Mod&tea.ModCtrl != 0 {
		if m.cancelBridge != nil {
			m.cancelBridge()
		}
		return m, func() tea.Msg { return tea.QuitMsg{} }
	}

	// Forward to ask prompt if active.
	if m.askActive != nil {
		updated, cmd := m.askActive.Update(msg)
		m.askActive = &updated
		return m, cmd
	}

	// Escape priority: picker → agent interrupt → no-op.
	if k.Code == tea.KeyEsc {
		if m.inputBox.pickerActive() {
			var cmd tea.Cmd
			m.inputBox, cmd = m.inputBox.Update(msg)
			return m, cmd
		}
		if m.state == stateProcessing && m.cancelSend != nil {
			m.cancelSend()
			m.cancelSend = nil
			return m, nil
		}
		return m, nil
	}

	// Input is always active.
	var cmd tea.Cmd
	m.inputBox, cmd = m.inputBox.Update(msg)
	return m, cmd
}

func (m *appModel) handleSubmit(msg inputSubmitMsg) (tea.Model, tea.Cmd) {
	text := msg.text

	if text == "/quit" || text == "/exit" {
		if m.cancelBridge != nil {
			m.cancelBridge()
		}
		return m, func() tea.Msg { return tea.QuitMsg{} }
	}

	if text == "/help" {
		m.chatView.committed.WriteString("\n" + helpText() + "\n")
		return m, nil
	}

	if text == "/clear" {
		if m.cancelBridge != nil {
			m.cancelBridge()
		}
		m.eng.RemoveSession(m.sess.ID())
		newSess, err := m.eng.NewSession("")
		if err != nil {
			m.chatView.committed.WriteString("\n" + errorBlockStyle.Render("Error: "+err.Error()) + "\n")
			return m, nil
		}
		m.sess = newSess
		m.chatView.Clear()
		m.inputBox.Reset()
		m.cancelBridge = startBridge(m.ctx, m.program, m.sess.Chat(), m.eng.Events())
		m.state = stateIdle
		return m, nil
	}

	// Commit user message to viewport.
	m.chatView.commitUserMessage(text)
	m.chatView.markMessageSent()

	if m.state == stateProcessing {
		// Queue message while processing.
		sess := m.sess
		ctx := m.ctx
		sendStart := time.Now()
		return m, func() tea.Msg {
			_, err := sess.Send(ctx, text)
			return sendCompleteMsg{err: err, duration: time.Since(sendStart)}
		}
	}

	m.state = stateProcessing
	m.chatView.setProcessing(true)
	sendStart := time.Now()
	m.sendStart = sendStart

	// Create a cancellable context for this Send call.
	sendCtx, cancelSend := context.WithCancel(m.ctx)
	m.cancelSend = cancelSend

	sess := m.sess
	sendCmd := func() tea.Msg {
		_, err := sess.Send(sendCtx, text)
		return sendCompleteMsg{err: err, duration: time.Since(sendStart)}
	}

	return m, tea.Batch(sendCmd, tickCmd())
}

// updateTokenCounter refreshes the token count displayed below the input.
func (m *appModel) updateTokenCounter() {
	ur, ok := m.sess.Completer().(modeladapter.UsageReporter)
	if !ok {
		return
	}
	total := ur.UsageTracker().Total()
	totalTok := total.InputTokens + total.OutputTokens
	if totalTok > 0 {
		m.inputBox.tokenCount = fmtTokens(totalTok)
	}
}

func (m *appModel) handleAskUser(msg askUserMsg) (tea.Model, tea.Cmd) {
	m.askQueue = append(m.askQueue, msg)

	if m.askActive != nil {
		return m, nil
	}

	if !m.askBatching {
		m.askBatching = true
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return askBatchReadyMsg{}
		})
	}

	return m, nil
}

func (m *appModel) drainAskBatch() (tea.Model, tea.Cmd) {
	m.askBatching = false
	if len(m.askQueue) == 0 {
		return m, nil
	}

	batch := newAskBatch(m.askQueue, m.width)
	m.askActive = &batch
	m.askQueue = nil
	m.state = stateAskUser
	return m, nil
}

func (m *appModel) handleBatchAnswered(msg askBatchAnsweredMsg) (tea.Model, tea.Cmd) {
	m.askActive = nil

	if len(m.askQueue) > 0 {
		return m.drainAskBatch()
	}

	if m.state == stateAskUser {
		m.state = stateProcessing
	}

	if msg.answers == nil {
		sess := m.sess
		queue := m.askQueue
		return m, func() tea.Msg {
			for _, q := range queue {
				_ = sess.Respond(q.question.ID, "[user dismissed the question]")
			}
			return nil
		}
	}

	sess := m.sess
	answers := msg.answers
	respondCmd := func() tea.Msg {
		for _, ans := range answers {
			if err := sess.Respond(ans.questionID, ans.response); err != nil {
				return respondErrorMsg{err: err}
			}
		}
		return nil
	}

	return m, respondCmd
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// viewHeight returns the height of the input box area.
func (m inputModel) viewHeight() int {
	// Border (2) + textarea lines + token counter (1).
	lines := m.visualLineCount()
	h := min(max(lines, inputMinHeight), inputMaxHeight)
	return h + 2 + 1
}

func helpText() string {
	return lipgloss.NewStyle().Foreground(colorMuted).Render(
		fmt.Sprintf("Commands:\n" +
			"  /help          Show this help message\n" +
			"  /clear         Clear the chat and start a new session\n" +
			"  /quit          Exit the chat\n\n" +
			"Shortcuts:\n" +
			"  Enter          Submit message\n" +
			"  Shift+Enter    New line\n" +
			"  Alt+Enter      New line\n" +
			"  Escape         Interrupt agent / dismiss picker\n" +
			"  Ctrl+C         Exit\n" +
			"  @              File picker\n" +
			"  /              Command picker"),
	)
}
