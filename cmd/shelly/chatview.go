package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// chatViewModel manages active agent containers and prints committed content
// to the terminal scrollback via tea.Println.
type chatViewModel struct {
	agents        map[string]*agentContainer
	agentOrder    []string // agent names in arrival order
	verbose       bool
	processing    bool   // true while the agent is working
	spinnerIdx    int    // frame index for standalone processing spinner
	processingMsg string // random message shown while waiting for first agent
	width         int
}

func newChatView(verbose bool) chatViewModel {
	return chatViewModel{
		agents:  make(map[string]*agentContainer),
		verbose: verbose,
	}
}

// View renders only the live portion: active agent containers and the
// standalone processing spinner. Committed content is printed to the
// terminal scrollback via tea.Println and is not part of this view.
func (m chatViewModel) View() string {
	var sb strings.Builder

	for _, name := range m.agentOrder {
		ac, ok := m.agents[name]
		if !ok {
			continue
		}
		live := ac.View(m.width)
		if live != "" {
			sb.WriteString(live)
		}
	}

	// Show standalone spinner when processing but no active agents yet.
	if m.processing && len(m.agents) == 0 {
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
	agentName := msg.Sender
	if agentName == "" {
		agentName = "assistant"
	}

	if len(calls) > 0 {
		ac := m.getOrCreateAgent(agentName, "")

		if text != "" {
			ac.addThinking(text)
		}

		// Detect parallel calls: count calls per tool name.
		toolCounts := make(map[string]int)
		for _, tc := range calls {
			toolCounts[tc.Name]++
		}

		for _, tc := range calls {
			if toolCounts[tc.Name] > 1 {
				// Parallel calls of the same tool → group them.
				tg := ac.findLastToolGroup(tc.Name)
				if tg == nil {
					tg = ac.addToolGroup(tc.Name, 4)
				}
				tg.addCall(tc.Arguments)
			} else {
				ac.addToolCall(tc.Name, tc.Arguments)
			}
		}
		return nil
	}

	// Final answer — no tool calls. Print to scrollback.
	if text != "" {
		ac := m.agents[agentName]
		prefix := "🤖"
		if ac != nil {
			prefix = ac.prefix
		}
		rendered := renderMarkdown(text)
		line := answerBlockStyle.Render(
			answerPrefixStyle.Render(fmt.Sprintf("%s %s > ", prefix, agentName)) + rendered,
		)
		return tea.Println(line)
	}
	return nil
}

func (m *chatViewModel) processToolMessage(msg message.Message) {
	agentName := msg.Sender
	if agentName == "" {
		agentName = "assistant"
	}

	ac, ok := m.agents[agentName]
	if !ok {
		return
	}

	for _, p := range msg.Parts {
		tr, ok := p.(content.ToolResult)
		if !ok {
			continue
		}
		ac.completeToolCall(tr.Content, tr.IsError)
	}
}

// startAgent creates or retrieves an agent container with the given prefix.
func (m *chatViewModel) startAgent(agentName, prefix string) {
	if _, ok := m.agents[agentName]; ok {
		return
	}
	ac := newAgentContainer(agentName, prefix, 0)
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
}

// endAgent collapses the named agent's container into a summary and prints it.
func (m *chatViewModel) endAgent(agentName string) tea.Cmd {
	ac, ok := m.agents[agentName]
	if !ok {
		return nil
	}

	ac.done = true
	summary := ac.collapsedSummary()

	delete(m.agents, agentName)
	for i, name := range m.agentOrder {
		if name == agentName {
			m.agentOrder = append(m.agentOrder[:i], m.agentOrder[i+1:]...)
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

// advanceSpinners increments the spinner frame for all active containers.
func (m *chatViewModel) advanceSpinners() {
	m.spinnerIdx++
	for _, ac := range m.agents {
		ac.advanceSpinners()
	}
}

// hasActiveChains returns true if any agent container is still in progress.
func (m *chatViewModel) hasActiveChains() bool {
	return len(m.agents) > 0
}

func (m *chatViewModel) getOrCreateAgent(agentName, prefix string) *agentContainer {
	ac, ok := m.agents[agentName]
	if !ok {
		ac = newAgentContainer(agentName, prefix, 0)
		m.agents[agentName] = ac
		m.agentOrder = append(m.agentOrder, agentName)
	}
	return ac
}
