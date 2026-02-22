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
	height       int
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
		m.chatView.addMessage(msg.msg)
		m.recalcLayout()
		return m, nil

	case agentStartMsg:
		m.chatView.getOrCreateChain(msg.agent)
		m.recalcLayout()
		return m, nil

	case agentEndMsg:
		m.chatView.endAgent(msg.agent)
		m.recalcLayout()
		return m, nil

	case sendCompleteMsg:
		m.statusBar.duration = msg.duration
		m.state = stateIdle
		focusCmd := m.inputBox.enable()
		m.chatView.setProcessing(false)
		if msg.err != nil && m.ctx.Err() == nil {
			m.chatView.blocks = append(m.chatView.blocks, chatBlock{
				content: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("error: " + msg.err.Error()),
				kind:    "error",
			})
			m.chatView.updateViewport()
		}
		m.recalcLayout()
		return m, focusCmd

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
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	chatSection := m.chatView.View()

	var inputSection string
	if m.askActive != nil {
		inputSection = m.askActive.View()
	} else {
		inputSection = m.inputBox.View()
	}

	statusSection := m.statusBar.View()

	return lipgloss.JoinVertical(lipgloss.Left,
		chatSection,
		inputSection,
		statusSection,
	)
}

func (m *appModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	initMarkdownRenderer(m.width - 4)
	m.inputBox.setWidth(m.width)
	m.recalcLayout()

	var cmd tea.Cmd
	m.chatView, cmd = m.chatView.Update(msg)
	return m, cmd
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
		m.chatView.blocks = append(m.chatView.blocks, chatBlock{
			content: helpText(),
		})
		m.chatView.updateViewport()
		m.recalcLayout()
		return m, nil
	}

	// Add user message to the chat view.
	m.chatView.blocks = append(m.chatView.blocks, chatBlock{
		content: userStyle.Render("you> ") + text,
		kind:    "user",
	})
	m.chatView.updateViewport()

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

	return m, tea.Batch(sendCmd, tickCmd())
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
	m.recalcLayout()
	return m, nil
}

func (m *appModel) handleBatchAnswered(msg askBatchAnsweredMsg) (tea.Model, tea.Cmd) {
	for _, ans := range msg.answers {
		if err := m.sess.Respond(ans.questionID, ans.response); err != nil {
			m.chatView.blocks = append(m.chatView.blocks, chatBlock{
				content: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("error responding: " + err.Error()),
				kind:    "error",
			})
			m.chatView.updateViewport()
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
	m.recalcLayout()
	return m, nil
}

func (m *appModel) recalcLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// Status bar = 1 line, input box ~ border(2) + content lines + optional picker.
	statusHeight := 1
	var inputHeight int
	if m.askActive != nil {
		inputHeight = lipgloss.Height(m.askActive.View())
	} else {
		inputHeight = m.inputBox.totalHeight()
	}
	chatHeight := max(m.height-inputHeight-statusHeight, 1)
	m.chatView.setSize(m.width, chatHeight)
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
