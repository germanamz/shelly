package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

var (
	userStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	assistantStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	userBlockStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("2"))
	assistantBlockStyle = lipgloss.NewStyle().
				PaddingLeft(1)
	errorBlockStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("1"))
)

// chatViewModel manages active reasoning chains and prints committed content
// to the terminal scrollback via tea.Println.
type chatViewModel struct {
	activeChains  map[string]*reasonChain
	chainOrder    []string // agent names in arrival order
	verbose       bool
	processing    bool   // true while the agent is working
	spinnerIdx    int    // frame index for standalone processing spinner
	processingMsg string // random message shown while waiting for first chain
}

func newChatView(verbose bool) chatViewModel {
	return chatViewModel{
		activeChains: make(map[string]*reasonChain),
		verbose:      verbose,
	}
}

// View renders only the live portion: active reasoning chains and the
// standalone processing spinner. Committed content is printed to the
// terminal scrollback via tea.Println and is not part of this view.
func (m chatViewModel) View() string {
	var sb strings.Builder

	for _, agent := range m.chainOrder {
		chain, ok := m.activeChains[agent]
		if !ok {
			continue
		}
		live := chain.renderLive(m.verbose)
		if live != "" {
			sb.WriteString(live)
		}
	}

	// Show standalone spinner when processing but no active chains yet.
	if m.processing && len(m.activeChains) == 0 {
		frame := spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
		fmt.Fprintf(&sb, "  %s %s\n",
			spinnerStyle.Render(frame),
			spinnerStyle.Render(m.processingMsg),
		)
	}

	return sb.String()
}

// addMessage processes a chat message. Committed content (final assistant
// answers) is returned as a tea.Println command for the terminal scrollback.
func (m *chatViewModel) addMessage(msg message.Message) tea.Cmd {
	switch msg.Role {
	case role.System, role.User:
		// System messages are hidden; user messages are already printed
		// by handleSubmit, so skip them to avoid duplication.
		return nil
	case role.Assistant:
		return m.processAssistantMessage(msg)
	case role.Tool:
		m.processToolMessage(msg)
		return nil
	}
	return nil
}

func (m *chatViewModel) processAssistantMessage(msg message.Message) tea.Cmd {
	calls := msg.ToolCalls()
	text := msg.TextContent()
	agent := msg.Sender
	if agent == "" {
		agent = "assistant"
	}

	if len(calls) > 0 {
		chain := m.getOrCreateChain(agent)

		if text != "" {
			chain.addThinking(text)
		}

		for _, tc := range calls {
			chain.addCall(tc.Name, tc.Arguments)
		}
		return nil
	}

	// Final answer — no tool calls. Print to scrollback.
	if text != "" {
		rendered := renderMarkdown(text)
		line := assistantBlockStyle.Render(
			assistantStyle.Render(fmt.Sprintf("%s> ", agent)) + rendered,
		)
		return tea.Println(line)
	}
	return nil
}

func (m *chatViewModel) processToolMessage(msg message.Message) {
	agent := msg.Sender
	if agent == "" {
		agent = "assistant"
	}

	chain, ok := m.activeChains[agent]
	if !ok {
		return
	}

	for _, p := range msg.Parts {
		tr, ok := p.(content.ToolResult)
		if !ok {
			continue
		}
		chain.addResult(tr.Content, tr.IsError)
	}
}

// endAgent collapses the named agent's chain into a summary and prints it
// to the terminal scrollback.
func (m *chatViewModel) endAgent(agent string) tea.Cmd {
	chain, ok := m.activeChains[agent]
	if !ok {
		return nil
	}

	summary := chain.collapsedSummary()

	delete(m.activeChains, agent)
	// Remove from chainOrder.
	for i, name := range m.chainOrder {
		if name == agent {
			m.chainOrder = append(m.chainOrder[:i], m.chainOrder[i+1:]...)
			break
		}
	}

	if summary != "" {
		return tea.Println(summary)
	}
	return nil
}

// setProcessing sets the processing state and picks a random spinner message.
func (m *chatViewModel) setProcessing(on bool) {
	m.processing = on
	if on {
		m.processingMsg = randomThinkingMessage()
	}
}

// advanceSpinners increments the spinner frame for all active chains
// and the standalone processing spinner.
func (m *chatViewModel) advanceSpinners() {
	m.spinnerIdx++
	for _, chain := range m.activeChains {
		chain.frameIdx++
	}
}

// hasActiveChains returns true if any agent chain is still in progress.
func (m *chatViewModel) hasActiveChains() bool {
	return len(m.activeChains) > 0
}

func (m *chatViewModel) getOrCreateChain(agent string) *reasonChain {
	chain, ok := m.activeChains[agent]
	if !ok {
		chain = newReasonChain(agent)
		m.activeChains[agent] = chain
		m.chainOrder = append(m.chainOrder, agent)
	}
	return chain
}
