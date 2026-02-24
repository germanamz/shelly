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
