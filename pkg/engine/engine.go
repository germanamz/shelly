package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/agentctx"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	shellybrowser "github.com/germanamz/shelly/pkg/codingtoolbox/browser"
	shellyexec "github.com/germanamz/shelly/pkg/codingtoolbox/exec"
	"github.com/germanamz/shelly/pkg/codingtoolbox/filesystem"
	shellygit "github.com/germanamz/shelly/pkg/codingtoolbox/git"
	shellyhttp "github.com/germanamz/shelly/pkg/codingtoolbox/http"
	"github.com/germanamz/shelly/pkg/codingtoolbox/notes"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/codingtoolbox/search"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/projectctx"
	"github.com/germanamz/shelly/pkg/shellydir"
	"github.com/germanamz/shelly/pkg/skill"
	"github.com/germanamz/shelly/pkg/state"
	"github.com/germanamz/shelly/pkg/tasks"
	"github.com/germanamz/shelly/pkg/tools/mcpclient"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Engine is the composition root that assembles all framework components from
// configuration and exposes them through a frontend-agnostic API.
type Engine struct {
	cfg            Config
	cancel         context.CancelFunc
	events         *EventBus
	store          *state.Store
	taskStore      *tasks.Store
	responder      *ask.Responder
	registry       *agent.Registry
	completers     map[string]modeladapter.Completer
	toolboxes      map[string]*toolbox.ToolBox
	mcpClients     []*mcpclient.MCPClient
	browserToolbox *shellybrowser.Browser
	dir            shellydir.Dir
	projectCtx     projectctx.Context
	skills         []skill.Skill

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

	// Resolve .shelly/ directory.
	shellyDirPath := cfg.ShellyDir
	if shellyDirPath == "" {
		shellyDirPath = ".shelly"
	}

	dir := shellydir.New(shellyDirPath)

	ctx, cancel := context.WithCancel(ctx)

	e := &Engine{
		cfg:        cfg,
		cancel:     cancel,
		events:     NewEventBus(),
		registry:   agent.NewRegistry(),
		completers: make(map[string]modeladapter.Completer, len(cfg.Providers)),
		toolboxes:  make(map[string]*toolbox.ToolBox),
		sessions:   make(map[string]*Session),
		dir:        dir,
	}

	// Bootstrap .shelly/ directory structure.
	if dir.Exists() {
		if err := shellydir.EnsureStructure(dir); err != nil {
			return nil, fmt.Errorf("engine: shelly dir: %w", err)
		}

		if err := shellydir.MigratePermissions(dir); err != nil {
			return nil, fmt.Errorf("engine: migrate permissions: %w", err)
		}
	}

	// Load skills once at engine level from .shelly/skills/.
	skillsDir := dir.SkillsDir()
	if _, err := os.Stat(skillsDir); err == nil {
		skills, err := skill.LoadDir(skillsDir)
		if err != nil {
			return nil, fmt.Errorf("engine: skills: %w", err)
		}
		e.skills = skills
	}

	// Load project context (best-effort).
	e.projectCtx = projectctx.Load(dir, filepath.Dir(dir.Root()))

	// Build provider completers.
	for _, pc := range cfg.Providers {
		c, err := buildCompleter(pc)
		if err != nil {
			return nil, fmt.Errorf("engine: provider %q: %w", pc.Name, err)
		}
		e.completers[pc.Name] = c
	}

	// Connect MCP clients and build toolboxes.
	if err := e.connectMCPClients(ctx, cfg.MCPServers); err != nil {
		return nil, err
	}

	// Create ask responder (always available).
	e.responder = ask.NewResponder(func(ctx context.Context, q ask.Question) {
		sid, _ := sessionIDFromContext(ctx)
		aname := agentctx.AgentNameFromContext(ctx)
		e.events.Publish(Event{
			Kind:      EventAskUser,
			SessionID: sid,
			Agent:     aname,
			Timestamp: time.Now(),
			Data:      q,
		})
	})
	e.toolboxes["ask"] = e.responder.Tools()

	// Determine which built-in toolboxes are referenced by at least one agent.
	refs := referencedBuiltins(cfg.Agents)

	// Create state store if referenced.
	if _, ok := refs["state"]; ok {
		e.store = &state.Store{}
		e.toolboxes["state"] = e.store.Tools("shared")
	}

	// Create task store if referenced.
	if _, ok := refs["tasks"]; ok {
		e.taskStore = &tasks.Store{}
		e.toolboxes["tasks"] = e.taskStore.Tools("shared")
	}

	// Create notes store if referenced. Notes persist in .shelly/notes/.
	if _, ok := refs["notes"]; ok {
		notesDir := filepath.Join(dir.Root(), "notes")
		notesStore := notes.New(notesDir)
		e.toolboxes["notes"] = notesStore.Tools()
	}

	// Resolve permissions file path: prefer shellydir, fall back to config.
	permFile := dir.PermissionsPath()
	if !dir.Exists() && cfg.Filesystem.PermissionsFile != "" {
		permFile = cfg.Filesystem.PermissionsFile
	}

	// Create permission-gated tools only if referenced by at least one agent.
	permToolboxes := []string{"filesystem", "exec", "search", "git", "http", "browser"}
	needsPerm := false
	for _, name := range permToolboxes {
		if _, ok := refs[name]; ok {
			needsPerm = true
			break
		}
	}

	if needsPerm {
		permStore, err := permissions.New(permFile)
		if err != nil {
			_ = e.Close()
			return nil, fmt.Errorf("engine: permissions: %w", err)
		}

		// Pre-approve the process CWD so sub-agents inherit filesystem
		// access to the working directory without being prompted.
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			_ = permStore.ApproveDir(cwd)
		}

		if _, ok := refs["filesystem"]; ok {
			notifyFn := func(ctx context.Context, message string) {
				sid, _ := sessionIDFromContext(ctx)
				aname := agentctx.AgentNameFromContext(ctx)
				e.events.Publish(Event{
					Kind:      EventFileChange,
					SessionID: sid,
					Agent:     aname,
					Timestamp: time.Now(),
					Data:      message,
				})
			}
			fsTools := filesystem.New(permStore, e.responder.Ask, notifyFn)
			e.toolboxes["filesystem"] = fsTools.Tools()
		}

		if _, ok := refs["exec"]; ok {
			execTools := shellyexec.New(permStore, e.responder.Ask)
			e.toolboxes["exec"] = execTools.Tools()
		}

		if _, ok := refs["search"]; ok {
			searchTools := search.New(permStore, e.responder.Ask)
			e.toolboxes["search"] = searchTools.Tools()
		}

		if _, ok := refs["git"]; ok {
			gitTools := shellygit.New(permStore, e.responder.Ask, cfg.Git.WorkDir)
			e.toolboxes["git"] = gitTools.Tools()
		}

		if _, ok := refs["http"]; ok {
			httpTools := shellyhttp.New(permStore, e.responder.Ask)
			e.toolboxes["http"] = httpTools.Tools()
		}

		if _, ok := refs["browser"]; ok {
			var browserOpts []shellybrowser.Option
			if cfg.Browser.Headless {
				browserOpts = append(browserOpts, shellybrowser.WithHeadless())
			}
			bt := shellybrowser.New(ctx, permStore, e.responder.Ask, browserOpts...)
			e.toolboxes["browser"] = bt.Tools()
			e.browserToolbox = bt
		}
	}

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

