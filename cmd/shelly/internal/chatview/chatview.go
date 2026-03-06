package chatview

import (
	"fmt"
	"sort"
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

// maxViewStackDepth is the maximum navigation depth for the view stack.
const maxViewStackDepth = 32

// viewStackEntry holds state for one level of agent navigation.
type viewStackEntry struct {
	AgentID      string
	Container    *AgentContainer // pinned reference — survives map deletion on agent end
	ScrollOffset int             // preserved viewport scroll position for this level
}

// ChatViewModel renders all chat content (committed history + live agent
// activity) inside a viewport. Content that was previously emitted via
// tea.Println is now appended to an internal committed buffer and rendered
// within bubbletea's managed view area.
type ChatViewModel struct {
	viewport  viewport.Model
	committed []string // committed content lines (user msgs, summaries, etc.)

	agents         map[string]*AgentContainer
	subAgents      map[string]*AgentContainer // nested sub-agent containers keyed by agent name
	subAgentParent map[string]string          // child agent name → parent agent name
	agentOrder     []string                   // agent names in arrival order
	colorRegistry  map[string]string          // agent name → hex color string
	nextColorSlot  int

	viewedAgent string           // "" = root view, or agent instance name
	viewStack   []viewStackEntry // navigation stack for back functionality

	HasMessages    bool
	Processing     bool
	SpinnerIdx     int
	ProcessingMsg  string
	scrollToBottom bool // when true, next rebuildContent forces scroll to bottom

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
		viewport:       vp,
		agents:         make(map[string]*AgentContainer),
		subAgents:      make(map[string]*AgentContainer),
		subAgentParent: make(map[string]string),
		colorRegistry:  make(map[string]string),
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
		m.commitUserMessage(msg.Text, msg.Parts)
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewFocusAgentMsg:
		m.focusAgent(msg.AgentID)
		m.rebuildContent()
		return m, nil
	case msgs.ChatViewNavigateBackMsg:
		m.navigateBack()
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

// SubAgentInfo describes a currently running sub-agent for the sub-agent browser.
type SubAgentInfo struct {
	ID       string // agent instance name
	Label    string // agent instance name (display)
	Provider string // provider label (e.g. "anthropic/claude-sonnet-4")
	Status   string // "running" or "done"
	Color    string // hex color from colorRegistry
	ParentID string // parent agent name ("" for top-level sub-agents)
	Depth    int    // nesting depth (0 = direct child of root)
}

// SubAgents returns info about all currently running sub-agents.
// Only called from the bubbletea Update goroutine — no mutex needed.
func (m ChatViewModel) SubAgents() []SubAgentInfo {
	if len(m.subAgents) == 0 {
		return nil
	}

	infos := make([]SubAgentInfo, 0, len(m.subAgents))
	for name, ac := range m.subAgents {
		status := "running"
		if ac.Done {
			status = "done"
		}

		parentID := m.subAgentParent[name]
		depth := m.subAgentDepth(name)

		infos = append(infos, SubAgentInfo{
			ID:       name,
			Label:    name,
			Provider: ac.ProviderLabel,
			Status:   status,
			Color:    m.colorRegistry[name],
			ParentID: parentID,
			Depth:    depth,
		})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].ID < infos[j].ID })
	return infos
}

// subAgentDepth computes the nesting depth of a sub-agent by walking the parent chain.
// Depth 0 = direct child of a top-level agent.
func (m ChatViewModel) subAgentDepth(name string) int {
	depth := 0
	current := name
	for {
		parent, ok := m.subAgentParent[current]
		if !ok {
			break
		}
		// If the parent is a top-level agent (not in subAgentParent), stop.
		if _, isSub := m.subAgentParent[parent]; !isSub {
			break
		}
		depth++
		current = parent
	}
	return depth
}

// FindContainer resolves an agent ID to its AgentContainer pointer.
// Checks top-level agents first, then sub-agents.
func (m ChatViewModel) FindContainer(agentID string) *AgentContainer {
	if ac, ok := m.agents[agentID]; ok {
		return ac
	}
	if ac, ok := m.subAgents[agentID]; ok {
		return ac
	}
	return nil
}

// HasActiveChains returns true if any agent container is still in progress.
func (m ChatViewModel) HasActiveChains() bool {
	return len(m.agents) > 0
}

// ViewedAgent returns the currently viewed agent ID ("" for root view).
func (m ChatViewModel) ViewedAgent() string { return m.viewedAgent }

// ViewStackEntry exposes the agent ID for a view stack level.
type ViewStackEntry struct {
	AgentID string
}

// ViewStack returns the current view stack entries (for cleanup purposes).
func (m ChatViewModel) ViewStack() []ViewStackEntry {
	entries := make([]ViewStackEntry, len(m.viewStack))
	for i, e := range m.viewStack {
		entries[i] = ViewStackEntry{AgentID: e.AgentID}
	}
	return entries
}

