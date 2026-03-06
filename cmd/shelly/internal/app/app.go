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
	"github.com/germanamz/shelly/cmd/shelly/internal/menubar"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/cmd/shelly/internal/subagentpanel"
	"github.com/germanamz/shelly/cmd/shelly/internal/taskpanel"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
)

// State represents the application state machine.
type State int

const (
	StateIdle State = iota
	StateProcessing
	StateAskUser
)

// ActivePanel tracks which list panel is currently open.
type ActivePanel int

const (
	PanelNone ActivePanel = iota
	PanelSubAgents
	PanelTasks
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
	sessionCost    string // formatted cumulative USD cost for status bar
	menuBar        menubar.Model
	subAgentPanel  subagentpanel.Model
	activePanel    ActivePanel
	menuFocused    bool
	menuHintShown  bool // whether the transient ctrl+b hint has been shown
	menuHintActive bool // whether the transient hint is currently visible
	configPath     string
	shellyDir      string
	configWizard   *configwizard.WizardModel
	sessionPicker  input.SessionPickerModel
	agentUsage     map[string]AgentUsageInfo // per-agent usage data
	width          int
	height         int

	// InitialMessage, when set, is auto-submitted once the TUI is ready.
	InitialMessage string
}

// AgentUsageInfo holds per-agent usage data for the status bar.
type AgentUsageInfo struct {
	Usage usage.TokenCount
	Ended bool // true if agent has completed
}

