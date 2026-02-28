package app

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/askprompt"
	"github.com/germanamz/shelly/cmd/shelly/internal/bridge"
	"github.com/germanamz/shelly/cmd/shelly/internal/chatview"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/input"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/modeladapter"
)

// State represents the application state machine.
type State int

const (
	StateIdle State = iota
	StateProcessing
	StateAskUser
)

// AppModel is the root bubbletea v2 model.
type AppModel struct {
	ctx            context.Context
	sess           *engine.Session
	eng            *engine.Engine
	events         *engine.EventBus
	program        *tea.Program
	chatView       chatview.ChatViewModel
	inputBox       input.InputModel
	askQueue       []msgs.AskUserMsg
	askActive      *askprompt.AskBatchModel
	askBatching    bool
	state          State
	cancelBridge   context.CancelFunc
	cancelSend     context.CancelFunc // cancels the current Send when Escape is pressed
	sendGeneration uint64
	width          int
	height         int
	sendStart      time.Time
}

// NewAppModel creates a new AppModel.
func NewAppModel(ctx context.Context, sess *engine.Session, eng *engine.Engine) AppModel {
	return AppModel{
		ctx:      ctx,
		sess:     sess,
		eng:      eng,
		events:   eng.Events(),
		chatView: chatview.New(),
		inputBox: input.New(),
		state:    StateIdle,
	}
}

// InputEnabled returns whether the input box is enabled.
// Used by tty.NewStaleEscapeFilter closure.
func (m AppModel) InputEnabled() bool {
	return m.inputBox.Enabled
}

