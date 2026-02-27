package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// logoArt is the ASCII art displayed before the first message.
const logoArt = `
       __       ____
  ___ / /  ___ / / /_ __
 (_-</ _ \/ -_) / / // /
/___/_//_/\__/_/_/\_, /
                 /___/
`

// chatViewModel uses a viewport to display committed content and live agent work.
type chatViewModel struct {
	viewport   viewport.Model
	agents     map[string]*agentContainer
	subAgents  map[string]*subAgentItem // nested sub-agent containers keyed by agent name
	agentOrder []string                 // agent names in arrival order

	committed     *strings.Builder // rendered committed content (pointer to avoid copy panic)
	hasMessages   bool
	processing    bool
	spinnerIdx    int
	processingMsg string

	width, height int
}

func newChatView() chatViewModel {
	vp := viewport.New()
	return chatViewModel{
		viewport:  vp,
		agents:    make(map[string]*agentContainer),
		subAgents: make(map[string]*subAgentItem),
		committed: &strings.Builder{},
	}
}

// View renders the viewport content (committed + live).
func (m *chatViewModel) View() string {
	var live strings.Builder

	// Show empty state before any messages.
	if !m.hasMessages && !m.processing && len(m.agents) == 0 {
		return dimStyle.Render(logoArt)
	}

	// Render active agent containers (live content).
	for _, name := range m.agentOrder {
		ac, ok := m.agents[name]
		if !ok {
			continue
		}
		liveView := ac.View(m.width)
		if liveView != "" {
			live.WriteString(liveView)
		}
	}

	// Show standalone spinner when processing but no active agents yet.
	if m.processing && len(m.agents) == 0 {
		frame := spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
		fmt.Fprintf(&live, "  %s %s\n",
			spinnerStyle.Render(frame),
			spinnerStyle.Render(m.processingMsg),
		)
	}

	// Combine committed + live content.
	combined := m.committed.String() + live.String()
	m.viewport.SetContent(combined)

	// Always scroll to bottom — viewport state is ephemeral (View operates on
	// a copy due to bubbletea v2 value receivers) so the offset resets to 0
	// each render. Without GotoBottom, content taller than the viewport would
	// only show the first screenful.
	m.viewport.GotoBottom()

	return m.viewport.View()
}

// setSize sets the viewport dimensions.
func (m *chatViewModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h)
}

// markMessageSent records that content has been displayed.
func (m *chatViewModel) markMessageSent() {
	m.hasMessages = true
}

// commitUserMessage renders a user message and appends it to committed content.
func (m *chatViewModel) commitUserMessage(text string) {
	userLine := "\n" + renderUserMessage(text)
	m.committed.WriteString(userLine)
	m.committed.WriteString("\n")
	m.hasMessages = true
}

// addMessage processes a chat message. Final answers are committed directly.
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
		ac := m.getOrCreateContainer(agentName, "")

		if text != "" {
			if ac.prefix == "📝" {
				ac.addPlan(text)
			} else {
				ac.addThinking(text)
			}
		}

		// Detect parallel calls: count calls per tool name.
		toolCounts := make(map[string]int)
		for _, tc := range calls {
			toolCounts[tc.Name]++
		}

		for _, tc := range calls {
			if toolCounts[tc.Name] > 1 {
				tg := ac.findLastToolGroup(tc.Name)
				if tg == nil {
					tg = ac.addToolGroup(tc.Name, 4)
				}
				ac.addGroupCall(tg, tc.ID, tc.Arguments)
			} else {
				ac.addToolCall(tc.ID, tc.Name, tc.Arguments)
			}
		}
		return nil
	}

	// Final answer — no tool calls. Commit to scrollback.
	if text != "" {
		ac := m.resolveContainer(agentName)
		prefix := "🤖"
		if ac != nil {
			prefix = ac.prefix
		}
		rendered := renderMarkdown(text)
		header := answerPrefixStyle.Render(fmt.Sprintf("%s %s", prefix, agentName))
		lines := strings.Split(rendered, "\n")
		var sb strings.Builder
		sb.WriteString(header)
		for i, line := range lines {
			if i == 0 {
				fmt.Fprintf(&sb, "\n %s%s", treeCorner, line)
			} else {
				fmt.Fprintf(&sb, "\n   %s", line)
			}
		}
		m.committed.WriteString("\n")
		m.committed.WriteString(sb.String())
		m.committed.WriteString("\n")
		return nil
	}
	return nil
}

func (m *chatViewModel) processToolMessage(msg message.Message) {
	agentName := msg.Sender
	if agentName == "" {
		agentName = "assistant"
	}

	ac := m.resolveContainer(agentName)
	if ac == nil {
		return
	}

	for _, p := range msg.Parts {
		tr, ok := p.(content.ToolResult)
		if !ok {
			continue
		}
		ac.completeToolCall(tr.ToolCallID, tr.Content, tr.IsError)
	}
}

// resolveContainer finds the agent container by name, checking top-level agents
// first, then nested sub-agents.
func (m *chatViewModel) resolveContainer(agentName string) *agentContainer {
	if ac, ok := m.agents[agentName]; ok {
		return ac
	}
	if sa, ok := m.subAgents[agentName]; ok {
		return sa.container
	}
	return nil
}

// getOrCreateContainer returns an existing container (top-level or nested) or
// creates a new top-level one.
func (m *chatViewModel) getOrCreateContainer(agentName, prefix string) *agentContainer {
	if ac := m.resolveContainer(agentName); ac != nil {
		return ac
	}
	ac := newAgentContainer(agentName, prefix, 0)
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
	return ac
}

// startAgent creates or retrieves an agent container with the given prefix.
func (m *chatViewModel) startAgent(agentName, prefix, parent string) {
	if parent != "" {
		parentAC := m.resolveContainer(parent)
		if parentAC == nil {
			parentAC = m.getOrCreateContainer(parent, "")
		}
		childAC := newAgentContainer(agentName, prefix, 4)
		sa := &subAgentItem{container: childAC}
		parentAC.items = append(parentAC.items, sa)
		m.subAgents[agentName] = sa
		return
	}

	if _, ok := m.agents[agentName]; ok {
		return
	}
	ac := newAgentContainer(agentName, prefix, 0)
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
}

// endAgent collapses the named agent's container into a summary and commits it.
func (m *chatViewModel) endAgent(agentName, _ string) {
	// Check if this is a nested sub-agent.
	if sa, ok := m.subAgents[agentName]; ok {
		sa.container.done = true
		delete(m.subAgents, agentName)
		return
	}

	// Top-level agent.
	ac, ok := m.agents[agentName]
	if !ok {
		return
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
		m.committed.WriteString("\n")
		m.committed.WriteString(summary)
		m.committed.WriteString("\n")
	}
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

// Clear resets the chat view state.
func (m *chatViewModel) Clear() {
	m.agents = make(map[string]*agentContainer)
	m.subAgents = make(map[string]*subAgentItem)
	m.agentOrder = nil
	m.committed.Reset()
	m.processing = false
	m.spinnerIdx = 0
	m.processingMsg = ""
	m.hasMessages = false
}
