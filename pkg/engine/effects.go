package engine

import (
	"context"
	"fmt"
	"sort"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/agent/effects"
)

// EffectWiringContext provides engine-level resources needed by effect
// factories to construct concrete Effect instances.
type EffectWiringContext struct {
	ContextWindow int
	AgentName     string
	StorageDir    string // Directory for effects that need persistent storage (e.g. offload).

	AskFunc    func(ctx context.Context, text string, options []string) (string, error)
	NotifyFunc func(ctx context.Context, message string)
}

// EffectFactory constructs an Effect from its YAML params and engine context.
type EffectFactory func(params map[string]any, wctx EffectWiringContext) (agent.Effect, error)

// effectFactories maps effect kind strings to their constructors.
var effectFactories = map[string]EffectFactory{
	"compact":           buildCompactEffect,
	"trim_tool_results": buildTrimToolResultsEffect,
	"loop_detect":       buildLoopDetectEffect,
	"sliding_window":    buildSlidingWindowEffect,
	"observation_mask":  buildObservationMaskEffect,
	"reflection":        buildReflectionEffect,
	"progress":          buildProgressEffect,
	"tool_scope":        buildToolScopeEffect,
	"offload":           buildOffloadEffect,
}

// KnownEffectKinds returns the sorted list of recognised effect kind strings,
// derived from the effect factory registry.
func KnownEffectKinds() []string {
	kinds := make([]string, 0, len(effectFactories))
	for k := range effectFactories {
		kinds = append(kinds, k)
	}

	sort.Strings(kinds)
	return kinds
}

// buildEffects constructs all effects for an agent from its config.
func buildEffects(ecs []EffectConfig, wctx EffectWiringContext) ([]agent.Effect, error) {
	if len(ecs) == 0 {
		return nil, nil
	}

	effs := make([]agent.Effect, 0, len(ecs))
	for i, ec := range ecs {
		factory, ok := effectFactories[ec.Kind]
		if !ok {
			return nil, fmt.Errorf("engine: effect[%d]: unknown kind %q", i, ec.Kind)
		}

		eff, err := factory(ec.Params, wctx)
		if err != nil {
			return nil, fmt.Errorf("engine: effect[%d] (%s): %w", i, ec.Kind, err)
		}

		effs = append(effs, eff)
	}

	sortEffects(effs)

	return effs, nil
}

// sortEffects ensures compaction-class effects run before others so that
// effects injecting messages (e.g. ReflectionEffect, LoopDetectEffect)
// are not immediately summarized away in the same iteration.
func sortEffects(effs []agent.Effect) {
	sort.SliceStable(effs, func(i, j int) bool {
		return effectPriority(effs[i]) < effectPriority(effs[j])
	})
}

// effectPriority returns 0 for compaction-class effects (which should run
// first) and 1 for everything else.
func effectPriority(e agent.Effect) int {
	switch e.(type) {
	case *effects.CompactEffect, *effects.SlidingWindowEffect, *effects.ObservationMaskEffect:
		return 0
	case *effects.ToolScopeEffect, *effects.OffloadEffect:
		return 1
	default:
		return 1
	}
}

// paramFloat extracts a float64 from a params map with a default value.
// Accepts both float64 and int values from YAML.
func paramFloat(params map[string]any, key string, def float64) (float64, error) {
	v, ok := params[key]
	if !ok {
		return def, nil
	}
	switch t := v.(type) {
	case float64:
		return t, nil
	case int:
		return float64(t), nil
	default:
		return 0, fmt.Errorf("%s must be a number, got %T", key, v)
	}
}

// paramInt extracts an int from a params map with a default value.
// Accepts both int and float64 values from YAML.
func paramInt(params map[string]any, key string, def int) (int, error) {
	v, ok := params[key]
	if !ok {
		return def, nil
	}
	switch t := v.(type) {
	case float64:
		return int(t), nil
	case int:
		return t, nil
	default:
		return 0, fmt.Errorf("%s must be a number, got %T", key, v)
	}
}

// paramStringSlice extracts a []string from a params map.
// Returns nil when the key is absent.
func paramStringSlice(params map[string]any, key string) ([]string, error) {
	v, ok := params[key]
	if !ok {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array, got %T", key, v)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s items must be strings, got %T", key, item)
		}
		out = append(out, s)
	}
	return out, nil
}

// buildCompactEffect creates a CompactEffect from YAML params.
func buildCompactEffect(params map[string]any, wctx EffectWiringContext) (agent.Effect, error) {
	threshold, err := paramFloat(params, "threshold", 0.8)
	if err != nil {
		return nil, err
	}
	return effects.NewCompactEffect(effects.CompactConfig{
		ContextWindow: wctx.ContextWindow,
		Threshold:     threshold,
		AskFunc:       wctx.AskFunc,
		NotifyFunc:    wctx.NotifyFunc,
	}), nil
}