// Tasks returns the shared task store, or nil if tasks are not enabled.
func (e *Engine) Tasks() *tasks.Store { return e.taskStore }

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
	a.Init()

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

// RemoveSession removes a session from the engine. Returns true if the session
// existed and was removed, false if no session with that ID was found. The
// caller is responsible for ensuring the session is no longer active before
// removing it.
func (e *Engine) RemoveSession(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, ok := e.sessions[id]
	if ok {
		delete(e.sessions, id)
	}
	return ok
}

// connectMCPClients connects to all configured MCP servers and populates toolboxes.
func (e *Engine) connectMCPClients(ctx context.Context, servers []MCPConfig) error {
	for _, mc := range servers {
		var client *mcpclient.MCPClient
		var err error
		if mc.URL != "" {
			client, err = mcpclient.NewHTTP(ctx, mc.URL)
		} else {
			client, err = mcpclient.New(ctx, mc.Command, mc.Args...)
		}
		if err != nil {
			_ = e.Close()
			return fmt.Errorf("engine: mcp %q: %w", mc.Name, err)
		}
		e.mcpClients = append(e.mcpClients, client)

		tools, err := client.ListTools(ctx)
		if err != nil {
			_ = e.Close()
			return fmt.Errorf("engine: mcp %q: list tools: %w", mc.Name, err)
		}

		tb := toolbox.New()
		tb.Register(tools...)
		e.toolboxes[mc.Name] = tb
	}
	return nil
}

