package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/projectctx"
	"github.com/germanamz/shelly/pkg/sessions"
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
	sessionStore   *sessions.Store
	store          *state.Store
	taskStore      *tasks.Store
	responder      *ask.Responder
	registry       *agent.Registry
	completers     map[string]modeladapter.Completer
	toolboxes      map[string]*toolbox.ToolBox
	mcpClients     []*mcpclient.Client
	dir            shellydir.Dir
	projectCtx     projectctx.Context
	knowledgeStale bool
	skills         []skill.Skill

	mu        sync.RWMutex
	sessions  map[string]*Session
	nextID    int
	closed    bool
	wg        sync.WaitGroup
	closeOnce sync.Once

	cancelMu     sync.Mutex
	agentCancels map[string]context.CancelFunc
}

// New creates an Engine from the given configuration. It validates the config,
// creates provider adapters, connects MCP clients, loads skills, and registers
// agent factories.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	status := func(msg string) {
		if cfg.StatusFunc != nil {
			cfg.StatusFunc(msg)
		}
	}

	// Resolve .shelly/ directory.
	shellyDirPath := cfg.ShellyDir
	if shellyDirPath == "" {
		shellyDirPath = ".shelly"
	}

	dir := shellydir.New(shellyDirPath)

	ctx, cancel := context.WithCancel(ctx)

	e := &Engine{
		cfg:          cfg,
		cancel:       cancel,
		events:       NewEventBus(),
		registry:     agent.NewRegistry(),
		completers:   make(map[string]modeladapter.Completer, len(cfg.Providers)),
		toolboxes:    make(map[string]*toolbox.ToolBox),
		sessions:     make(map[string]*Session),
		dir:          dir,
		agentCancels: make(map[string]context.CancelFunc),
	}

	e.sessionStore = sessions.New(dir.SessionsDir())

	// Bootstrap .shelly/ directory structure.
	if dir.Exists() {
		if err := shellydir.EnsureStructure(dir); err != nil {
			return nil, fmt.Errorf("engine: shelly dir: %w", err)
		}

		if err := shellydir.MigratePermissions(dir); err != nil {
			return nil, fmt.Errorf("engine: migrate permissions: %w", err)
		}
	}

	// Load skills, project context, and MCP connections in parallel.
	if err := e.parallelInit(ctx, cfg, dir, status); err != nil {
		return nil, err
	}

	// Build provider completers.
	for _, pc := range cfg.Providers {
		status(fmt.Sprintf("Initializing provider %q...", pc.Name))
		c, err := buildCompleter(pc)
		if err != nil {
			return nil, fmt.Errorf("engine: provider %q: %w", pc.Name, err)
		}
		e.completers[pc.Name] = c
	}

	// Wire built-in toolboxes (ask, state, tasks, notes, filesystem, etc.).
	if err := e.wireBuiltinToolboxes(cfg, dir); err != nil {
		_ = e.Close()
		return nil, err
	}

	// Register agent factories.
	status("Registering agents...")
	for _, ac := range cfg.Agents {
		if err := e.registerAgent(ac); err != nil {
			_ = e.Close()
			return nil, err
		}
	}

	status("Ready")

	return e, nil
}

// Events returns the engine's event bus.
func (e *Engine) Events() *EventBus { return e.events }

// KnowledgeStale reports whether the knowledge graph is outdated relative to
// the latest git commit. The check runs during engine initialization.
func (e *Engine) KnowledgeStale() bool { return e.knowledgeStale }

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

	// Perform expensive work outside the lock.
	a := factory()
	a.SetRegistry(e.registry)
	a.Init()

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil, fmt.Errorf("engine: closed")
	}
	e.nextID++
	id := fmt.Sprintf("session-%d", e.nextID)

	s := newSession(id, a, e, e.events, e.responder)
	s.providerInfo = e.resolveProviderInfo(agentName)
	e.wireAutoSave(s)

	e.sessions[id] = s
	e.mu.Unlock()

	return s, nil
}

