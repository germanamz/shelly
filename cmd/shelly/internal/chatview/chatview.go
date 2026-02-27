package chatview

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// LogoArt is the ASCII art displayed before the first message.
const LogoArt = `
       __       ____
  ___ / /  ___ / / /_ __
 (_-</ _ \/ -_) / / // /
/___/_//_/\__/_/_/\_, /
                 /___/
`

// ChatViewModel uses a viewport to display committed content and live agent work.
type ChatViewModel struct {
	viewport   viewport.Model
	agents     map[string]*AgentContainer
	subAgents  map[string]*SubAgentItem // nested sub-agent containers keyed by agent name
	agentOrder []string                 // agent names in arrival order

	Committed     *strings.Builder // rendered committed content (pointer to avoid copy panic)
	HasMessages   bool
	Processing    bool
	SpinnerIdx    int
	ProcessingMsg string

	Width, Height int
}

// New creates a new ChatViewModel.
func New() ChatViewModel {
	vp := viewport.New()
	return ChatViewModel{
		viewport:  vp,
		agents:    make(map[string]*AgentContainer),
		subAgents: make(map[string]*SubAgentItem),
		Committed: &strings.Builder{},
	}
}

// View renders the viewport content (committed + live).
func (m *ChatViewModel) View() string {
	var live strings.Builder

	// Show empty state before any messages.
	if !m.HasMessages && !m.Processing && len(m.agents) == 0 {
		return styles.DimStyle.Render(LogoArt)
	}

	// Render active agent containers (live content).
	for _, name := range m.agentOrder {
		ac, ok := m.agents[name]
		if !ok {
			continue
		}
		liveView := ac.View(m.Width)
		if liveView != "" {
			live.WriteString(liveView)
		}
	}

	// Show standalone spinner when processing but no active agents yet.
	if m.Processing && len(m.agents) == 0 {
		frame := format.SpinnerFrames[m.SpinnerIdx%len(format.SpinnerFrames)]
		fmt.Fprintf(&live, "  %s %s\n",
			styles.SpinnerStyle.Render(frame),
			styles.SpinnerStyle.Render(m.ProcessingMsg),
		)
	}

	// Combine committed + live content.
	combined := m.Committed.String() + live.String()
	m.viewport.SetContent(combined)

	// Always scroll to bottom — viewport state is ephemeral (View operates on
	// a copy due to bubbletea v2 value receivers) so the offset resets to 0
	// each render. Without GotoBottom, content taller than the viewport would
	// only show the first screenful.
	m.viewport.GotoBottom()

	return m.viewport.View()
}

// SetSize sets the viewport dimensions.
func (m *ChatViewModel) SetSize(w, h int) {
	m.Width = w
	m.Height = h
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h)
}

// MarkMessageSent records that content has been displayed.
func (m *ChatViewModel) MarkMessageSent() {
	m.HasMessages = true
}

// CommitUserMessage renders a user message and appends it to committed content.
func (m *ChatViewModel) CommitUserMessage(text string) {
	userLine := "\n" + format.RenderUserMessage(text)
	m.Committed.WriteString(userLine)
	m.Committed.WriteString("\n")
	m.HasMessages = true
}

// AddMessage processes a chat message. Final answers are committed directly.
func (m *ChatViewModel) AddMessage(msg message.Message) tea.Cmd {
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

func (m *ChatViewModel) processAssistantMessage(msg message.Message) tea.Cmd {
	calls := msg.ToolCalls()
	text := msg.TextContent()
	agentName := msg.Sender
	if agentName == "" {
		agentName = "assistant"
	}

	if len(calls) > 0 {
		ac := m.getOrCreateContainer(agentName, "")

		if text != "" {
			if ac.Prefix == "📝" {
				ac.AddPlan(text)
			} else {
				ac.AddThinking(text)
			}
		}

		// Detect parallel calls: count calls per tool name.
		toolCounts := make(map[string]int)
		for _, tc := range calls {
			toolCounts[tc.Name]++
		}

		for _, tc := range calls {
			if toolCounts[tc.Name] > 1 {
				tg := ac.FindLastToolGroup(tc.Name)
				if tg == nil {
					tg = ac.AddToolGroup(tc.Name, 4)
				}
				ac.AddGroupCall(tg, tc.ID, tc.Arguments)
			} else {
				ac.AddToolCall(tc.ID, tc.Name, tc.Arguments)
			}
		}
		return nil
	}

	// Final answer — no tool calls. Commit to scrollback.
	if text != "" {
		ac := m.resolveContainer(agentName)
		prefix := "🤖"
		if ac != nil {
			prefix = ac.Prefix
		}
		rendered := format.RenderMarkdown(text)
		header := styles.AnswerPrefixStyle.Render(fmt.Sprintf("%s %s", prefix, agentName))
		lines := strings.Split(rendered, "\n")
		var sb strings.Builder
		sb.WriteString(header)
		for i, line := range lines {
			if i == 0 {
				fmt.Fprintf(&sb, "\n %s%s", styles.TreeCorner, line)
			} else {
				fmt.Fprintf(&sb, "\n   %s", line)
			}
		}
		m.Committed.WriteString("\n")
		m.Committed.WriteString(sb.String())
		m.Committed.WriteString("\n")
		return nil
	}
	return nil
}

func (m *ChatViewModel) processToolMessage(msg message.Message) {
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
		ac.CompleteToolCall(tr.ToolCallID, tr.Content, tr.IsError)
	}
}