// HeaderHeight returns the number of extra lines needed above the input for
// the breadcrumb (0 when at root, 1 when viewing a sub-agent).
func (m ChatViewModel) HeaderHeight() int {
	if m.viewedAgent == "" {
		return 0
	}
	return 1
}

// focusAgent switches the view to display the given agent's history.
func (m *ChatViewModel) focusAgent(agentID string) {
	ac := m.FindContainer(agentID)
	if ac == nil {
		return
	}
	if len(m.viewStack) >= maxViewStackDepth {
		return
	}

	// Save current scroll position on the current stack top (or root).
	scrollPos := m.viewport.ScrollPercent()
	scrollOffset := int(scrollPos * float64(m.viewport.TotalLineCount()))
	if len(m.viewStack) > 0 {
		m.viewStack[len(m.viewStack)-1].ScrollOffset = scrollOffset
	}

	m.viewStack = append(m.viewStack, viewStackEntry{
		AgentID:   agentID,
		Container: ac,
	})
	m.viewedAgent = agentID
	m.scrollToBottom = true // Force scroll to bottom after rebuildContent.
}

// navigateBack pops the view stack and returns to the previous view.
func (m *ChatViewModel) navigateBack() {
	if len(m.viewStack) == 0 {
		return
	}

	m.viewStack = m.viewStack[:len(m.viewStack)-1]

	if len(m.viewStack) == 0 {
		m.viewedAgent = ""
	} else {
		top := m.viewStack[len(m.viewStack)-1]
		m.viewedAgent = top.AgentID
	}
	m.scrollToBottom = true // Auto-scroll to bottom on view switch.
}

// RenderBreadcrumb returns the breadcrumb line showing the navigation path.
// Only renders when viewing a sub-agent. Composed by AppModel in the layout.
func (m ChatViewModel) RenderBreadcrumb() string {
	if m.viewedAgent == "" {
		return ""
	}

	backStyle := lipgloss.NewStyle().Foreground(styles.ColorAccent)
	sepStyle := styles.DimStyle
	var parts []string
	parts = append(parts, backStyle.Render("<- root"))

	for i, entry := range m.viewStack {
		parts = append(parts, sepStyle.Render(" > "))
		agentStyle := colorStyle(m.colorRegistry[entry.AgentID])
		if i == len(m.viewStack)-1 {
			agentStyle = agentStyle.Bold(true)
		}
		// Strikethrough for completed agents.
		ac := entry.Container
		if ac != nil && ac.Done {
			agentStyle = agentStyle.Strikethrough(true)
		}
		parts = append(parts, agentStyle.Render(entry.AgentID))
	}

	line := strings.Join(parts, "")

	// Truncate if exceeds width.
	if m.Width > 0 && lipgloss.Width(line) > m.Width {
		// Simple truncation: keep first and last segment, collapse middle.
		if len(m.viewStack) > 1 {
			var truncParts []string
			truncParts = append(truncParts, backStyle.Render("<- root"))
			truncParts = append(truncParts, sepStyle.Render(" > "))
			truncParts = append(truncParts, sepStyle.Render("..."))
			truncParts = append(truncParts, sepStyle.Render(" > "))
			last := m.viewStack[len(m.viewStack)-1]
			lastStyle := colorStyle(m.colorRegistry[last.AgentID]).Bold(true)
			if last.Container != nil && last.Container.Done {
				lastStyle = lastStyle.Strikethrough(true)
			}
			truncParts = append(truncParts, lastStyle.Render(last.AgentID))
			line = strings.Join(truncParts, "")
		}
	}

	return line
}

// cleanViewStackEntry removes the view stack entry for the given agent,
// unless it's the currently viewed agent (pinned pointer keeps it alive).
func (m *ChatViewModel) cleanViewStackEntry(agentID string) {
	if m.viewedAgent == agentID {
		return // keep pinned — will be removed on navigate-away
	}
	for i, entry := range m.viewStack {
		if entry.AgentID == agentID {
			m.viewStack = append(m.viewStack[:i], m.viewStack[i+1:]...)
			return
		}
	}
}

// appendContent adds text to the committed buffer.
func (m *ChatViewModel) appendContent(text string) {
	m.committed = append(m.committed, text)
}

// commitUserMessage renders a user message and appends it to the committed buffer.
func (m *ChatViewModel) commitUserMessage(text string, parts []content.Part) {
	highlighted := highlightFilePaths(text)

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(format.RenderUserMessage(highlighted, m.Width))

	// Render attachment indicators for non-text parts.
	for _, p := range parts {
		label := renderAttachmentLabel(p)
		if label != "" {
			sb.WriteString("\n   ")
			sb.WriteString(styles.DimStyle.Render(label))
		}
	}

	sb.WriteString("\n")
	m.HasMessages = true
	m.committed = append(m.committed, sb.String())
}

