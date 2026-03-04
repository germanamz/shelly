package engine

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/skill"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// registrationContext groups intermediate resolved state used while registering
// an agent factory.
type registrationContext struct {
	name          string
	desc          string
	instr         string
	prefix        string
	completer     modeladapter.Completer
	toolboxes     []*toolbox.ToolBox
	skills        []skill.Skill
	contextStr    string
	contextWindow int
	effectConfigs []EffectConfig
	wiringCtx     EffectWiringContext
	eventNotifier agent.EventNotifier
	eventFunc     agent.EventFunc
	reflectionDir string
	taskBoard     agent.TaskBoard
	maxIter       int
	maxDepth      int
}

// registerAgent creates a factory for the given agent config and registers it.
func (e *Engine) registerAgent(ac AgentConfig) error {
	rc, err := e.buildRegistrationContext(ac)
	if err != nil {
		return err
	}

	// Validate effect configs eagerly so registration fails fast on bad config.
	if _, err := buildEffects(rc.effectConfigs, rc.wiringCtx); err != nil {
		return fmt.Errorf("engine: agent %q: %w", ac.Name, err)
	}

	return e.registerFactory(rc)
}

// buildRegistrationContext resolves all configuration for a single agent.
func (e *Engine) buildRegistrationContext(ac AgentConfig) (registrationContext, error) {
	completer, err := e.resolveCompleter(ac)
	if err != nil {
		return registrationContext{}, err
	}

	tbs, err := e.collectToolboxes(ac)
	if err != nil {
		return registrationContext{}, err
	}

	skills := e.filterSkills(ac)
	tbs = e.appendSkillToolbox(skills, tbs)

	contextWindow := e.resolveAgentContextWindow(ac)

	// Default threshold to 0.8 when context window is set but threshold is not.
	contextThreshold := ac.Options.ContextThreshold
	if contextWindow > 0 && contextThreshold == 0 {
		contextThreshold = 0.8
	}

	// Build notify function for compaction events.
	var notifyFn func(ctx context.Context, msg string)
	if contextWindow > 0 {
		notifyFn = func(ctx context.Context, msg string) {
			publishFromContext(e.events, ctx, EventCompaction, msg)
		}
	}

	effectConfigs := e.buildEffectConfigs(ac, contextWindow, contextThreshold)

	wctx := EffectWiringContext{
		ContextWindow: contextWindow,
		AgentName:     ac.Name,
		StorageDir:    e.effectStorageDir(ac.Name),
		AskFunc:       e.responder.Ask,
		NotifyFunc:    notifyFn,
	}

	// Resolve reflection directory (enabled when .shelly/ exists).
	var reflectionDir string
	if e.dir.Exists() {
		reflectionDir = e.dir.ReflectionsDir()
	}

	// Wire task board adapter if the task store is available.
	var taskBoard agent.TaskBoard
	if e.taskStore != nil {
		taskBoard = &taskBoardAdapter{store: e.taskStore}
	}

	return registrationContext{
		name:          ac.Name,
		desc:          ac.Description,
		instr:         ac.Instructions,
		prefix:        ac.Prefix,
		completer:     completer,
		toolboxes:     tbs,
		skills:        skills,
		contextStr:    e.projectCtx.String(),
		contextWindow: contextWindow,
		effectConfigs: effectConfigs,
		wiringCtx:     wctx,
		eventNotifier: e.buildAgentEventNotifier(),
		eventFunc:     e.buildAgentEventFunc(),
		reflectionDir: reflectionDir,
		taskBoard:     taskBoard,
		maxIter:       ac.Options.MaxIterations,
		maxDepth:      ac.Options.MaxDelegationDepth,
	}, nil
}

// resolveCompleter finds the completer for the agent's provider reference.
func (e *Engine) resolveCompleter(ac AgentConfig) (modeladapter.Completer, error) {
	providerName := ac.Provider
	if providerName == "" && len(e.cfg.Providers) > 0 {
		providerName = e.cfg.Providers[0].Name
	}

	completer, ok := e.completers[providerName]
	if !ok {
		return nil, fmt.Errorf("engine: agent %q: provider %q not found", ac.Name, providerName)
	}
	return completer, nil
}

