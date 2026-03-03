package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/bridge"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
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
		return commandResult{cmd: m.executeHelp(), handled: true}
	case "/clear":
		return commandResult{cmd: m.executeClear(), handled: true}
	}
	return commandResult{}
}

func (m *AppModel) executeQuit() tea.Cmd {
	if m.cancelBridge != nil {
		m.cancelBridge()
	}
	return func() tea.Msg { return tea.QuitMsg{} }
}

func (m *AppModel) executeHelp() tea.Cmd {
	helpOutput := "\n" + styles.DimStyle.Render("⌘ /help") + "\n\n" + helpText() + "\n"
	return tea.Println(helpOutput)
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
		return tea.Println("\n" + errLine + "\n")
	}
	m.sess = newSess
	m.chatView, _ = m.chatView.Update(msgs.ChatViewClearMsg{})
	m.inputBox, _ = m.inputBox.Update(msgs.InputResetMsg{})
	m.cancelBridge = bridge.Start(m.ctx, m.program, m.sess.Chat(), m.eng.Events(), m.eng.Tasks(), m.sess.AgentName())
	m.state = StateIdle
	return tea.Println("\n" + styles.DimStyle.Render("⌘ /clear") + "\n")
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
