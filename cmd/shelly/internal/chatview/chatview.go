package chatview

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

// LogoArt is the ASCII art displayed at startup.
const LogoArt = `
       __       ____
  ___ / /  ___ / / /_ __
 (_-</ _ \/ -_) / / // /
/___/_//_/\__/_/_/\_, /
                 /___/
`

// ChatViewModel renders all chat content (committed history + live agent
// activity) inside a viewport. Content that was previously emitted via
// tea.Println is now appended to an internal committed buffer and rendered
// within bubbletea's managed view area.
type ChatViewModel struct {
	viewport  viewport.Model
	committed []string // committed content lines (user msgs, summaries, etc.)

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
	vp := viewport.New()
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 1
	// Disable default keyboard bindings — we forward keys explicitly.
	vp.KeyMap = viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithDisabled()),
		PageUp:       key.NewBinding(key.WithDisabled()),
		HalfPageUp:   key.NewBinding(key.WithDisabled()),
		HalfPageDown: key.NewBinding(key.WithDisabled()),
		Down:         key.NewBinding(key.WithDisabled()),
		Up:           key.NewBinding(key.WithDisabled()),
		Left:         key.NewBinding(key.WithDisabled()),
		Right:        key.NewBinding(key.WithDisabled()),
	}

	return ChatViewModel{
		viewport:      vp,
		agents:        make(map[string]*AgentContainer),
		subAgents:     make(map[string]*SubAgentItem),
		colorRegistry: make(map[string]string),
	}
}

