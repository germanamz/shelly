package main

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/germanamz/shelly/pkg/engine"
)

// appState represents the application state machine.
type appState int

const (
	stateIdle appState = iota
	stateProcessing
	stateAskUser
)

// appModel is the root bubbletea model.
type appModel struct {
	ctx          context.Context
	sess         *engine.Session
	events       *engine.EventBus
	verbose      bool
	chatView     chatViewModel
	inputBox     inputModel
	statusBar    statusBarModel
	askQueue     []askUserMsg
	askActive    *askBatchModel
	askBatching  bool
	state        appState
	cancelBridge context.CancelFunc
	width        int
	sendStart    time.Time
}

func newAppModel(ctx context.Context, sess *engine.Session, events *engine.EventBus, verbose bool) appModel {
	return appModel{
		ctx:       ctx,
		sess:      sess,
		events:    events,
		verbose:   verbose,
		chatView:  newChatView(verbose),
		inputBox:  newInput(),
		statusBar: newStatusBar(sess.Completer()),
		state:     stateIdle,
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

	case tea.KeyMsg:
		return m.handleKey(msg)

	case initDrainMsg:
		cmd := m.inputBox.enable()
		return m, cmd

	case programReadyMsg:
		m.cancelBridge = startBridge(m.ctx, msg.program, m.sess.Chat(), m.events)
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
		m.chatView.startAgent(msg.agent, msg.prefix)
		return m, nil

	case agentEndMsg:
		cmd := m.chatView.endAgent(msg.agent)
		return m, cmd

	case sendCompleteMsg:
		m.statusBar.duration = msg.duration
		m.state = stateIdle
		focusCmd := m.inputBox.enable()
		m.chatView.setProcessing(false)

		var cmds []tea.Cmd
		cmds = append(cmds, focusCmd)

		if msg.err != nil && m.ctx.Err() == nil {
			errLine := errorBlockStyle.Render(
				lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("error: " + msg.err.Error()),
			)
			cmds = append(cmds, tea.Println(errLine))
		}

		return m, tea.Batch(cmds...)

	case askUserMsg:
		return m.handleAskUser(msg)

	case askBatchReadyMsg:
		return m.drainAskBatch()

	case askBatchAnsweredMsg:
		return m.handleBatchAnswered(msg)

	case tickMsg:
		if m.state == stateProcessing || m.chatView.hasActiveChains() {
			m.chatView.advanceSpinners()
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
	case m.state == stateIdle:
		var cmd tea.Cmd
		m.inputBox, cmd = m.inputBox.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m appModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var parts []string

	// Active reasoning chains (live content).
	chainView := m.chatView.View()
	if chainView != "" {
		parts = append(parts, chainView)
	}

	// Input area or ask prompt.
	if m.askActive != nil {
		parts = append(parts, m.askActive.View())
	} else {
		parts = append(parts, m.inputBox.View())
	}

	// Status bar.
	statusSection := m.statusBar.View()
	if statusSection != "" {
		parts = append(parts, statusSection)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *appModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.chatView.width = m.width
	initMarkdownRenderer(m.width - 4)
	m.inputBox.setWidth(m.width)
	return m, nil
}

func (m *appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		if m.cancelBridge != nil {
			m.cancelBridge()
		}
		return m, tea.Quit
	}

	// Forward to ask prompt if active.
	if m.askActive != nil {
		updated, cmd := m.askActive.Update(msg)
		m.askActive = &updated
		return m, cmd
	}

	// Forward to input box when idle.
	if m.state == stateIdle {
		var cmd tea.Cmd
		m.inputBox, cmd = m.inputBox.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *appModel) handleSubmit(msg inputSubmitMsg) (tea.Model, tea.Cmd) {
	text := msg.text

	if text == "/quit" || text == "/exit" {
		if m.cancelBridge != nil {
			m.cancelBridge()
		}
		return m, tea.Quit
	}

	if text == "/help" {
		return m, tea.Println(helpText())
	}

	// Print user message to terminal scrollback with a leading blank line.
	userLine := "\n" + renderUserMessage(text)
	printCmd := tea.Println(userLine)

	m.state = stateProcessing
	m.inputBox.disable()
	m.chatView.setProcessing(true)
	m.sendStart = time.Now()

	// Start the send in a background goroutine via tea.Cmd.
	sess := m.sess
	ctx := m.ctx
	sendCmd := func() tea.Msg {
		_, err := sess.Send(ctx, text)
		return sendCompleteMsg{err: err, duration: time.Since(m.sendStart)}
	}

	return m, tea.Batch(printCmd, sendCmd, tickCmd())
}

func (m *appModel) handleAskUser(msg askUserMsg) (tea.Model, tea.Cmd) {
	m.askQueue = append(m.askQueue, msg)

	// If a batch is already displayed, don't start another.
	if m.askActive != nil {
		return m, nil
	}

	// Start batching window if not already batching.
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
	var cmds []tea.Cmd
	for _, ans := range msg.answers {
		if err := m.sess.Respond(ans.questionID, ans.response); err != nil {
			errLine := errorBlockStyle.Render(
				lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("error responding: " + err.Error()),
			)
			cmds = append(cmds, tea.Println(errLine))
		}
	}

	m.askActive = nil

	// Check if more questions arrived during answering.
	if len(m.askQueue) > 0 {
		return m.drainAskBatch()
	}

	// Return to previous state.
	if m.state == stateAskUser {
		m.state = stateProcessing
		m.inputBox.disable()
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func helpText() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"Commands:\n" +
			"  /help          Show this help message\n" +
			"  /quit          Exit the chat\n\n" +
			"Shortcuts:\n" +
			"  Enter          Submit message\n" +
			"  Alt+Enter      New line\n" +
			"  Ctrl+C         Exit\n" +
			"  @              File picker",
	)
}
