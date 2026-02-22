package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

var (
	userStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	assistantStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
)

// chatBlock is a committed, fully rendered block of text in the chat history.
type chatBlock struct {
	content string
}

// chatViewModel manages the scrollable chat viewport and active reasoning chains.
type chatViewModel struct {
	viewport      viewport.Model
	blocks        []chatBlock
	activeChains  map[string]*reasonChain
	chainOrder    []string // agent names in arrival order
	verbose       bool
	width         int
	processing    bool   // true while the agent is working
	spinnerIdx    int    // frame index for standalone processing spinner
	processingMsg string // random message shown while waiting for first chain
}

func newChatView(verbose bool) chatViewModel {
	vp := viewport.New(80, 20)
	vp.SetContent("")
	return chatViewModel{
		viewport:     vp,
		activeChains: make(map[string]*reasonChain),
		verbose:      verbose,
	}
}

func (m chatViewModel) Update(msg tea.Msg) (chatViewModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m chatViewModel) View() string {
	return m.viewport.View()
}

// addMessage processes a chat message and updates blocks/chains accordingly.
func (m *chatViewModel) addMessage(msg message.Message) {
	switch msg.Role {
	case role.System, role.User:
		// System messages are hidden; user messages are already rendered
		// by handleSubmit, so skip them to avoid duplication.
		return
	case role.Assistant:
		m.processAssistantMessage(msg)
	case role.Tool:
		m.processToolMessage(msg)
	}
	m.updateViewport()
}

func (m *chatViewModel) processAssistantMessage(msg message.Message) {
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
		return
	}

	// Final answer — no tool calls.
	if text != "" {
		rendered := renderMarkdown(text)
		m.blocks = append(m.blocks, chatBlock{
			content: assistantStyle.Render(fmt.Sprintf("%s> ", agent)) + rendered,
		})
	}
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

// endAgent collapses the named agent's chain into a summary block.
func (m *chatViewModel) endAgent(agent string) {
	chain, ok := m.activeChains[agent]
	if !ok {
		return
	}

	summary := chain.collapsedSummary()
	if summary != "" {
		m.blocks = append(m.blocks, chatBlock{content: summary})
	}

	delete(m.activeChains, agent)
	// Remove from chainOrder.
	for i, name := range m.chainOrder {
		if name == agent {
			m.chainOrder = append(m.chainOrder[:i], m.chainOrder[i+1:]...)
			break
		}
	}

	m.updateViewport()
}

// setProcessing sets the processing state and picks a random spinner message.
func (m *chatViewModel) setProcessing(on bool) {
	m.processing = on
	if on {
		m.processingMsg = randomThinkingMessage()
	}
	m.updateViewport()
}

// advanceSpinners increments the spinner frame for all active chains
// and the standalone processing spinner.
func (m *chatViewModel) advanceSpinners() {
	m.spinnerIdx++
	for _, chain := range m.activeChains {
		chain.frameIdx++
	}
	m.updateViewport()
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

func (m *chatViewModel) updateViewport() {
	var sb strings.Builder

	for _, block := range m.blocks {
		sb.WriteString(block.content)
		sb.WriteString("\n\n")
	}

	// Render active chains below committed blocks.
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

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m *chatViewModel) setSize(width, height int) {
	m.width = width
	m.viewport.Width = width
	m.viewport.Height = height
}
