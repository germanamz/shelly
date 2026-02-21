package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/skill"
	"github.com/germanamz/shelly/pkg/state"
	"github.com/germanamz/shelly/pkg/tools/ask"
	"github.com/germanamz/shelly/pkg/tools/mcpclient"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Engine is the composition root that assembles all framework components from
// configuration and exposes them through a frontend-agnostic API.
type Engine struct {
	cfg        Config
	events     *EventBus
	store      *state.Store
	responder  *ask.Responder
	registry   *agent.Registry
	completers map[string]modeladapter.Completer
	toolboxes  map[string]*toolbox.ToolBox
	mcpClients []*mcpclient.MCPClient

	mu       sync.Mutex
	sessions map[string]*Session
	nextID   int
}

// New creates an Engine from the given configuration. It validates the config,
// creates provider adapters, connects MCP clients, loads skills, and registers
// agent factories.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	e := &Engine{
		cfg:        cfg,
		events:     NewEventBus(),
		registry:   agent.NewRegistry(),
		completers: make(map[string]modeladapter.Completer, len(cfg.Providers)),
		toolboxes:  make(map[string]*toolbox.ToolBox),
		sessions:   make(map[string]*Session),
	}

	// Build provider completers.
	for _, pc := range cfg.Providers {
		c, err := buildCompleter(pc)
		if err != nil {
			return nil, fmt.Errorf("engine: provider %q: %w", pc.Name, err)
		}
		e.completers[pc.Name] = c
	}

	// Connect MCP clients and build toolboxes.
	for _, mc := range cfg.MCPServers {
		client, err := mcpclient.New(ctx, mc.Command, mc.Args...)
		if err != nil {
			_ = e.Close()
			return nil, fmt.Errorf("engine: mcp %q: %w", mc.Name, err)
		}
		e.mcpClients = append(e.mcpClients, client)

		tools, err := client.ListTools(ctx)
		if err != nil {
			_ = e.Close()
			return nil, fmt.Errorf("engine: mcp %q: list tools: %w", mc.Name, err)
		}

		tb := toolbox.New()
		tb.Register(tools...)
		e.toolboxes[mc.Name] = tb
	}

	// Create state store if enabled.
	if cfg.StateEnabled {
		e.store = &state.Store{}
		e.toolboxes["state"] = e.store.Tools("shared")
	}

	// Create ask responder.
	e.responder = ask.NewResponder(func(ctx context.Context, q ask.Question) {
		sid, _ := sessionIDFromContext(ctx)
		aname, _ := agentNameFromContext(ctx)
		e.events.Publish(Event{
			Kind:      EventAskUser,
			SessionID: sid,
			Agent:     aname,
			Timestamp: time.Now(),
			Data:      q,
		})
	})
	e.toolboxes["ask"] = e.responder.Tools()

	// Register agent factories.
	for _, ac := range cfg.Agents {
		if err := e.registerAgent(ac); err != nil {
			_ = e.Close()
			return nil, err
		}
	}

	return e, nil
}

// Events returns the engine's event bus.
func (e *Engine) Events() *EventBus { return e.events }

// State returns the shared state store, or nil if state is not enabled.
func (e *Engine) State() *state.Store { return e.store }

// NewSession creates a new interactive session. If agentName is empty the
// config's EntryAgent is used. If EntryAgent is also empty, the first agent
// in the config is used.
func (e *Engine) NewSession(agentName string) (*Session, error) {
	if agentName == "" {
		agentName = e.cfg.EntryAgent
	}
	if agentName == "" && len(e.cfg.Agents) > 0 {
		agentName = e.cfg.Agents[0].Name
	}

	factory, ok := e.registry.Get(agentName)
	if !ok {
		return nil, fmt.Errorf("engine: agent %q not found", agentName)
	}

	e.mu.Lock()
	e.nextID++
	id := fmt.Sprintf("session-%d", e.nextID)
	e.mu.Unlock()

	a := factory()
	a.SetRegistry(e.registry)

	s := newSession(id, a, e.events, e.responder)

	e.mu.Lock()
	e.sessions[id] = s
	e.mu.Unlock()

	return s, nil
}

// Session returns an existing session by ID.
func (e *Engine) Session(id string) (*Session, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[id]
	return s, ok
}

// Close shuts down MCP clients and releases resources.
func (e *Engine) Close() error {
	var firstErr error
	for _, c := range e.mcpClients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// registerAgent creates a factory for the given agent config and registers it.
func (e *Engine) registerAgent(ac AgentConfig) error {
	// Resolve provider — default to first provider.
	providerName := ac.Provider
	if providerName == "" && len(e.cfg.Providers) > 0 {
		providerName = e.cfg.Providers[0].Name
	}

	completer, ok := e.completers[providerName]
	if !ok {
		return fmt.Errorf("engine: agent %q: provider %q not found", ac.Name, providerName)
	}

	// Collect toolboxes.
	var tbs []*toolbox.ToolBox
	for _, name := range ac.ToolBoxNames {
		tb, ok := e.toolboxes[name]
		if !ok {
			return fmt.Errorf("engine: agent %q: toolbox %q not found", ac.Name, name)
		}
		tbs = append(tbs, tb)
	}

	// Load skills.
	var skills []skill.Skill
	if ac.SkillsDir != "" {
		var err error
		skills, err = skill.LoadDir(ac.SkillsDir)
		if err != nil {
			return fmt.Errorf("engine: agent %q: %w", ac.Name, err)
		}
	}

	// Collect all tool declarations for ToolAware completers.
	var allTools []toolbox.Tool
	for _, tb := range tbs {
		allTools = append(allTools, tb.Tools()...)
	}

	// Set tools on ToolAware completers.
	if ta, ok := completer.(modeladapter.ToolAware); ok && len(allTools) > 0 {
		ta.SetTools(allTools)
	}

	// Capture values for factory closure.
	name := ac.Name
	desc := ac.Description
	instr := ac.Instructions
	opts := agent.Options{
		MaxIterations:      ac.Options.MaxIterations,
		MaxDelegationDepth: ac.Options.MaxDelegationDepth,
		Skills:             skills,
	}

	e.registry.Register(name, desc, func() *agent.Agent {
		a := agent.New(name, desc, instr, completer, opts)
		a.AddToolBoxes(tbs...)
		return a
	})

	return nil
}