// buildTrimToolResultsEffect creates a TrimToolResultsEffect from YAML params.
func buildTrimToolResultsEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	maxLen, err := paramInt(params, "max_result_length", 0)
	if err != nil {
		return nil, err
	}
	preserve, err := paramInt(params, "preserve_recent", 0)
	if err != nil {
		return nil, err
	}
	return effects.NewTrimToolResultsEffect(effects.TrimToolResultsConfig{
		MaxResultLength: maxLen,
		PreserveRecent:  preserve,
	}), nil
}

// buildLoopDetectEffect creates a LoopDetectEffect from YAML params.
func buildLoopDetectEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	threshold, err := paramInt(params, "threshold", 0)
	if err != nil {
		return nil, err
	}
	windowSize, err := paramInt(params, "window_size", 0)
	if err != nil {
		return nil, err
	}
	return effects.NewLoopDetectEffect(effects.LoopDetectConfig{
		Threshold:  threshold,
		WindowSize: windowSize,
	}), nil
}

// buildObservationMaskEffect creates an ObservationMaskEffect from YAML params.
func buildObservationMaskEffect(params map[string]any, wctx EffectWiringContext) (agent.Effect, error) {
	threshold, err := paramFloat(params, "threshold", 0)
	if err != nil {
		return nil, err
	}
	recentWindow, err := paramInt(params, "recent_window", 0)
	if err != nil {
		return nil, err
	}
	return effects.NewObservationMaskEffect(effects.ObservationMaskConfig{
		ContextWindow: wctx.ContextWindow,
		Threshold:     threshold,
		RecentWindow:  recentWindow,
	}), nil
}

// buildReflectionEffect creates a ReflectionEffect from YAML params.
func buildReflectionEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	threshold, err := paramInt(params, "failure_threshold", 0)
	if err != nil {
		return nil, err
	}
	return effects.NewReflectionEffect(effects.ReflectionConfig{
		FailureThreshold: threshold,
	}), nil
}

// buildProgressEffect creates a ProgressEffect from YAML params.
func buildProgressEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	interval, err := paramInt(params, "interval", 0)
	if err != nil {
		return nil, err
	}
	return effects.NewProgressEffect(effects.ProgressConfig{
		Interval: interval,
	}), nil
}

// buildSlidingWindowEffect creates a SlidingWindowEffect from YAML params.
func buildSlidingWindowEffect(params map[string]any, wctx EffectWiringContext) (agent.Effect, error) {
	threshold, err := paramFloat(params, "threshold", 0.7)
	if err != nil {
		return nil, err
	}
	recentZone, err := paramInt(params, "recent_zone", 0)
	if err != nil {
		return nil, err
	}
	mediumZone, err := paramInt(params, "medium_zone", 0)
	if err != nil {
		return nil, err
	}
	trimLength, err := paramInt(params, "trim_length", 0)
	if err != nil {
		return nil, err
	}
	return effects.NewSlidingWindowEffect(effects.SlidingWindowConfig{
		ContextWindow: wctx.ContextWindow,
		NotifyFunc:    wctx.NotifyFunc,
		Threshold:     threshold,
		RecentZone:    recentZone,
		MediumZone:    mediumZone,
		TrimLength:    trimLength,
	}), nil
}

// buildToolScopeEffect creates a ToolScopeEffect from YAML params.
func buildToolScopeEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	exclude, err := paramStringSlice(params, "exclude")
	if err != nil {
		return nil, err
	}
	return effects.NewToolScopeEffect(effects.ToolScopeConfig{
		Exclude: exclude,
	}), nil
}

// buildOffloadEffect creates an OffloadEffect from YAML params.
func buildOffloadEffect(params map[string]any, wctx EffectWiringContext) (agent.Effect, error) {
	threshold, err := paramFloat(params, "threshold", 0)
	if err != nil {
		return nil, err
	}
	minResultLen, err := paramInt(params, "min_result_len", 0)
	if err != nil {
		return nil, err
	}
	recentWindow, err := paramInt(params, "recent_window", 0)
	if err != nil {
		return nil, err
	}
	return effects.NewOffloadEffect(effects.OffloadConfig{
		ContextWindow: wctx.ContextWindow,
		StorageDir:    wctx.StorageDir,
		Threshold:     threshold,
		MinResultLen:  minResultLen,
		RecentWindow:  recentWindow,
	}), nil
}
