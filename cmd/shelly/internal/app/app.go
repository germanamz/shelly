package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/askprompt"
	"github.com/germanamz/shelly/cmd/shelly/internal/bridge"
	"github.com/germanamz/shelly/cmd/shelly/internal/chatview"
	"github.com/germanamz/shelly/cmd/shelly/internal/configwizard"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/input"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/cmd/shelly/internal/taskpanel"
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

// askSet groups questions from a single agent for sequential presentation.
type askSet struct {
	agent     string
	questions []msgs.AskUserMsg
}

// AppModel is the root bubbletea v2 model.
type AppModel struct {
	ctx            context.Context
	sess           *engine.Session
	eng            *engine.Engine
	program        *tea.Program
	chatView       chatview.ChatViewModel
	inputBox       input.InputModel
	taskPanel      taskpanel.TaskPanelModel
	askSets        []askSet
	askActiveAgent string
	askActive      *askprompt.AskBatchModel
	askBatching    bool
	state          State
	cancelBridge   context.CancelFunc
	cancelSend     context.CancelFunc // cancels the current Send when Escape is pressed
	sendGeneration uint64
	tokenCount     string // formatted total session tokens for status bar
	cacheInfo      string // formatted cache hit ratio for status bar
	configPath     string
	shellyDir      string
	configWizard   *configwizard.WizardModel
	width          int
	height         int

	// InitialMessage, when set, is auto-submitted once the TUI is ready.
	InitialMessage string
}

// NewAppModel creates a new AppModel.
func NewAppModel(ctx context.Context, sess *engine.Session, eng *engine.Engine, historyPath, configPath, shellyDir string) AppModel {
	cv := chatview.New()
	// Append logo to viewport as initial content.
	cv, _ = cv.Update(msgs.ChatViewAppendMsg{Content: styles.DimStyle.Render(chatview.LogoArt)})
	return AppModel{
		ctx:        ctx,
		sess:       sess,
		eng:        eng,
		chatView:   cv,
		inputBox:   input.New(historyPath),
		taskPanel:  taskpanel.New(),
		state:      StateIdle,
		configPath: configPath,
		shellyDir:  shellyDir,
	}
}

// InputEnabled returns whether the input box is enabled.
// Implements tty.InputEnabler for use with tty.NewStaleEscapeFilter.
func (m AppModel) InputEnabled() bool {
	return m.inputBox.Enabled
}

func (m AppModel) Init() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return msgs.InitDrainMsg{}
	})
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// --- Config wizard overlay ---
	if m.configWizard != nil {
		return m.handleConfigWizard(msg)
	}

	switch msg := msg.(type) {
	// --- Global keys ---
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	// --- Window management ---
	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	// --- Lifecycle ---
	case msgs.InitDrainMsg:
		var cmd tea.Cmd
		m.inputBox, cmd = m.inputBox.Update(msgs.InputEnableMsg{})
		return m, cmd

	case msgs.ProgramReadyMsg:
		m.program = msg.Program
		m.cancelBridge = bridge.Start(m.ctx, msg.Program, m.sess.Chat(), m.eng.Events(), m.eng.Tasks(), m.sess.AgentName())
		if m.InitialMessage != "" {
			text := m.InitialMessage
			m.InitialMessage = ""
			return m.handleSubmit(msgs.InputSubmitMsg{Text: text})
		}
		return m, nil

	// --- User input ---
	case msgs.InputSubmitMsg:
		return m.handleSubmit(msg)

	// --- Chat view (agent activity) ---
	case msgs.ChatMessageMsg, msgs.AgentStartMsg, msgs.AgentEndMsg:
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		return m, cmd

	// --- Session completion ---
	case msgs.SendCompleteMsg:
		return m.handleSendComplete(msg)

	case msgs.CompactCompleteMsg:
		return m.handleCompactComplete(msg)

	// --- Ask-user coordination ---
	case msgs.AskUserMsg:
		return m.handleAskUser(msg)

	case msgs.AskBatchReadyMsg:
		return m.drainAskBatch()

	case msgs.AskBatchAnsweredMsg:
		return m.handleBatchAnswered(msg)

	case msgs.RespondErrorMsg:
		errLine := styles.ErrorBlockStyle.Width(m.width).Render(
			lipgloss.NewStyle().Foreground(styles.ColorError).Render("error responding: " + msg.Err.Error()),
		)
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
		return m, nil

	// --- Task panel ---
	case msgs.TasksChangedMsg:
		m.taskPanel, _ = m.taskPanel.Update(msg)
		return m, nil

	// --- Animation tick ---
	case msgs.TickMsg:
		if m.state == StateProcessing || m.chatView.HasActiveChains() || m.taskPanel.HasActiveTasks() {
			m.chatView, _ = m.chatView.Update(msgs.ChatViewAdvanceSpinnersMsg{})
			m.taskPanel, _ = m.taskPanel.Update(msg)
			m.updateTokenCounter()
			return m, tickCmd()
		}
		return m, nil

	// --- Forward mouse wheel to viewport ---
	case tea.MouseWheelMsg:
		m.chatView, _ = m.chatView.Update(msg)
		return m, nil
	}

	// --- Delegate to focused component ---
	if m.askActive != nil {
		updated, cmd := m.askActive.Update(msg)
		m.askActive = &updated
		cmds = append(cmds, cmd)
	} else {
		var cmd tea.Cmd
		m.inputBox, cmd = m.inputBox.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m AppModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	if m.configWizard != nil {
		return m.configWizard.View()
	}

	parts := []string{
		m.chatView.View(),
	}

	if m.askActive != nil {
		parts = append(parts, m.askActive.View())
	} else {
		parts = append(parts, m.inputBox.View())
	}

	parts = append(parts, m.statusBar())

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// --- Private helpers ---

func (m *AppModel) handleConfigWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case configwizard.WizardDoneMsg:
		m.configWizard = nil
		if msg.Saved {
			note := styles.DimStyle.Render("Config saved. Changes will apply on next restart.")
			m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + note + "\n"})
		} else {
			note := styles.DimStyle.Render("Settings dismissed.")
			m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + note + "\n"})
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 80)
		m.height = msg.Height
		updated, cmd := m.configWizard.Update(msg)
		wiz := updated.(configwizard.WizardModel)
		m.configWizard = &wiz
		return m, cmd
	default:
		updated, cmd := m.configWizard.Update(msg)
		wiz := updated.(configwizard.WizardModel)
		m.configWizard = &wiz
		return m, cmd
	}
}

