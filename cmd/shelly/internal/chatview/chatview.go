package chatview

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// LogoArt is the ASCII art displayed at startup via tea.Println.
const LogoArt = `
       __       ____
  ___ / /  ___ / / /_ __
 (_-</ _ \/ -_) / / // /
/___/_//_/\__/_/_/\_, /
                 /___/
`

// ChatViewModel renders live agent activity. Committed content is printed to
// the terminal via tea.Println so the terminal's own scroll handles history.
type ChatViewModel struct {
	agents        map[string]*AgentContainer
	subAgents     map[string]*SubAgentItem // nested sub-agent containers keyed by agent name
	agentOrder    []string                 // agent names in arrival order
	colorRegistry map[string]string        // agent name → hex color string
	nextColorSlot int

	HasMessages   bool
	Processing    bool
	SpinnerIdx    int
	ProcessingMsg string

	Width int
}

// New creates a new ChatViewModel.
func New() ChatViewModel {
	return ChatViewModel{
		agents:        make(map[string]*AgentContainer),
		subAgents:     make(map[string]*SubAgentItem),
		colorRegistry: make(map[string]string),
	}
}

// View renders only the live (in-progress) agent content and spinner.
// Committed content has already been printed to the terminal via tea.Println.
func (m *ChatViewModel) View() string {
	var live strings.Builder

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

	return live.String()
}

// SetWidth sets the render width used for content formatting.
func (m *ChatViewModel) SetWidth(w int) {
	m.Width = w
}

// MarkMessageSent records that content has been displayed.
func (m *ChatViewModel) MarkMessageSent() {
	m.HasMessages = true
}

// CommitUserMessage renders a user message and emits it as a tea.Println cmd.
func (m *ChatViewModel) CommitUserMessage(text string) tea.Cmd {
	highlighted := highlightFilePaths(text)
	userLine := "\n" + format.RenderUserMessage(highlighted)
	m.HasMessages = true
	return tea.Println(userLine)
}

// highlightFilePaths applies accent color to @path tokens in user input text.
func highlightFilePaths(text string) string {
	accentStyle := lipgloss.NewStyle().Foreground(styles.ColorAccent)
	runes := []rune(text)
	var result strings.Builder
	i := 0
	for i < len(runes) {
		if runes[i] == '@' {
			j := i + 1
			for j < len(runes) && runes[j] != ' ' && runes[j] != '\n' && runes[j] != '\t' {
				j++
			}
			token := string(runes[i:j])
			result.WriteString(accentStyle.Render(token))
			i = j
		} else {
			result.WriteRune(runes[i])
			i++
		}
	}
	return result.String()
}

// AddMessage processes a chat message. Final answers are emitted via tea.Println.
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

	// Final answer — no tool calls.
	if text != "" {
		// Sub-agent final answers are stored on the container.
		if _, isSub := m.subAgents[agentName]; isSub {
			ac := m.resolveContainer(agentName)
			if ac != nil {
				ac.FinalAnswer = text
			}
			return nil
		}

		// Top-level final answer — store on container for CollapsedSummary.
		// If there is no active container (e.g. agent never made tool calls), emit directly.
		ac := m.resolveContainer(agentName)
		if ac != nil {
			ac.FinalAnswer = text
		} else {
			rendered := format.RenderMarkdown(text)
			m.HasMessages = true
			return tea.Println("\n" + rendered + "\n")
		}
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
	ac := NewAgentContainer(agentName, prefix, 0, "")
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
		color := styles.SubAgentPalette[m.nextColorSlot%len(styles.SubAgentPalette)]
		m.nextColorSlot++
		m.colorRegistry[agentName] = color
		childAC := NewAgentContainer(agentName, prefix, 4, color)
		sa := &SubAgentItem{Container: childAC}
		parentAC.Items = append(parentAC.Items, sa)
		m.subAgents[agentName] = sa
		return
	}

	if _, ok := m.agents[agentName]; ok {
		return
	}
	m.colorRegistry[agentName] = ""
	ac := NewAgentContainer(agentName, prefix, 0, "")
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
}

// EndAgent collapses the named agent's container into a summary and emits it
// via tea.Println so it persists in the terminal's scroll buffer.
func (m *ChatViewModel) EndAgent(agentName, _ string) tea.Cmd {
	// Check if this is a nested sub-agent.
	if sa, ok := m.subAgents[agentName]; ok {
		sa.Container.Done = true
		sa.Container.EndTime = time.Now()
		delete(m.subAgents, agentName)
		return nil
	}

	// Top-level agent.
	ac, ok := m.agents[agentName]
	if !ok {
		return nil
	}

	ac.Done = true
	ac.EndTime = time.Now()
	summary := ac.CollapsedSummary()

	delete(m.agents, agentName)
	for i, name := range m.agentOrder {
		if name == agentName {
			m.agentOrder = append(m.agentOrder[:i], m.agentOrder[i+1:]...)
			break
		}
	}

	if summary != "" {
		return tea.Println("\n" + summary + "\n")
	}
	return nil
}

// FlushAll ends all remaining agents and emits their collapsed summaries via
// tea.Println. Call this when the send completes to avoid stale live-view
// content caused by AgentEndMsg arriving after SendCompleteMsg.
func (m *ChatViewModel) FlushAll() tea.Cmd {
	var cmds []tea.Cmd

	// End sub-agents first so their containers are marked Done.
	for name, sa := range m.subAgents {
		sa.Container.Done = true
		if sa.Container.EndTime.IsZero() {
			sa.Container.EndTime = time.Now()
		}
		delete(m.subAgents, name)
	}

	// End all top-level agents and emit summaries.
	for _, name := range m.agentOrder {
		ac, ok := m.agents[name]
		if !ok {
			continue
		}
		ac.Done = true
		if ac.EndTime.IsZero() {
			ac.EndTime = time.Now()
		}
		summary := ac.CollapsedSummary()
		if summary != "" {
			cmds = append(cmds, tea.Println("\n"+summary+"\n"))
		}
	}

	m.agents = make(map[string]*AgentContainer)
	m.agentOrder = nil

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
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
	m.Processing = false
	m.SpinnerIdx = 0
	m.ProcessingMsg = ""
	m.HasMessages = false
}
