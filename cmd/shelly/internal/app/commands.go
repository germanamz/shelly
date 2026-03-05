package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/bridge"
	"github.com/germanamz/shelly/cmd/shelly/internal/chatview"
	"github.com/germanamz/shelly/cmd/shelly/internal/configwizard"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/engine"
)

// commandResult holds the result of dispatching a slash command.
type commandResult struct {
	cmd     tea.Cmd
	handled bool
}

// dispatchCommand checks if text is a recognized slash command and handles it.
func (m *AppModel) dispatchCommand(text string) commandResult {
	switch text {
	case "/quit", "/exit":
		return commandResult{cmd: m.executeQuit(), handled: true}
	case "/help":
		m.executeHelp()
		return commandResult{handled: true}
	case "/clear":
		return commandResult{cmd: m.executeClear(), handled: true}
	case "/compact":
		return commandResult{cmd: m.executeCompact(), handled: true}
	case "/sessions":
		return commandResult{cmd: m.executeSessions(), handled: true}
	case "/settings":
		m.executeSettings()
		return commandResult{handled: true}
	}
	return commandResult{}
}

func (m *AppModel) executeQuit() tea.Cmd {
	if m.cancelBridge != nil {
		m.cancelBridge()
	}
	return func() tea.Msg { return tea.QuitMsg{} }
}

func (m *AppModel) executeHelp() {
	helpOutput := "\n" + styles.DimStyle.Render("⌘ /help") + "\n\n" + helpText() + "\n"
	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: helpOutput})
}

func (m *AppModel) executeClear() tea.Cmd {
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
		errLine := styles.ErrorBlockStyle.Width(m.width).Render("Error: " + err.Error())
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
		return nil
	}
	m.sess = newSess
	m.chatView, _ = m.chatView.Update(msgs.ChatViewClearMsg{})
	// Re-add the logo after clearing.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: styles.DimStyle.Render(chatview.LogoArt)})
	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + styles.DimStyle.Render("⌘ /clear") + "\n"})
	m.inputBox, _ = m.inputBox.Update(msgs.InputResetMsg{})
	m.tokenCount = ""
	m.cancelBridge = bridge.Start(m.ctx, m.program, m.sess.Chat(), m.eng.Events(), m.eng.Tasks(), m.sess.AgentName())
	m.state = StateIdle
	return nil
}

func (m *AppModel) executeCompact() tea.Cmd {
	if m.state == StateProcessing {
		errLine := styles.ErrorBlockStyle.Width(m.width).Render("Cannot compact while the agent is running.")
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
		return nil
	}

	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + styles.DimStyle.Render("⌘ /compact") + "\n"})
	m.state = StateProcessing
	m.chatView, _ = m.chatView.Update(msgs.ChatViewSetProcessingMsg{Processing: true})

	sess := m.sess
	ctx := m.ctx
	return tea.Batch(func() tea.Msg {
		result, err := sess.Compact(ctx)
		return msgs.CompactCompleteMsg{
			Err:          err,
			Summary:      result.Summary,
			MessageCount: result.MessageCount,
		}
	}, tickCmd())
}

func (m *AppModel) executeSessions() tea.Cmd {
	sessionList, err := m.eng.SessionStore().List()
	if err != nil {
		errLine := styles.ErrorBlockStyle.Width(m.width).Render("Error: " + err.Error())
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
		return nil
	}
	if len(sessionList) == 0 {
		note := styles.DimStyle.Render("No previous sessions found.")
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + note + "\n"})
		return nil
	}
	return func() tea.Msg {
		return msgs.SessionPickerActivateMsg{Sessions: sessionList}
	}
}

func (m *AppModel) executeResumeSession(id string) tea.Cmd {
	// Cancel current activity.
	if m.cancelSend != nil {
		m.cancelSend()
		m.cancelSend = nil
	}
	if m.cancelBridge != nil {
		m.cancelBridge()
		m.cancelBridge = nil
	}
	m.eng.RemoveSession(m.sess.ID())

	newSess, err := m.eng.ResumeSession(id)
	if err != nil {
		errLine := styles.ErrorBlockStyle.Width(m.width).Render("Error resuming session: " + err.Error())
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
		return nil
	}
	m.sess = newSess

	// Clear and render historical messages.
	m.chatView, _ = m.chatView.Update(msgs.ChatViewClearMsg{})
	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: styles.DimStyle.Render(chatview.LogoArt)})

	for _, msg := range newSess.Chat().Messages() {
		switch msg.Role {
		case role.User:
			text := msg.TextContent()
			if text != "" {
				var attachments []content.Part
				for _, p := range msg.Parts {
					switch p.(type) {
					case content.Image, content.Document:
						attachments = append(attachments, p)
					}
				}
				m.chatView, _ = m.chatView.Update(msgs.ChatViewCommitUserMsg{Text: text, Parts: attachments})
			}
		case role.Assistant:
			if len(msg.ToolCalls()) > 0 {
				continue
			}
			text := msg.TextContent()
			if text != "" {
				rendered := format.RenderMarkdown(text)
				m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + rendered + "\n"})
			}
		}
	}

	m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{
		Content: "\n" + styles.DimStyle.Render("⌘ Resumed session") + "\n",
	})

	m.inputBox, _ = m.inputBox.Update(msgs.InputResetMsg{})
	m.tokenCount = ""
	m.cancelBridge = bridge.Start(m.ctx, m.program, m.sess.Chat(), m.eng.Events(), m.eng.Tasks(), m.sess.AgentName())
	m.state = StateIdle
	return nil
}

func (m *AppModel) executeSettings() {
	cfg, err := engine.LoadConfigRaw(m.configPath)
	if err != nil {
		errLine := styles.ErrorBlockStyle.Width(m.width).Render("Error loading config: " + err.Error())
		m.chatView, _ = m.chatView.Update(msgs.ChatViewAppendMsg{Content: "\n" + errLine + "\n"})
		return
	}

	wiz := configwizard.NewWizardModel(cfg, m.configPath, m.shellyDir)
	wiz.Embedded = true
	m.configWizard = &wiz
}

func helpText() string {
	return lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(
		fmt.Sprintf("Commands:\n" +
			"  /help          Show this help message\n" +
			"  /clear         Clear the chat and start a new session\n" +
			"  /compact       Compact conversation to reclaim context\n" +
			"  /sessions      Browse and resume previous sessions\n" +
			"  /settings      Open the configuration wizard\n" +
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