func (m AppModel) Init() tea.Cmd {
	// Delay focusing the input so that stale terminal escape-sequence
	// responses (e.g. OSC 11 background-color) are drained first.
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return msgs.InitDrainMsg{}
	})
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case msgs.InitDrainMsg:
		m.inputBox.Enabled = true
		cmd := m.inputBox.Enable()
		return m, cmd

	case msgs.ProgramReadyMsg:
		m.program = msg.Program
		m.cancelBridge = bridge.Start(m.ctx, msg.Program, m.sess.Chat(), m.eng.Events(), m.sess.AgentName())
		return m, nil

	case msgs.FilePickerEntriesMsg:
		m.inputBox.FilePicker.SetEntries(msg.Entries)
		return m, nil

	case msgs.InputSubmitMsg:
		return m.handleSubmit(msg)

	case msgs.ChatMessageMsg:
		cmd := m.chatView.AddMessage(msg.Msg)
		return m, cmd

	case msgs.AgentStartMsg:
		m.chatView.StartAgent(msg.Agent, msg.Prefix, msg.Parent)
		return m, nil

	case msgs.AgentEndMsg:
		m.chatView.EndAgent(msg.Agent, msg.Parent)
		return m, nil

	case msgs.SendCompleteMsg:
		// Ignore stale completions from cancelled sends.
		if msg.Generation != m.sendGeneration {
			return m, nil
		}
		m.state = StateIdle
		m.cancelSend = nil
		m.chatView.SetProcessing(false)
		m.updateTokenCounter()

		if msg.Err != nil && m.ctx.Err() == nil {
			errLine := styles.ErrorBlockStyle.Render(
				lipgloss.NewStyle().Foreground(styles.ColorError).Render("error: " + msg.Err.Error()),
			)
			m.chatView.Committed.WriteString("\n" + errLine + "\n")
		}

		return m, nil

	case msgs.AskUserMsg:
		return m.handleAskUser(msg)

	case msgs.AskBatchReadyMsg:
		return m.drainAskBatch()

	case msgs.AskBatchAnsweredMsg:
		return m.handleBatchAnswered(msg)

	case msgs.RespondErrorMsg:
		errLine := styles.ErrorBlockStyle.Render(
			lipgloss.NewStyle().Foreground(styles.ColorError).Render("error responding: " + msg.Err.Error()),
		)
		m.chatView.Committed.WriteString("\n" + errLine + "\n")
		return m, nil

	case msgs.TickMsg:
		if m.state == StateProcessing || m.chatView.HasActiveChains() {
			m.chatView.AdvanceSpinners()
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

func (m AppModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var parts []string

	// Chat viewport.
	chatContent := m.chatView.View()
	if chatContent != "" {
		parts = append(parts, chatContent)
	}

	// Input area or ask prompt.
	if m.askActive != nil {
		parts = append(parts, m.askActive.View())
	} else {
		parts = append(parts, m.inputBox.View())
	}

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (m *AppModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	format.InitMarkdownRenderer(m.width - 4)
	m.inputBox.SetWidth(m.width)

	// Calculate chat area height: total minus input area.
	inputHeight := m.inputBox.ViewHeight()
	chatHeight := max(m.height-inputHeight, 4)
	m.chatView.SetSize(m.width, chatHeight)

	return m, nil
}

func (m *AppModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		if m.inputBox.PickerActive() {
			var cmd tea.Cmd
			m.inputBox, cmd = m.inputBox.Update(msg)
			return m, cmd
		}
		if m.state == StateProcessing && m.cancelSend != nil {
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

func (m *AppModel) handleSubmit(msg msgs.InputSubmitMsg) (tea.Model, tea.Cmd) {
	text := msg.Text

	if text == "/quit" || text == "/exit" {
		if m.cancelBridge != nil {
			m.cancelBridge()
		}
		return m, func() tea.Msg { return tea.QuitMsg{} }
	}

	if text == "/help" {
		m.chatView.Committed.WriteString("\n" + helpText() + "\n")
		return m, nil
	}

	if text == "/clear" {
		if m.cancelSend != nil {
			m.cancelSend()
			m.cancelSend = nil
		}
		m.state = StateIdle
		if m.cancelBridge != nil {
			m.cancelBridge()
			m.cancelBridge = nil
		}
		m.eng.RemoveSession(m.sess.ID())
		newSess, err := m.eng.NewSession("")
		if err != nil {
			m.chatView.Committed.WriteString("\n" + styles.ErrorBlockStyle.Render("Error: "+err.Error()) + "\n")
			return m, nil
		}
		m.sess = newSess
		m.chatView.Clear()
		m.inputBox.Reset()
		m.cancelBridge = bridge.Start(m.ctx, m.program, m.sess.Chat(), m.eng.Events(), m.sess.AgentName())
		m.state = StateIdle
		return m, nil
	}

	// Commit user message to viewport.
	m.chatView.CommitUserMessage(text)
	m.chatView.MarkMessageSent()

	if m.state == StateProcessing {
		if m.cancelSend != nil {
			m.cancelSend()
		}
		m.sendGeneration++
		gen := m.sendGeneration
		sendCtx, cancelSend := context.WithCancel(m.ctx)
		m.cancelSend = cancelSend
		sess := m.sess
		sendStart := time.Now()
		return m, func() tea.Msg {
			_, err := sess.Send(sendCtx, text)
			return msgs.SendCompleteMsg{Err: err, Duration: time.Since(sendStart), Generation: gen}
		}
	}

	m.state = StateProcessing
	m.chatView.SetProcessing(true)
	sendStart := time.Now()
	m.sendStart = sendStart

	// Create a cancellable context for this Send call.
	m.sendGeneration++
	gen := m.sendGeneration
	sendCtx, cancelSend := context.WithCancel(m.ctx)
	m.cancelSend = cancelSend

	sess := m.sess
	sendCmd := func() tea.Msg {
		_, err := sess.Send(sendCtx, text)
		return msgs.SendCompleteMsg{Err: err, Duration: time.Since(sendStart), Generation: gen}
	}

	return m, tea.Batch(sendCmd, tickCmd())
}

// updateTokenCounter refreshes the token count displayed below the input.
func (m *AppModel) updateTokenCounter() {
	ur, ok := m.sess.Completer().(modeladapter.UsageReporter)
	if !ok {
		return
	}
	total := ur.UsageTracker().Total()
	totalTok := total.InputTokens + total.OutputTokens
	if totalTok > 0 {
		m.inputBox.TokenCount = format.FmtTokens(totalTok)
	}
}

func (m *AppModel) handleAskUser(msg msgs.AskUserMsg) (tea.Model, tea.Cmd) {
	m.askQueue = append(m.askQueue, msg)

	if m.askActive != nil {
		return m, nil
	}

	if !m.askBatching {
		m.askBatching = true
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return msgs.AskBatchReadyMsg{}
		})
	}

	return m, nil
}

func (m *AppModel) drainAskBatch() (tea.Model, tea.Cmd) {
	m.askBatching = false
	if len(m.askQueue) == 0 {
		return m, nil
	}

	batch := askprompt.NewAskBatch(m.askQueue, m.width)
	m.askActive = &batch
	m.askQueue = nil
	m.state = StateAskUser
	return m, nil
}

func (m *AppModel) handleBatchAnswered(msg msgs.AskBatchAnsweredMsg) (tea.Model, tea.Cmd) {
	if m.askActive == nil {
		return m, nil
	}
	dismissed := m.askActive.Questions()
	m.askActive = nil

	if len(m.askQueue) > 0 {
		return m.drainAskBatch()
	}

	if m.state == StateAskUser {
		m.state = StateProcessing
	}

	if msg.Answers == nil {
		sess := m.sess
		return m, func() tea.Msg {
			for _, q := range dismissed {
				if err := sess.Respond(q.Question.ID, "[user dismissed the question]"); err != nil {
					return msgs.RespondErrorMsg{Err: err}
				}
			}
			return nil
		}
	}

	sess := m.sess
	answers := msg.Answers
	respondCmd := func() tea.Msg {
		for _, ans := range answers {
			if err := sess.Respond(ans.QuestionID, ans.Response); err != nil {
				return msgs.RespondErrorMsg{Err: err}
			}
		}
		return nil
	}

	return m, respondCmd
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return msgs.TickMsg(t)
	})
}

func helpText() string {
	return lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(
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
