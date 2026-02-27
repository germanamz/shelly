package configwizard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/engine"
)

// -----------------------------------------------------------------------
// MCP Server List Screen
// -----------------------------------------------------------------------

type mcpListScreen struct {
	cfg    *engine.Config
	cursor int
	form   *mcpFormScreen
}

func newMCPListScreen(cfg *engine.Config) *mcpListScreen {
	return &mcpListScreen{cfg: cfg}
}

func (s *mcpListScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if s.form != nil {
		updated, cmd := s.form.Update(msg)
		if updated == nil {
			s.form = nil
			return s, nil
		}
		s.form = updated.(*mcpFormScreen)
		return s, cmd
	}

	kmsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	n := len(s.cfg.MCPServers)
	switch kmsg.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < n {
			s.cursor++
		}
	case "enter":
		if s.cursor < n {
			s.form = newMCPFormScreen(&s.cfg.MCPServers[s.cursor], false)
		} else {
			s.cfg.MCPServers = append(s.cfg.MCPServers, engine.MCPConfig{})
			s.form = newMCPFormScreen(&s.cfg.MCPServers[len(s.cfg.MCPServers)-1], true)
		}
		return s, s.form.init()
	case "d":
		if s.cursor < n {
			s.cfg.MCPServers = append(s.cfg.MCPServers[:s.cursor], s.cfg.MCPServers[s.cursor+1:]...)
			if s.cursor >= len(s.cfg.MCPServers) && s.cursor > 0 {
				s.cursor--
			}
		}
	case "esc":
		return nil, nil
	}
	return s, nil
}

func (s *mcpListScreen) View() string {
	if s.form != nil {
		return s.form.View()
	}

	var b strings.Builder
	title := styles.AskTitleStyle.Render("MCP Servers")
	b.WriteString(title + "\n\n")

	for i, m := range s.cfg.MCPServers {
		cursor := "  "
		if i == s.cursor {
			cursor = "> "
		}
		transport := "stdio"
		if m.URL != "" {
			transport = "sse"
		}
		line := fmt.Sprintf("%s%s (%s)", cursor, m.Name, transport)
		if i == s.cursor {
			b.WriteString(styles.AskSelStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	cursor := "  "
	if s.cursor == len(s.cfg.MCPServers) {
		cursor = "> "
	}
	addLine := cursor + "+ Add new MCP server"
	if s.cursor == len(s.cfg.MCPServers) {
		b.WriteString(styles.AskSelStyle.Render(addLine))
	} else {
		b.WriteString(styles.DimStyle.Render(addLine))
	}
	b.WriteString("\n\n")
	b.WriteString(styles.DimStyle.Render("Enter: edit  d: delete  Esc: back"))

	return b.String()
}

// -----------------------------------------------------------------------
// MCP Form Screen
// -----------------------------------------------------------------------

type mcpFormScreen struct {
	mcp  *engine.MCPConfig
	form *FormModel
}

func newMCPFormScreen(m *engine.MCPConfig, isNew bool) *mcpFormScreen {
	nameField := NewTextField("Name", "e.g. my-mcp", true)
	transportField := NewSelectField("Transport", []string{"stdio", "sse"})
	commandField := NewTextField("Command", "e.g. npx mcp-server", false)
	argsField := NewTextField("Args", "space-separated args", false)
	urlField := NewTextField("URL", "e.g. https://mcp.example.com/sse", false)

	// Pre-fill.
	if m.Name != "" {
		nameField.SetValue(m.Name)
	}
	if m.URL != "" {
		transportField.SetValue("sse")
		urlField.SetValue(m.URL)
	} else {
		transportField.SetValue("stdio")
		if m.Command != "" {
			commandField.SetValue(m.Command)
		}
		if len(m.Args) > 0 {
			argsField.SetValue(strings.Join(m.Args, " "))
		}
	}

	title := "Edit MCP Server"
	if isNew {
		title = "Add MCP Server"
	}

	form := NewFormModel(title, []FormField{
		nameField, transportField, commandField, argsField, urlField,
	})

	return &mcpFormScreen{mcp: m, form: form}
}

func (s *mcpFormScreen) init() tea.Cmd {
	return s.form.Init()
}

func (s *mcpFormScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg.(type) {
	case formSubmitMsg:
		s.applyToMCP()
		return nil, nil
	case formCancelMsg:
		return nil, nil
	}

	_, cmd := s.form.Update(msg)
	return s, cmd
}

func (s *mcpFormScreen) applyToMCP() {
	s.mcp.Name = s.form.Fields[0].Value()
	transport := s.form.Fields[1].Value()

	if transport == "sse" {
		s.mcp.Command = ""
		s.mcp.Args = nil
		s.mcp.URL = s.form.Fields[4].Value()
	} else {
		s.mcp.URL = ""
		s.mcp.Command = s.form.Fields[2].Value()
		argsStr := strings.TrimSpace(s.form.Fields[3].Value())
		if argsStr != "" {
			s.mcp.Args = strings.Fields(argsStr)
		} else {
			s.mcp.Args = nil
		}
	}
}

func (s *mcpFormScreen) View() string {
	return s.form.View().Content
}