// resolveContainer finds the agent container by name, checking top-level agents
// first, then nested sub-agents.
func (m *ChatViewModel) resolveContainer(agentName string) *AgentContainer {
	if ac, ok := m.agents[agentName]; ok {
		return ac
	}
	if sa, ok := m.subAgents[agentName]; ok {
		return sa.Container
	}
	return nil
}

// getOrCreateContainer returns an existing container (top-level or nested) or
// creates a new top-level one.
func (m *ChatViewModel) getOrCreateContainer(agentName, prefix string) *AgentContainer {
	if ac := m.resolveContainer(agentName); ac != nil {
		return ac
	}
	ac := NewAgentContainer(agentName, prefix, 0)
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
	return ac
}

// StartAgent creates or retrieves an agent container with the given prefix.
func (m *ChatViewModel) StartAgent(agentName, prefix, parent string) {
	if parent != "" {
		parentAC := m.resolveContainer(parent)
		if parentAC == nil {
			parentAC = m.getOrCreateContainer(parent, "")
		}
		childAC := NewAgentContainer(agentName, prefix, 4)
		sa := &SubAgentItem{Container: childAC}
		parentAC.Items = append(parentAC.Items, sa)
		m.subAgents[agentName] = sa
		return
	}

	if _, ok := m.agents[agentName]; ok {
		return
	}
	ac := NewAgentContainer(agentName, prefix, 0)
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
}

// EndAgent collapses the named agent's container into a summary and commits it.
func (m *ChatViewModel) EndAgent(agentName, _ string) {
	// Check if this is a nested sub-agent.
	if sa, ok := m.subAgents[agentName]; ok {
		sa.Container.Done = true
		delete(m.subAgents, agentName)
		return
	}

	// Top-level agent.
	ac, ok := m.agents[agentName]
	if !ok {
		return
	}

	ac.Done = true
	summary := ac.CollapsedSummary()

	delete(m.agents, agentName)
	for i, name := range m.agentOrder {
		if name == agentName {
			m.agentOrder = append(m.agentOrder[:i], m.agentOrder[i+1:]...)
			break
		}
	}

	if summary != "" {
		m.Committed.WriteString("\n")
		m.Committed.WriteString(summary)
		m.Committed.WriteString("\n")
	}
}

// SetProcessing sets the processing state and picks a random spinner message.
func (m *ChatViewModel) SetProcessing(on bool) {
	m.Processing = on
	if on {
		m.ProcessingMsg = format.RandomThinkingMessage()
	}
}

// AdvanceSpinners increments the spinner frame for all active containers.
func (m *ChatViewModel) AdvanceSpinners() {
	m.SpinnerIdx++
	for _, ac := range m.agents {
		ac.AdvanceSpinners()
	}
}

// HasActiveChains returns true if any agent container is still in progress.
func (m *ChatViewModel) HasActiveChains() bool {
	return len(m.agents) > 0
}

// Clear resets the chat view state.
func (m *ChatViewModel) Clear() {
	m.agents = make(map[string]*AgentContainer)
	m.subAgents = make(map[string]*SubAgentItem)
	m.agentOrder = nil
	m.Committed.Reset()
	m.Processing = false
	m.SpinnerIdx = 0
	m.ProcessingMsg = ""
	m.HasMessages = false
}