func (m *AppModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = max(msg.Width, 80)
	m.height = msg.Height
	format.InitMarkdownRenderer(m.width - 4)
	m.inputBox, _ = m.inputBox.Update(msgs.InputSetWidthMsg{Width: m.width})
	m.chatView, _ = m.chatView.Update(msgs.ChatViewSetWidthMsg{Width: m.width})
	m.recalcViewportHeight()
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

	// Page Up / Page Down always scroll the viewport.
	if m.chatView.HandleScrollKey(msg) {
		return m, nil
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

	var cmd tea.Cmd
	m.inputBox, cmd = m.inputBox.Update(msg)
	return m, cmd
}

func (m *AppModel) handleSubmit(msg msgs.InputSubmitMsg) (tea.Model, tea.Cmd) {
	text := msg.Text

	if result := m.dispatchCommand(text); result.handled {
		return m, result.cmd
	}

	// Commit user message to viewport.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewCommitUserMsg{Text: text})
	m.chatView, _ = m.chatView.Update(msgs.ChatViewMarkSentMsg{})

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
	m.chatView, _ = m.chatView.Update(msgs.ChatViewSetProcessingMsg{Processing: true})
	sendStart := time.Now()

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

func (m *AppModel) handleSendComplete(msg msgs.SendCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.Generation != m.sendGeneration {
		return m, nil
	}
	m.state = StateIdle
	m.cancelSend = nil
	m.chatView, _ = m.chatView.Update(msgs.ChatViewSetProcessingMsg{Processing: false})
	m.updateTokenCounter()

	m.chatView, _ = m.chatView.Update(msgs.ChatViewFlushAllMsg{})

	if msg.Err != nil && m.ctx.Err() == nil {
		errLine := styles.ErrorBlockStyle.Width(m.width).Render(
			lipgloss.NewStyle().Foreground(styles.ColorError).Render("error: " + msg.Err.Error()),
		)
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
	}

	return m, nil
}

func (m *AppModel) handleCompactComplete(msg msgs.CompactCompleteMsg) (tea.Model, tea.Cmd) {
	m.state = StateIdle
	m.chatView, _ = m.chatView.Update(msgs.ChatViewSetProcessingMsg{Processing: false})
	m.updateTokenCounter()

	if msg.Err != nil && m.ctx.Err() == nil {
		errLine := styles.ErrorBlockStyle.Width(m.width).Render(
			lipgloss.NewStyle().Foreground(styles.ColorError).Render("compact error: " + msg.Err.Error()),
		)
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
		return m, nil
	}

	// Clear the viewport and show the compaction summary.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewClearMsg{})
	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: styles.DimStyle.Render(chatview.LogoArt)})
	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + styles.DimStyle.Render(
		fmt.Sprintf("⌘ /compact — %d messages compacted", msg.MessageCount),
	) + "\n"})
	if msg.Summary != "" {
		rendered := format.RenderMarkdown(msg.Summary)
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + rendered + "\n"})
	}

	return m, nil
}