// Update processes messages for the chat view.
func (m ChatViewModel) Update(msg tea.Msg) (ChatViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case msgs.ChatMessageMsg:
		m.addMessage(msg.Msg)
		m.rebuildContent()
		return m, nil
	case msgs.AgentStartMsg:
		m.startAgent(msg.Agent, msg.Prefix, msg.Parent, msg.ProviderLabel, msg.Task)
		return m, nil
	case msgs.AgentEndMsg:
		m.endAgent(msg.Agent, msg.Summary)
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewSetWidthMsg:
		m.Width = msg.Width
		m.viewport.SetWidth(msg.Width)
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewSetHeightMsg:
		m.viewport.SetHeight(msg.Height)
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewSetProcessingMsg:
		m.setProcessing(msg.Processing)
		return m, nil
	case msgs.ChatViewAdvanceSpinnersMsg:
		m.advanceSpinners()
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewClearMsg:
		m.clear()
		return m, nil
	case msgs.ChatViewFlushAllMsg:
		m.flushAll()
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewMarkSentMsg:
		m.HasMessages = true
		return m, nil
	case msgs.ChatViewCommitUserMsg:
		m.commitUserMessage(msg.Text)
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewAppendMsg:
		m.appendContent(msg.Content)
		m.rebuildContent()
		return m, nil
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// HandleScrollKey processes scroll keys (PageUp/Down) and returns true if handled.
func (m *ChatViewModel) HandleScrollKey(msg tea.KeyPressMsg) bool {
	k := msg.Key()
	switch k.Code {
	case tea.KeyPgUp:
		m.viewport.PageUp()
		return true
	case tea.KeyPgDown:
		m.viewport.PageDown()
		return true
	}
	return false
}

// View renders the viewport containing all committed + live content.
func (m ChatViewModel) View() string {
	return m.viewport.View()
}

// HasActiveChains returns true if any agent container is still in progress.
func (m ChatViewModel) HasActiveChains() bool {
	return len(m.agents) > 0
}

// appendContent adds text to the committed buffer.
func (m *ChatViewModel) appendContent(text string) {
	m.committed = append(m.committed, text)
}

// commitUserMessage renders a user message and appends it to the committed buffer.
func (m *ChatViewModel) commitUserMessage(text string) {
	highlighted := highlightFilePaths(text)
	userLine := "\n" + format.RenderUserMessage(highlighted) + "\n"
	m.HasMessages = true
	m.committed = append(m.committed, userLine)
}

// rebuildContent rebuilds the viewport content from committed + live content,
// auto-scrolling to the bottom if the user was already there.
func (m *ChatViewModel) rebuildContent() {
	wasAtBottom := m.viewport.AtBottom() || m.viewport.TotalLineCount() <= m.viewport.Height()

	var full strings.Builder
	for _, c := range m.committed {
		full.WriteString(c)
	}

	// Append live agent content.
	live := m.liveContent()
	if live != "" {
		full.WriteString(live)
	}

	m.viewport.SetContent(full.String())

	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

// liveContent renders the current live agent activity (spinners, tool calls).
func (m *ChatViewModel) liveContent() string {
	var live strings.Builder

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

	if m.Processing && len(m.agents) == 0 {
		frame := format.SpinnerFrames[m.SpinnerIdx%len(format.SpinnerFrames)]
		fmt.Fprintf(&live, "  %s %s\n",
			styles.SpinnerStyle.Render(frame),
			styles.SpinnerStyle.Render(m.ProcessingMsg),
		)
	}

	return live.String()
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

// addMessage processes a chat message.
func (m *ChatViewModel) addMessage(msg message.Message) {
	switch msg.Role {
	case role.System, role.User:
		return
	case role.Assistant:
		m.processAssistantMessage(msg)
	case role.Tool:
		m.processToolMessage(msg)
	}
}

func (m *ChatViewModel) processAssistantMessage(msg message.Message) {
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
		return
	}

	// Final answer — no tool calls.
	if text != "" {
		// Sub-agent final answers are stored on the container.
		if _, isSub := m.subAgents[agentName]; isSub {
			ac := m.resolveContainer(agentName)
			if ac != nil {
				ac.FinalAnswer = text
			}
			return
		}

		// Top-level final answer — store on container for CollapsedSummary.
		// If there is no active container (e.g. agent never made tool calls), commit directly.
		ac := m.resolveContainer(agentName)
		if ac != nil {
			ac.FinalAnswer = text
		} else {
			rendered := format.RenderMarkdown(text)
			m.HasMessages = true
			m.committed = append(m.committed, "\n"+rendered+"\n")
		}
	}
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
	ac := NewAgentContainer(agentName, prefix, 0, "", "")
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
	return ac
}

// startAgent creates or retrieves an agent container with the given prefix.
func (m *ChatViewModel) startAgent(agentName, prefix, parent, providerLabel, task string) {
	if parent != "" {
		parentAC := m.resolveContainer(parent)
		if parentAC == nil {
			parentAC = m.getOrCreateContainer(parent, "")
		}
		color := styles.SubAgentPalette[m.nextColorSlot%len(styles.SubAgentPalette)]
		m.nextColorSlot++
		m.colorRegistry[agentName] = color
		childAC := NewAgentContainer(agentName, prefix, 4, color, providerLabel)
		if task != "" {
			childAC.Items = append(childAC.Items, &TaskMessageItem{Text: task, Color: color})
		}
		sa := &SubAgentItem{Container: childAC}
		parentAC.Items = append(parentAC.Items, sa)
		m.subAgents[agentName] = sa
		return
	}

	if _, ok := m.agents[agentName]; ok {
		return
	}
	m.colorRegistry[agentName] = ""
	ac := NewAgentContainer(agentName, prefix, 0, "", providerLabel)
	m.agents[agentName] = ac
	m.agentOrder = append(m.agentOrder, agentName)
}

// endAgent collapses the named agent's container into a summary and appends
// it to the committed buffer. completionSummary is the agent's completion
// summary (from CompletionResult or final text); it is used as FinalAnswer
// when the container has none.
func (m *ChatViewModel) endAgent(agentName, completionSummary string) {
	// Check if this is a nested sub-agent.
	if sa, ok := m.subAgents[agentName]; ok {
		sa.Container.Done = true
		sa.Container.EndTime = time.Now()
		if sa.Container.FinalAnswer == "" && completionSummary != "" {
			sa.Container.FinalAnswer = completionSummary
		}
		delete(m.subAgents, agentName)
		return
	}

	// Top-level agent.
	ac, ok := m.agents[agentName]
	if !ok {
		return
	}

	ac.Done = true
	ac.EndTime = time.Now()
	if ac.FinalAnswer == "" && completionSummary != "" {
		ac.FinalAnswer = completionSummary
	}
	summary := ac.CollapsedSummary()

	delete(m.agents, agentName)
	for i, name := range m.agentOrder {
		if name == agentName {
			m.agentOrder = append(m.agentOrder[:i], m.agentOrder[i+1:]...)
			break
		}
	}

	if summary != "" {
		m.committed = append(m.committed, "\n"+summary+"\n")
	}
}

// flushAll ends all remaining agents and appends their collapsed summaries
// to the committed buffer.
func (m *ChatViewModel) flushAll() {
	// End sub-agents first so their containers are marked Done.
	for name, sa := range m.subAgents {
		sa.Container.Done = true
		if sa.Container.EndTime.IsZero() {
			sa.Container.EndTime = time.Now()
		}
		delete(m.subAgents, name)
	}

	// End all top-level agents and commit summaries.
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
			m.committed = append(m.committed, "\n"+summary+"\n")
		}
	}

	m.agents = make(map[string]*AgentContainer)
	m.agentOrder = nil
}

// setProcessing sets the processing state and picks a random spinner message.
func (m *ChatViewModel) setProcessing(on bool) {
	m.Processing = on
	if on {
		m.ProcessingMsg = format.RandomThinkingMessage()
	}
}

// advanceSpinners increments the spinner frame for all active containers.
func (m *ChatViewModel) advanceSpinners() {
	m.SpinnerIdx++
	for _, ac := range m.agents {
		ac.AdvanceSpinners()
	}
}

// clear resets the chat view state including the viewport content.
func (m *ChatViewModel) clear() {
	m.agents = make(map[string]*AgentContainer)
	m.subAgents = make(map[string]*SubAgentItem)
	m.agentOrder = nil
	m.committed = nil
	m.Processing = false
	m.SpinnerIdx = 0
	m.ProcessingMsg = ""
	m.HasMessages = false
	m.viewport.SetContent("")
	m.viewport.GotoTop()
}