// Close cancels the engine context, shuts down MCP clients, and releases
// resources. Callers should ensure all active sessions have drained before
// calling Close, as session cancellation depends on the caller-provided
// context passed to Send/SendParts.
func (e *Engine) Close() error {
	if e.cancel != nil {
		e.cancel()
	}

	if e.browserToolbox != nil {
		e.browserToolbox.Close()
	}

	var firstErr error
	for _, c := range e.mcpClients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// taskBoardAdapter implements agent.TaskBoard using a *tasks.Store.
type taskBoardAdapter struct {
	store *tasks.Store
}

func (a *taskBoardAdapter) ClaimTask(id, agentName string) error {
	return a.store.Reassign(id, agentName)
}

func (a *taskBoardAdapter) UpdateTaskStatus(id, status string) error {
	s := tasks.Status(status)
	return a.store.Update(id, tasks.Update{Status: &s})
}

// builtinToolboxNames are toolbox names managed by the engine itself (not MCP).
var builtinToolboxNames = map[string]struct{}{
	"state":      {},
	"tasks":      {},
	"ask":        {},
	"filesystem": {},
	"exec":       {},
	"search":     {},
	"git":        {},
	"http":       {},
	"browser":    {},
	"notes":      {},
}

// referencedBuiltins returns the set of built-in toolbox names that appear in
// at least one agent's Toolboxes list.
func referencedBuiltins(agents []AgentConfig) map[string]struct{} {
	refs := make(map[string]struct{})
	for _, a := range agents {
		for _, name := range a.Toolboxes {
			if _, ok := builtinToolboxNames[name]; ok {
				refs[name] = struct{}{}
			}
		}
	}
	return refs
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

	// Always include the ask toolbox.
	var tbs []*toolbox.ToolBox
	if askTB, ok := e.toolboxes["ask"]; ok {
		tbs = append(tbs, askTB)
	}

	// Collect the agent's declared toolboxes, skipping ask (already added).
	seen := map[string]struct{}{"ask": {}}
	for _, name := range ac.Toolboxes {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}

		tb, ok := e.toolboxes[name]
		if !ok {
			return fmt.Errorf("engine: agent %q: toolbox %q not found", ac.Name, name)
		}
		tbs = append(tbs, tb)
	}

	// Use engine-level skills, optionally filtered by per-agent config.
	skills := e.skills
	if len(ac.Skills) > 0 {
		allowed := make(map[string]struct{}, len(ac.Skills))
		for _, name := range ac.Skills {
			allowed[name] = struct{}{}
		}
		filtered := make([]skill.Skill, 0, len(ac.Skills))
		for _, s := range e.skills {
			if _, ok := allowed[s.Name]; ok {
				filtered = append(filtered, s)
			}
		}
		skills = filtered
	}

	// If any loaded skills have descriptions, create a Store and add its
	// load_skill toolbox so the agent can retrieve full content on demand.
	for _, s := range skills {
		if s.HasDescription() {
			store := skill.NewStore(skills, filepath.Dir(e.dir.Root()))
			tbs = append(tbs, store.Tools())
			break
		}
	}

	// Compose project context string.
	ctxStr := e.projectCtx.String()

	// Resolve context window from the provider config.
	var contextWindow int
	for _, pc := range e.cfg.Providers {
		if pc.Name == providerName {
			contextWindow = resolveContextWindow(pc, e.cfg.DefaultContextWindows)
			break
		}
	}

	// Default threshold to 0.8 when context window is set but threshold is not.
	contextThreshold := ac.Options.ContextThreshold
	if contextWindow > 0 && contextThreshold == 0 {
		contextThreshold = 0.8
	}

	// Build notify function for compaction events.
	var notifyFn func(ctx context.Context, msg string)
	if contextWindow > 0 {
		notifyFn = func(ctx context.Context, msg string) {
			sid, _ := sessionIDFromContext(ctx)
			aname := agentctx.AgentNameFromContext(ctx)
			e.events.Publish(Event{
				Kind:      EventCompaction,
				SessionID: sid,
				Agent:     aname,
				Timestamp: time.Now(),
				Data:      msg,
			})
		}
	}

	// Build effects from explicit config or auto-generate from legacy options.
	effectConfigs := ac.Effects
	if len(effectConfigs) == 0 && contextWindow > 0 {
		// Auto-generate default effects: trim tool results (lightweight, runs
		// after each completion) + compact (full summarization as fallback).
		effectConfigs = []EffectConfig{
			{Kind: "trim_tool_results"},
			{Kind: "compact", Params: map[string]any{"threshold": contextThreshold}},
		}
	}

	wctx := EffectWiringContext{
		ContextWindow: contextWindow,
		AgentName:     ac.Name,
		AskFunc:       e.responder.Ask,
		NotifyFunc:    notifyFn,
	}

	// Validate effect configs eagerly so registration fails fast on bad config.
	// The actual construction happens inside the factory closure below so that
	// each agent instance gets its own fresh (non-shared) effect state.
	if _, err := buildEffects(effectConfigs, wctx); err != nil {
		return fmt.Errorf("engine: agent %q: %w", ac.Name, err)
	}

	// Build EventNotifier that publishes sub-agent lifecycle events.
	eventNotifier := agent.EventNotifier(func(ctx context.Context, kind string, agentName string, data any) {
		sid, _ := sessionIDFromContext(ctx)
		var ek EventKind
		switch kind {
		case "agent_start":
			ek = EventAgentStart
		case "agent_end":
			ek = EventAgentEnd
		default:
			return
		}
		e.events.Publish(Event{
			Kind:      ek,
			SessionID: sid,
			Agent:     agentName,
			Timestamp: time.Now(),
			Data:      data,
		})
	})

	// Build EventFunc that publishes fine-grained loop events.
	eventFunc := agent.EventFunc(func(ctx context.Context, kind string, data any) {
		sid, _ := sessionIDFromContext(ctx)
		aname := agentctx.AgentNameFromContext(ctx)
		var ek EventKind
		switch kind {
		case "tool_call_start":
			ek = EventToolCallStart
		case "tool_call_end":
			ek = EventToolCallEnd
		case "message_added":
			ek = EventMessageAdded
		default:
			return
		}
		e.events.Publish(Event{
			Kind:      ek,
			SessionID: sid,
			Agent:     aname,
			Timestamp: time.Now(),
			Data:      data,
		})
	})

	// Resolve reflection directory (enabled when .shelly/ exists).
	var reflectionDir string
	if e.dir.Exists() {
		reflectionDir = e.dir.ReflectionsDir()
	}

	// Capture values for factory closure.
	name := ac.Name
	desc := ac.Description
	instr := ac.Instructions
	prefix := ac.Prefix
	// Wire task board adapter if the task store is available.
	var taskBoard agent.TaskBoard
	if e.taskStore != nil {
		taskBoard = &taskBoardAdapter{store: e.taskStore}
	}

	e.registry.Register(name, desc, func() *agent.Agent {
		// Build fresh effects for each agent instance so stateful effects
		// (e.g. SlidingWindowEffect, ReflectionEffect, LoopDetectEffect)
		// are not shared across agents created by the same factory.
		agentEffects, _ := buildEffects(effectConfigs, wctx)

		opts := agent.Options{
			MaxIterations:      ac.Options.MaxIterations,
			MaxDelegationDepth: ac.Options.MaxDelegationDepth,
			Skills:             skills,
			Effects:            agentEffects,
			Context:            ctxStr,
			EventNotifier:      eventNotifier,
			EventFunc:          eventFunc,
			ReflectionDir:      reflectionDir,
			Prefix:             prefix,
			TaskBoard:          taskBoard,
		}

		a := agent.New(name, desc, instr, completer, opts)
		a.AddToolBoxes(tbs...)
		return a
	})

	return nil
}

// ClearState clears the shared state store if enabled.
func (e *Engine) ClearState() {
	// TODO v2: if e.store != nil { /* clear */ }
}

// ClearTasks clears the shared task store if enabled.
func (e *Engine) ClearTasks() {
	// TODO v2: if e.taskStore != nil { /* clear */ }
}