func (m *AppModel) updateTokenCounter() {
	ur, ok := m.sess.Completer().(modeladapter.UsageReporter)
	if !ok {
		return
	}
	total := ur.UsageTracker().Total()
	totalTok := total.InputTokens + total.OutputTokens
	if totalTok > 0 {
		m.tokenCount = format.FmtTokens(totalTok)
	}
	if ratio := total.CacheSavings(); ratio > 0 {
		m.cacheInfo = fmt.Sprintf("cache %.0f%%", ratio*100)
	}
}

// statusBar renders the task panel and token counter below the input.
func (m AppModel) statusBar() string {
	var parts []string
	if panel := m.taskPanel.View(); panel != "" {
		parts = append(parts, panel)
	}
	var segments []string
	if label := m.sess.ProviderInfo().Label(); label != "" {
		segments = append(segments, label)
	}
	if m.tokenCount != "" {
		segments = append(segments, m.tokenCount+" tokens")
	}
	if m.cacheInfo != "" {
		segments = append(segments, m.cacheInfo)
	}
	if len(segments) > 0 {
		status := " " + strings.Join(segments, " | ")
		parts = append(parts, styles.StatusStyle.Render(status))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// recalcViewportHeight computes the available viewport height and sends it
// to the chatview. Call after resize or when the input height changes.
func (m *AppModel) recalcViewportHeight() {
	// Status bar: 1 line for token counter (always reserve), plus task panel lines.
	statusLines := 1
	if panel := m.taskPanel.View(); panel != "" {
		statusLines += strings.Count(panel, "\n") + 1
	}
	vpHeight := max(m.height-m.inputBox.ViewHeight()-statusLines, 3)
	m.chatView, _ = m.chatView.Update(msgs.ChatViewSetHeightMsg{Height: vpHeight})
}

func (m *AppModel) handleAskUser(msg msgs.AskUserMsg) (tea.Model, tea.Cmd) {
	// Group into per-agent sets.
	agentName := msg.Agent
	found := false
	for i := range m.askSets {
		if m.askSets[i].agent == agentName {
			m.askSets[i].questions = append(m.askSets[i].questions, msg)
			found = true
			break
		}
	}
	if !found {
		m.askSets = append(m.askSets, askSet{agent: agentName, questions: []msgs.AskUserMsg{msg}})
	}

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
	if len(m.askSets) == 0 {
		return m, nil
	}

	// Pop the first set.
	set := m.askSets[0]
	m.askSets = m.askSets[1:]

	batch := askprompt.NewAskBatch(set.questions, set.agent, m.width)
	m.askActive = &batch
	m.askActiveAgent = set.agent
	m.state = StateAskUser
	return m, nil
}

func (m *AppModel) handleBatchAnswered(msg msgs.AskBatchAnsweredMsg) (tea.Model, tea.Cmd) {
	if m.askActive == nil {
		return m, nil
	}
	agentName := m.askActiveAgent
	m.askActive = nil
	m.askActiveAgent = ""

	// Dismissed — cancel the agent instead of sending "[user dismissed]".
	if msg.Answers == nil {
		m.purgeAgentSets(agentName)

		isMain := agentName == m.sess.AgentName()
		if isMain {
			// Cancel the main send and return to idle.
			if m.cancelSend != nil {
				m.cancelSend()
				m.cancelSend = nil
			}
			m.state = StateIdle
			m.chatView, _ = m.chatView.Update(msgs.ChatViewSetProcessingMsg{Processing: false})
		} else {
			// Cancel the sub-agent's context.
			m.eng.CancelAgent(agentName)
			dismissLine := styles.DimStyle.Render(fmt.Sprintf("[dismissed questions from %s]", agentName))
			m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + dismissLine + "\n"})
		}

		// Drain next set if available.
		if len(m.askSets) > 0 {
			return m.drainAskBatch()
		}
		if !isMain && m.state == StateAskUser {
			m.state = StateProcessing
		}
		return m, nil
	}

	// Normal answer path — always deliver answers first.
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

	if len(m.askSets) > 0 {
		_, drainCmd := m.drainAskBatch()
		return m, tea.Batch(respondCmd, drainCmd)
	}

	if m.state == StateAskUser {
		m.state = StateProcessing
	}

	return m, respondCmd
}

// purgeAgentSets removes all queued ask sets for the given agent.
func (m *AppModel) purgeAgentSets(agentName string) {
	filtered := m.askSets[:0]
	for _, s := range m.askSets {
		if s.agent != agentName {
			filtered = append(filtered, s)
		}
	}
	m.askSets = filtered
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return msgs.TickMsg(t)
	})
}