// ResumeSession loads a previously persisted session from disk and creates a
// new live session with the restored messages. The persistID is reused so
// subsequent saves overwrite the same file.
func (e *Engine) ResumeSession(persistID string) (*Session, error) {
	info, msgs, err := e.sessionStore.Load(persistID)
	if err != nil {
		return nil, fmt.Errorf("engine: load session: %w", err)
	}

	factory, ok := e.registry.Get(info.Agent)
	if !ok {
		return nil, fmt.Errorf("engine: agent %q not found (session references unavailable agent)", info.Agent)
	}

	a := factory()
	a.SetRegistry(e.registry)
	a.Init()

	// Skip the persisted system prompt (index 0) — the freshly initialized
	// agent already has its own system prompt. Append only the non-system
	// messages.
	if len(msgs) > 1 {
		a.Chat().Append(msgs[1:]...)
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil, fmt.Errorf("engine: closed")
	}
	e.nextID++
	id := fmt.Sprintf("session-%d", e.nextID)

	s := newSession(id, a, e, e.events, e.responder)
	s.persistID = info.ID
	s.createdAt = info.CreatedAt
	s.providerInfo = e.resolveProviderInfo(info.Agent)
	e.wireAutoSave(s)

	e.sessions[id] = s
	e.mu.Unlock()

	return s, nil
}

// Session returns an existing session by ID.
func (e *Engine) Session(id string) (*Session, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

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

// resolveProviderInfo looks up the ProviderConfig for the given agent and
// returns a ProviderInfo with Kind and Model.
func (e *Engine) resolveProviderInfo(agentName string) ProviderInfo {
	var providerName string
	for _, ac := range e.cfg.Agents {
		if ac.Name == agentName {
			providerName = ac.Provider
			break
		}
	}
	if providerName == "" && len(e.cfg.Providers) > 0 {
		providerName = e.cfg.Providers[0].Name
	}
	for _, pc := range e.cfg.Providers {
		if pc.Name == providerName {
			return ProviderInfo{Kind: pc.Kind, Model: pc.Model}
		}
	}
	return ProviderInfo{}
}

// SessionStore returns the session persistence store.
func (e *Engine) SessionStore() *sessions.Store { return e.sessionStore }

// wireAutoSave sets the onSendComplete callback on a session to persist it
// after each successful Send or Compact.
func (e *Engine) wireAutoSave(s *Session) {
	s.onSendComplete = func() {
		if err := e.saveSession(s); err != nil {
			slog.Warn("engine: auto-save session failed", "session", s.persistID, "error", err)
		}
	}
}

// saveSession persists the session's metadata and messages to disk.
func (e *Engine) saveSession(s *Session) error {
	ch := s.Chat()
	msgs := ch.Messages()

	var preview string
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == role.User {
			preview = msgs[i].TextContent()
			break
		}
	}
	if len(preview) > 100 {
		preview = preview[:100]
	}

	info := sessions.SessionInfo{
		ID:    s.persistID,
		Agent: s.AgentName(),
		Provider: sessions.ProviderMeta{
			Kind:  s.providerInfo.Kind,
			Model: s.providerInfo.Model,
		},
		CreatedAt: s.createdAt,
		UpdatedAt: time.Now(),
		Preview:   preview,
		MsgCount:  ch.Len(),
	}

	return e.sessionStore.Save(info, msgs)
}

// sessionLifecycle is the subset of Engine that Session needs for coordinating
// shutdown. Session holds this interface instead of *Engine to avoid reaching
// into Engine's internal fields.
type sessionLifecycle interface {
	acquireSend() error
	releaseSend()
}

// acquireSend checks that the engine is not closed and increments the in-flight
// send counter. Returns an error if the engine is closed.
func (e *Engine) acquireSend() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("engine: closed")
	}
	e.wg.Add(1)
	return nil
}

// releaseSend decrements the in-flight send counter.
func (e *Engine) releaseSend() {
	e.wg.Done()
}

// RegisterAgentCancel stores a cancel function for the named agent.
func (e *Engine) RegisterAgentCancel(name string, cancel context.CancelFunc) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	e.agentCancels[name] = cancel
}

// UnregisterAgentCancel removes a previously registered cancel function.
func (e *Engine) UnregisterAgentCancel(name string) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	delete(e.agentCancels, name)
}

// CancelAgent cancels the named agent's context and unregisters it. Returns
// true if the agent was found and cancelled.
func (e *Engine) CancelAgent(name string) bool {
	e.cancelMu.Lock()
	cancel, ok := e.agentCancels[name]
	if ok {
		delete(e.agentCancels, name)
	}
	e.cancelMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// Close cancels the engine context, shuts down MCP clients, and releases
// resources. Callers should ensure all active sessions have drained before
// calling Close, as session cancellation depends on the caller-provided
// context passed to Send/SendParts.
func (e *Engine) Close() error {
	var firstErr error
	e.closeOnce.Do(func() {
		e.mu.Lock()
		e.closed = true
		e.mu.Unlock()

		// Wait for all in-flight session sends to finish before tearing down.
		e.wg.Wait()

		if e.cancel != nil {
			e.cancel()
		}

		for _, c := range e.mcpClients {
			if err := c.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	})
	return firstErr
}