// collectToolboxes gathers the agent's declared toolboxes (always including ask).
func (e *Engine) collectToolboxes(ac AgentConfig) ([]*toolbox.ToolBox, error) {
	var tbs []*toolbox.ToolBox
	if askTB, ok := e.toolboxes["ask"]; ok {
		tbs = append(tbs, askTB)
	}

	seen := map[string]struct{}{"ask": {}}
	for _, ref := range ac.Toolboxes {
		if _, dup := seen[ref.Name]; dup {
			continue
		}
		seen[ref.Name] = struct{}{}

		tb, ok := e.toolboxes[ref.Name]
		if !ok {
			return nil, fmt.Errorf("engine: agent %q: toolbox %q not found", ac.Name, ref.Name)
		}
		tbs = append(tbs, tb.Filter(ref.Tools))
	}
	return tbs, nil
}

// filterSkills returns engine-level skills optionally filtered by per-agent config.
func (e *Engine) filterSkills(ac AgentConfig) []skill.Skill {
	if len(ac.Skills) == 0 {
		return e.skills
	}
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
	return filtered
}

// appendSkillToolbox adds the load_skill toolbox if any skill has a description.
func (e *Engine) appendSkillToolbox(skills []skill.Skill, tbs []*toolbox.ToolBox) []*toolbox.ToolBox {
	for _, s := range skills {
		if s.HasDescription() {
			store := skill.NewStore(skills, filepath.Dir(e.dir.Root()))
			return append(tbs, store.Tools())
		}
	}
	return tbs
}

// resolveAgentContextWindow finds the context window for the agent's provider.
func (e *Engine) resolveAgentContextWindow(ac AgentConfig) int {
	providerName := ac.Provider
	if providerName == "" && len(e.cfg.Providers) > 0 {
		providerName = e.cfg.Providers[0].Name
	}
	for _, pc := range e.cfg.Providers {
		if pc.Name == providerName {
			return resolveContextWindow(pc, e.cfg.DefaultContextWindows)
		}
	}
	return 0
}

// buildEffectConfigs returns explicit effect configs or auto-generates defaults.
func (e *Engine) buildEffectConfigs(ac AgentConfig, contextWindow int, contextThreshold float64) []EffectConfig {
	if len(ac.Effects) > 0 {
		return ac.Effects
	}
	if contextWindow > 0 {
		return []EffectConfig{
			{Kind: "trim_tool_results"},
			{Kind: "observation_mask", Params: map[string]any{"threshold": 0.5}},
			{Kind: "compact", Params: map[string]any{"threshold": contextThreshold}},
		}
	}
	return nil
}

// buildAgentEventNotifier creates an EventNotifier that publishes sub-agent
// lifecycle events to the engine's event bus.
func (e *Engine) buildAgentEventNotifier() agent.EventNotifier {
	return agent.EventNotifier(func(ctx context.Context, kind string, agentName string, data any) {
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
		e.events.publish(ek, sid, agentName, data)
	})
}

// buildAgentEventFunc creates an EventFunc that publishes fine-grained loop
// events to the engine's event bus.
func (e *Engine) buildAgentEventFunc() agent.EventFunc {
	return agent.EventFunc(func(ctx context.Context, kind string, data any) {
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
		publishFromContext(e.events, ctx, ek, data)
	})
}

// registerFactory captures registration context into a factory closure and
// registers it with the agent registry.
func (e *Engine) registerFactory(rc registrationContext) error {
	e.registry.Register(rc.name, rc.desc, func() *agent.Agent {
		// Build fresh effects for each agent instance so stateful effects
		// (e.g. SlidingWindowEffect, ReflectionEffect, LoopDetectEffect)
		// are not shared across agents created by the same factory.
		agentEffects, bErr := buildEffects(rc.effectConfigs, rc.wiringCtx)
		if bErr != nil {
			panic(fmt.Sprintf("engine: agent %q: buildEffects failed after validation: %v", rc.name, bErr))
		}

		opts := agent.Options{
			MaxIterations:      rc.maxIter,
			MaxDelegationDepth: rc.maxDepth,
			Skills:             rc.skills,
			Effects:            agentEffects,
			Context:            rc.contextStr,
			EventNotifier:      rc.eventNotifier,
			EventFunc:          rc.eventFunc,
			ReflectionDir:      rc.reflectionDir,
			Prefix:             rc.prefix,
			TaskBoard:          rc.taskBoard,
		}

		a := agent.New(rc.name, rc.desc, rc.instr, rc.completer, opts)
		a.AddToolBoxes(rc.toolboxes...)
		return a
	})

	return nil
}

// effectStorageDir returns a per-agent directory for effects that need
// persistent storage (e.g. offload). Returns empty string when the .shelly
// directory does not exist.
func (e *Engine) effectStorageDir(agentName string) string {
	if !e.dir.Exists() {
		return ""
	}
	return filepath.Join(e.dir.Root(), "offload", agentName)
}
