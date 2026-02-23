package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/agentctx"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	shellyexec "github.com/germanamz/shelly/pkg/codingtoolbox/exec"
	"github.com/germanamz/shelly/pkg/codingtoolbox/filesystem"
	shellygit "github.com/germanamz/shelly/pkg/codingtoolbox/git"
	shellyhttp "github.com/germanamz/shelly/pkg/codingtoolbox/http"
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
	cfg        Config
	events     *EventBus
	store      *state.Store
	taskStore  *tasks.Store
	responder  *ask.Responder
	registry   *agent.Registry
	completers map[string]modeladapter.Completer
	toolboxes  map[string]*toolbox.ToolBox
	mcpClients []*mcpclient.MCPClient
	dir        shellydir.Dir
	projectCtx projectctx.Context
	skills     []skill.Skill

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

	e := &Engine{
		cfg:        cfg,
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
	if skills, err := skill.LoadDir(dir.SkillsDir()); err == nil {
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

	// Resolve permissions file path: prefer shellydir, fall back to config.
	permFile := dir.PermissionsPath()
	if !dir.Exists() && cfg.Filesystem.PermissionsFile != "" {
		permFile = cfg.Filesystem.PermissionsFile
	}

	// Create permission-gated tools only if referenced by at least one agent.
	permToolboxes := []string{"filesystem", "exec", "search", "git", "http"}
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

	// Use engine-level skills loaded from .shelly/skills/.
	skills := e.skills

	// If any loaded skills have descriptions, create a Store and add its
	// load_skill toolbox so the agent can retrieve full content on demand.
	for _, s := range skills {
		if s.HasDescription() {
			store := skill.NewStore(skills)
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
			contextWindow = pc.ContextWindow
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
		// Backward compat: auto-generate a compact effect from context_threshold.
		effectConfigs = []EffectConfig{{
			Kind:   "compact",
			Params: map[string]any{"threshold": contextThreshold},
		}}
	}

	wctx := EffectWiringContext{
		ContextWindow: contextWindow,
		AgentName:     ac.Name,
		AskFunc:       e.responder.Ask,
		NotifyFunc:    notifyFn,
	}

	agentEffects, err := buildEffects(effectConfigs, wctx)
	if err != nil {
		return fmt.Errorf("engine: agent %q: %w", ac.Name, err)
	}

	// Capture values for factory closure.
	name := ac.Name
	desc := ac.Description
	instr := ac.Instructions
	opts := agent.Options{
		MaxIterations:      ac.Options.MaxIterations,
		MaxDelegationDepth: ac.Options.MaxDelegationDepth,
		Skills:             skills,
		Effects:            agentEffects,
		Context:            ctxStr,
	}

	e.registry.Register(name, desc, func() *agent.Agent {
		a := agent.New(name, desc, instr, completer, opts)
		a.AddToolBoxes(tbs...)
		return a
	})

	return nil
}