// rebuildContent rebuilds the viewport content from committed + live content,
// auto-scrolling to the bottom if the user was already there.
func (m *ChatViewModel) rebuildContent() {
	wasAtBottom := m.scrollToBottom || m.viewport.AtBottom() || m.viewport.TotalLineCount() <= m.viewport.Height()
	m.scrollToBottom = false

	var full strings.Builder
	for _, c := range m.committed {
		full.WriteString(c)
	}

	// Append live agent content with a blank-line separator.
	live := m.liveContent()
	if live != "" {
		if len(m.committed) > 0 {
			full.WriteString("\n")
		}
		full.WriteString(live)
	}

	m.viewport.SetContent(full.String())

	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

// liveContent renders the current live agent activity (spinners, tool calls).
func (m *ChatViewModel) liveContent() string {
	// Agent-scoped view: render only the viewed agent's container.
	if m.viewedAgent != "" {
		ac := m.viewedAgentContainer()
		if ac == nil {
			return ""
		}
		// Temporarily set MaxShow=0 to show full history.
		origMaxShow := ac.MaxShow
		ac.MaxShow = 0
		view := ac.View(m.Width)
		ac.MaxShow = origMaxShow
		return view
	}

	var live strings.Builder

	for i, name := range m.agentOrder {
		ac, ok := m.agents[name]
		if !ok {
			continue
		}
		liveView := ac.View(m.Width)
		if liveView != "" {
			if i > 0 {
				live.WriteString("\n")
			}
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

// viewedAgentContainer returns the container for the currently viewed agent,
// using the pinned pointer from the view stack if available.
func (m *ChatViewModel) viewedAgentContainer() *AgentContainer {
	if len(m.viewStack) > 0 {
		top := m.viewStack[len(m.viewStack)-1]
		if top.Container != nil {
			return top.Container
		}
	}
	return m.FindContainer(m.viewedAgent)
}

// renderAttachmentLabel returns a display label for an attachment part, or "" if not applicable.
func renderAttachmentLabel(p content.Part) string {
	switch v := p.(type) {
	case content.Image:
		size := format.FmtBytes(len(v.Data))
		mediaType := v.MediaType
		if mediaType == "" {
			mediaType = "image"
		}
		return fmt.Sprintf("[Image: %s (%s)]", mediaType, size)
	case content.Document:
		size := format.FmtBytes(len(v.Data))
		name := v.Path
		if name == "" {
			name = v.MediaType
		}
		return fmt.Sprintf("[Document: %s (%s)]", name, size)
	default:
		return ""
	}
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
	if ac, ok := m.subAgents[agentName]; ok {
		return ac
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
		parentAC.Items = append(parentAC.Items, childAC)
		m.subAgents[agentName] = childAC
		m.subAgentParent[agentName] = parent
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
		sa.Done = true
		sa.EndTime = time.Now()
		if sa.FinalAnswer == "" && completionSummary != "" {
			sa.FinalAnswer = completionSummary
		}

		// Replace the AgentContainer in parent's Items with a SummaryLineItem.
		parentName := m.subAgentParent[agentName]
		if parentAC := m.resolveContainer(parentName); parentAC != nil {
			m.replaceWithSummaryLine(parentAC, sa)
		}

		delete(m.subAgents, agentName)
		delete(m.subAgentParent, agentName)
		// Clean up view stack entry if not currently viewed.
		m.cleanViewStackEntry(agentName)
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

// replaceWithSummaryLine finds the AgentContainer for the given sub-agent
// in the parent's Items and replaces it with a SummaryLineItem.
func (m *ChatViewModel) replaceWithSummaryLine(parentAC *AgentContainer, sa *AgentContainer) {
	end := sa.EndTime
	if end.IsZero() {
		end = time.Now()
	}
	summary := &SummaryLineItem{
		Agent:         sa.Agent,
		Prefix:        sa.Prefix,
		ProviderLabel: sa.ProviderLabel,
		FinalAnswer:   sa.FinalAnswer,
		Color:         sa.Color,
		Elapsed:       format.FmtDuration(end.Sub(sa.StartTime)),
	}
	for i, item := range parentAC.Items {
		if ac, ok := item.(*AgentContainer); ok && ac.Agent == sa.Agent {
			parentAC.Items[i] = summary
			return
		}
	}
}

// flushAll ends all remaining agents and appends their collapsed summaries
// to the committed buffer.
func (m *ChatViewModel) flushAll() {
	// End sub-agents first and replace their containers with summary lines.
	for name, sa := range m.subAgents {
		sa.Done = true
		if sa.EndTime.IsZero() {
			sa.EndTime = time.Now()
		}
		parentName := m.subAgentParent[name]
		if parentAC := m.resolveContainer(parentName); parentAC != nil {
			m.replaceWithSummaryLine(parentAC, sa)
		}
		delete(m.subAgents, name)
		delete(m.subAgentParent, name)
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
	m.subAgents = make(map[string]*AgentContainer)
	m.subAgentParent = make(map[string]string)
	m.agentOrder = nil
	m.committed = nil
	m.viewedAgent = ""
	m.viewStack = nil
	m.Processing = false
	m.SpinnerIdx = 0
	m.ProcessingMsg = ""
	m.HasMessages = false
	m.viewport.SetContent("")
	m.viewport.GotoTop()
}
