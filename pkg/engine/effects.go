package engine

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/agent/effects"
)

// EffectWiringContext provides engine-level resources needed by effect
// factories to construct concrete Effect instances.
type EffectWiringContext struct {
	ContextWindow int
	AgentName     string

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

	return effs, nil
}

// buildCompactEffect creates a CompactEffect from YAML params.
func buildCompactEffect(params map[string]any, wctx EffectWiringContext) (agent.Effect, error) {
	threshold := 0.8 // default
	if v, ok := params["threshold"]; ok {
		switch t := v.(type) {
		case float64:
			threshold = t
		case int:
			threshold = float64(t)
		default:
			return nil, fmt.Errorf("threshold must be a number, got %T", v)
		}
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
	cfg := effects.TrimToolResultsConfig{}

	if v, ok := params["max_result_length"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.MaxResultLength = int(t)
		case int:
			cfg.MaxResultLength = t
		default:
			return nil, fmt.Errorf("max_result_length must be a number, got %T", v)
		}
	}

	if v, ok := params["preserve_recent"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.PreserveRecent = int(t)
		case int:
			cfg.PreserveRecent = t
		default:
			return nil, fmt.Errorf("preserve_recent must be a number, got %T", v)
		}
	}

	return effects.NewTrimToolResultsEffect(cfg), nil
}

// buildLoopDetectEffect creates a LoopDetectEffect from YAML params.
func buildLoopDetectEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	cfg := effects.LoopDetectConfig{}

	if v, ok := params["threshold"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.Threshold = int(t)
		case int:
			cfg.Threshold = t
		default:
			return nil, fmt.Errorf("threshold must be a number, got %T", v)
		}
	}

	if v, ok := params["window_size"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.WindowSize = int(t)
		case int:
			cfg.WindowSize = t
		default:
			return nil, fmt.Errorf("window_size must be a number, got %T", v)
		}
	}

	return effects.NewLoopDetectEffect(cfg), nil
}

// buildObservationMaskEffect creates an ObservationMaskEffect from YAML params.
func buildObservationMaskEffect(params map[string]any, wctx EffectWiringContext) (agent.Effect, error) {
	cfg := effects.ObservationMaskConfig{
		ContextWindow: wctx.ContextWindow,
	}

	if v, ok := params["threshold"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.Threshold = t
		case int:
			cfg.Threshold = float64(t)
		default:
			return nil, fmt.Errorf("threshold must be a number, got %T", v)
		}
	}

	if v, ok := params["recent_window"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.RecentWindow = int(t)
		case int:
			cfg.RecentWindow = t
		default:
			return nil, fmt.Errorf("recent_window must be a number, got %T", v)
		}
	}

	return effects.NewObservationMaskEffect(cfg), nil
}

// buildReflectionEffect creates a ReflectionEffect from YAML params.
func buildReflectionEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	cfg := effects.ReflectionConfig{}

	if v, ok := params["failure_threshold"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.FailureThreshold = int(t)
		case int:
			cfg.FailureThreshold = t
		default:
			return nil, fmt.Errorf("failure_threshold must be a number, got %T", v)
		}
	}

	return effects.NewReflectionEffect(cfg), nil
}

// buildProgressEffect creates a ProgressEffect from YAML params.
func buildProgressEffect(params map[string]any, _ EffectWiringContext) (agent.Effect, error) {
	cfg := effects.ProgressConfig{}

	if v, ok := params["interval"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.Interval = int(t)
		case int:
			cfg.Interval = t
		default:
			return nil, fmt.Errorf("interval must be a number, got %T", v)
		}
	}

	return effects.NewProgressEffect(cfg), nil
}

// buildSlidingWindowEffect creates a SlidingWindowEffect from YAML params.
func buildSlidingWindowEffect(params map[string]any, wctx EffectWiringContext) (agent.Effect, error) {
	cfg := effects.SlidingWindowConfig{
		ContextWindow: wctx.ContextWindow,
		NotifyFunc:    wctx.NotifyFunc,
	}

	threshold := 0.7 // default (lower than compact to trigger earlier)
	if v, ok := params["threshold"]; ok {
		switch t := v.(type) {
		case float64:
			threshold = t
		case int:
			threshold = float64(t)
		default:
			return nil, fmt.Errorf("threshold must be a number, got %T", v)
		}
	}
	cfg.Threshold = threshold

	if v, ok := params["recent_zone"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.RecentZone = int(t)
		case int:
			cfg.RecentZone = t
		default:
			return nil, fmt.Errorf("recent_zone must be a number, got %T", v)
		}
	}

	if v, ok := params["medium_zone"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.MediumZone = int(t)
		case int:
			cfg.MediumZone = t
		default:
			return nil, fmt.Errorf("medium_zone must be a number, got %T", v)
		}
	}

	if v, ok := params["trim_length"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.TrimLength = int(t)
		case int:
			cfg.TrimLength = t
		default:
			return nil, fmt.Errorf("trim_length must be a number, got %T", v)
		}
	}

	return effects.NewSlidingWindowEffect(cfg), nil
}