// NewAppModel creates a new AppModel.
func NewAppModel(ctx context.Context, sess *engine.Session, eng *engine.Engine, historyPath, configPath, shellyDir string) AppModel {
	cv := chatview.New()
	// Append logo to viewport as initial content.
	cv, _ = cv.Update(msgs.ChatViewAppendMsg{Content: styles.DimStyle.Render(chatview.LogoArt)})
	return AppModel{
		ctx:           ctx,
		sess:          sess,
		eng:           eng,
		chatView:      cv,
		inputBox:      input.New(historyPath),
		taskPanel:     taskpanel.New(),
		menuBar:       menubar.New(),
		subAgentPanel: subagentpanel.New(),
		sessionPicker: input.NewSessionPicker(),
		state:         StateIdle,
		configPath:    configPath,
		shellyDir:     shellyDir,
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
	case msgs.ChatMessageMsg:
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		return m, cmd

	case msgs.AgentStartMsg:
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		m.onAgentStart(msg)
		return m, cmd

	case msgs.AgentEndMsg:
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		m.onAgentEnd(msg)
		return m, cmd

	case msgs.AgentUsageUpdateMsg:
		m.recordAgentUsage(msg.AgentID, msg.Usage, false)
		return m, nil

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
		m.taskPanel.SetTasks(msg.Tasks)
		m.onTasksChanged()
		return m, nil

	// --- Session picker ---
	case msgs.SessionPickerActivateMsg:
		m.sessionPicker.Width = m.width
		m.sessionPicker, _ = m.sessionPicker.Update(msg)
		return m, nil

	case msgs.SessionPickerDismissMsg:
		m.sessionPicker, _ = m.sessionPicker.Update(msg)
		return m, nil

	case msgs.SessionPickerSelectionMsg:
		cmd := m.executeResumeSession(msg.ID)
		return m, cmd

	// --- Animation tick ---
	case msgs.TickMsg:
		if m.state == StateProcessing || m.chatView.HasActiveChains() || m.taskPanel.HasActiveTasks() {
			m.chatView, _ = m.chatView.Update(msgs.ChatViewAdvanceSpinnersMsg{})
			m.taskPanel.AdvanceSpinner()
			if m.subAgentPanel.Active() {
				m.subAgentPanel.AdvanceSpinner()
			}
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

	// List panel (between viewport and menu bar).
	if m.activePanel != PanelNone {
		if panelView := m.activePanelView(); panelView != "" {
			parts = append(parts, panelView)
		}
	}

	// Breadcrumb (between panel and menu bar, only when viewing sub-agent).
	if bc := m.chatView.RenderBreadcrumb(); bc != "" {
		parts = append(parts, bc)
	}

	// Menu bar (between breadcrumb and input).
	if menuView := m.menuBar.View(); menuView != "" {
		parts = append(parts, menuView)
	}

	switch {
	case m.sessionPicker.Active:
		parts = append(parts, m.sessionPicker.View())
	case m.askActive != nil:
		parts = append(parts, m.askActive.View())
	default:
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
	m.menuBar.SetWidth(m.width)
	switch m.activePanel {
	case PanelSubAgents:
		m.resizeSubAgentPanel()
	case PanelTasks:
		m.resizeTaskPanel()
	}
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

	// Dismiss the transient menu bar hint on any keypress.
	if m.menuHintActive {
		m.menuHintActive = false
	}

	// Page Up / Page Down always scroll the viewport.
	if m.chatView.HandleScrollKey(msg) {
		return m, nil
	}

	// Forward to session picker if active.
	if m.sessionPicker.Active {
		var cmd tea.Cmd
		m.sessionPicker, cmd = m.sessionPicker.Update(msg)
		return m, cmd
	}

	// Forward to ask prompt if active.
	if m.askActive != nil {
		updated, cmd := m.askActive.Update(msg)
		m.askActive = &updated
		return m, cmd
	}

	// List panel active — forward Up/Down/Enter/Esc.
	if m.activePanel != PanelNone {
		return m.handlePanelKey(msg)
	}

	// Menu bar focused — forward Left/Right/Enter/Esc.
	if m.menuFocused {
		return m.handleMenuBarKey(msg)
	}

	// Ctrl+B toggles menu bar focus (only when menu is visible).
	if k.Code == 'b' && k.Mod&tea.ModCtrl != 0 && m.menuBar.Visible() {
		m.menuFocused = true
		m.menuBar.SetActive(true)
		return m, nil
	}

	// Escape priority: picker → panel → menu → agent view back → agent interrupt → no-op.
	if k.Code == tea.KeyEsc {
		if m.inputBox.PickerActive() {
			var cmd tea.Cmd
			m.inputBox, cmd = m.inputBox.Update(msg)
			return m, cmd
		}
		// Navigate back in agent view stack (only when input is empty).
		if m.chatView.ViewedAgent() != "" && m.inputBox.IsEmpty() {
			m.chatView, _ = m.chatView.Update(msgs.ChatViewNavigateBackMsg{})
			m.recalcViewportHeight()
			return m, nil
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

// handleMenuBarKey handles key events when the menu bar is focused.
func (m *AppModel) handleMenuBarKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()

	switch {
	case k.Code == tea.KeyLeft:
		m.menuBar.MoveLeft()
	case k.Code == tea.KeyRight:
		m.menuBar.MoveRight()
	case k.Code == tea.KeyEnter || k.Code == tea.KeySpace:
		if sel := m.menuBar.Select(); sel != nil {
			return m.handleMenuItemSelected(sel.ID)
		}
	case k.Code == tea.KeyEsc:
		m.menuFocused = false
		m.menuBar.SetActive(false)
	case k.Code == 'b' && k.Mod&tea.ModCtrl != 0:
		// Ctrl+B toggles back to input.
		m.menuFocused = false
		m.menuBar.SetActive(false)
	}
	return m, nil
}

// handlePanelKey handles key events when a list panel is open.
func (m *AppModel) handlePanelKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()

	switch m.activePanel {
	case PanelSubAgents:
		switch k.Code {
		case tea.KeyUp:
			m.subAgentPanel.MoveUp()
		case tea.KeyDown:
			m.subAgentPanel.MoveDown()
		case tea.KeyEnter:
			if sel := m.subAgentPanel.Select(); sel != nil {
				agentID := sel.AgentID
				m.closePanel()
				m.chatView, _ = m.chatView.Update(msgs.ChatViewFocusAgentMsg{AgentID: agentID})
				m.recalcViewportHeight()
			}
		case tea.KeyEsc:
			m.closePanel()
		}
	case PanelTasks:
		switch k.Code {
		case tea.KeyUp:
			m.taskPanel.MoveUp()
		case tea.KeyDown:
			m.taskPanel.MoveDown()
		case tea.KeyEsc:
			m.closePanel()
		}
	}
	return m, nil
}

// handleMenuItemSelected activates the panel for the given menu item ID.
func (m *AppModel) handleMenuItemSelected(id string) (tea.Model, tea.Cmd) {
	switch id {
	case subagentpanel.PanelID:
		if m.activePanel == PanelSubAgents {
			m.closePanel()
			return m, nil
		}
		m.closePanel() // close any other panel first
		m.activePanel = PanelSubAgents
		m.subAgentPanel.SetActive(true)
		m.subAgentPanel.Refresh(m.chatView)
		m.resizeSubAgentPanel()
		m.recalcViewportHeight()
	case taskpanel.PanelID:
		if m.activePanel == PanelTasks {
			m.closePanel()
			return m, nil
		}
		m.closePanel() // close any other panel first
		m.activePanel = PanelTasks
		m.taskPanel.SetActive(true)
		m.resizeTaskPanel()
		m.recalcViewportHeight()
	}
	// Menu bar loses focus when a panel opens.
	m.menuFocused = false
	m.menuBar.SetActive(false)
	return m, nil
}

// closePanel closes whatever panel is open and restores viewport height.
func (m *AppModel) closePanel() {
	switch m.activePanel {
	case PanelSubAgents:
		m.subAgentPanel.SetActive(false)
	case PanelTasks:
		m.taskPanel.SetActive(false)
	}
	m.activePanel = PanelNone
	m.recalcViewportHeight()
}

func (m *AppModel) handleSubmit(msg msgs.InputSubmitMsg) (tea.Model, tea.Cmd) {
	text := msg.Text

	if result := m.dispatchCommand(text); result.handled {
		return m, result.cmd
	}

	// Commit user message to viewport.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewCommitUserMsg{Text: text, Parts: msg.Parts})
	m.chatView, _ = m.chatView.Update(msgs.ChatViewMarkSentMsg{})

	// Build content parts: text first, then any attachments.
	parts := buildSendParts(text, msg.Parts)

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
			_, err := sess.SendParts(sendCtx, parts...)
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
		_, err := sess.SendParts(sendCtx, parts...)
		return msgs.SendCompleteMsg{Err: err, Duration: time.Since(sendStart), Generation: gen}
	}

	return m, tea.Batch(sendCmd, tickCmd())
}

// buildSendParts assembles content parts from text and attachment parts.
func buildSendParts(text string, extraParts []content.Part) []content.Part {
	var parts []content.Part
	if text != "" {
		parts = append(parts, content.Text{Text: text})
	}
	parts = append(parts, extraParts...)
	return parts
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
	info := m.sess.ProviderInfo()
	if pricing, ok := usage.LookupPricing(info.Kind, info.Model); ok {
		cost := usage.CalculateCost(total, pricing)
		if cost > 0 {
			m.sessionCost = format.FmtCost(cost)
		}
	}
}

// statusBar renders the token counter and keyboard hints below the input.
func (m AppModel) statusBar() string {
	var parts []string
	var segments []string
	if label := m.sess.ProviderInfo().Label(); label != "" {
		segments = append(segments, label)
	}
	if m.tokenCount != "" {
		segments = append(segments, m.tokenCount+" tokens")
	}
	if m.sessionCost != "" {
		segments = append(segments, m.sessionCost)
	}
	if m.cacheInfo != "" {
		segments = append(segments, m.cacheInfo)
	}
	// Keyboard hints.
	if hint := m.keyboardHint(); hint != "" {
		segments = append(segments, hint)
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

// keyboardHint returns context-sensitive keyboard hints for the status bar.
func (m AppModel) keyboardHint() string {
	switch {
	case m.activePanel == PanelTasks:
		return styles.DimStyle.Render("↑↓ scroll  esc close")
	case m.activePanel != PanelNone:
		return styles.DimStyle.Render("↑↓ navigate  ⏎ select  esc close")
	case m.menuFocused:
		return styles.DimStyle.Render("←→ navigate  ⏎ select  esc back")
	case m.chatView.ViewedAgent() != "":
		return styles.DimStyle.Render("esc back to parent")
	case m.menuHintActive:
		return styles.DimStyle.Render("ctrl+b to browse sub-agents")
	}
	return ""
}

// recalcViewportHeight computes the available viewport height and sends it
// to the chatview. Call after resize or when the input height changes.
func (m *AppModel) recalcViewportHeight() {
	// Status bar: 1 line for token counter (always reserve).
	statusLines := 1
	// Menu bar, sub-agent panel, task panel, and breadcrumb heights.
	extraLines := m.menuBar.Height() + m.subAgentPanel.Height() + m.taskPanel.Height() + m.chatView.HeaderHeight()
	vpHeight := max(m.height-m.inputBox.ViewHeight()-statusLines-extraLines, 3)
	m.chatView, _ = m.chatView.Update(msgs.ChatViewSetHeightMsg{Height: vpHeight})
}

// activePanelView returns the rendered view for the currently active panel.
func (m AppModel) activePanelView() string {
	switch m.activePanel {
	case PanelSubAgents:
		return m.subAgentPanel.View()
	case PanelTasks:
		return m.taskPanel.View()
	default:
		return ""
	}
}

// onAgentStart handles menu bar badge updates when a sub-agent starts.
func (m *AppModel) onAgentStart(msg msgs.AgentStartMsg) {
	if msg.Parent == "" {
		return // top-level agent, not a sub-agent
	}
	agents := m.chatView.SubAgents()
	badge := len(agents)

	// Ensure menu bar is visible and has the "Subagents" item.
	if !m.menuBar.Visible() {
		m.menuBar.SetVisible(true)
		m.menuBar.SetWidth(m.width)
		// Show transient hint on first appearance.
		if !m.menuHintShown {
			m.menuHintShown = true
			m.menuHintActive = true
		}
		m.recalcViewportHeight()
	}
	m.menuBar.AddOrUpdateItem(menubar.Item{
		ID:    subagentpanel.PanelID,
		Label: "Subagents",
		Badge: badge,
	})

	// Refresh the panel list if it's currently open.
	if m.activePanel == PanelSubAgents {
		m.subAgentPanel.Refresh(m.chatView)
	}
}

// recordAgentUsage stores a per-agent usage snapshot.
func (m *AppModel) recordAgentUsage(agentID string, u usage.TokenCount, ended bool) {
	if m.agentUsage == nil {
		m.agentUsage = make(map[string]AgentUsageInfo)
	}
	m.agentUsage[agentID] = AgentUsageInfo{Usage: u, Ended: ended}
}

// onAgentEnd handles menu bar badge updates when a sub-agent ends.
func (m *AppModel) onAgentEnd(msg msgs.AgentEndMsg) {
	if msg.Parent == "" {
		return
	}

	if msg.Usage != nil {
		m.recordAgentUsage(msg.Agent, *msg.Usage, true)
	}

	agents := m.chatView.SubAgents()
	badge := len(agents)

	m.menuBar.AddOrUpdateItem(menubar.Item{
		ID:    subagentpanel.PanelID,
		Label: "Subagents",
		Badge: badge,
	})

	// Refresh the panel list if it's currently open.
	if m.activePanel == PanelSubAgents {
		m.subAgentPanel.Refresh(m.chatView)
	}
}

// resizeSubAgentPanel computes and sets the panel size based on current items.
func (m *AppModel) resizeSubAgentPanel() {
	agents := m.chatView.SubAgents()
	// Panel height: min(items + 2 borders, 12), or 3 for empty state.
	h := len(agents) + 2
	if len(agents) == 0 {
		h = 3
	}
	if h > 12 {
		h = 12
	}
	m.subAgentPanel.SetSize(m.width, h)
}

// resizeTaskPanel computes and sets the panel size based on current task count.
func (m *AppModel) resizeTaskPanel() {
	count := len(m.taskPanel.Tasks())
	// Panel height: min(items + 2 borders, 12), or 3 for empty state.
	h := count + 2
	if count == 0 {
		h = 3
	}
	if h > 12 {
		h = 12
	}
	m.taskPanel.SetSize(m.width, h)
}

// onTasksChanged handles menu bar badge updates and panel refresh when tasks change.
func (m *AppModel) onTasksChanged() {
	badge := m.taskPanel.ActiveTaskCount()

	// Lazy item creation: add "Tasks" item on first TasksChangedMsg.
	if !m.menuBar.Visible() {
		m.menuBar.SetVisible(true)
		m.menuBar.SetWidth(m.width)
		m.recalcViewportHeight()
	}
	m.menuBar.AddOrUpdateItem(menubar.Item{
		ID:    taskpanel.PanelID,
		Label: "Tasks",
		Badge: badge,
	})

	// Refresh the panel if it's currently open.
	if m.activePanel == PanelTasks {
		m.resizeTaskPanel()
		m.recalcViewportHeight()
	}
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
