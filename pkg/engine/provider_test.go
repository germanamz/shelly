package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveContextWindow_DefaultsForKnownKinds(t *testing.T) {
	tests := []struct {
		kind     string
		expected int
	}{
		{"anthropic", 200000},
		{"openai", 128000},
		{"grok", 131072},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			cfg := ProviderConfig{Kind: tt.kind}
			assert.Equal(t, tt.expected, resolveContextWindow(cfg, nil))
		})
	}
}

func TestResolveContextWindow_ExplicitValue(t *testing.T) {
	cfg := ProviderConfig{Kind: "anthropic", ContextWindow: intPtr(100000)}
	assert.Equal(t, 100000, resolveContextWindow(cfg, nil))
}

func TestResolveContextWindow_ExplicitZeroDisablesCompaction(t *testing.T) {
	cfg := ProviderConfig{Kind: "anthropic", ContextWindow: intPtr(0)}
	assert.Equal(t, 0, resolveContextWindow(cfg, nil))
}

func TestResolveContextWindow_UnknownKindNilReturnsZero(t *testing.T) {
	cfg := ProviderConfig{Kind: "custom-llm"}
	assert.Equal(t, 0, resolveContextWindow(cfg, nil))
}

func TestResolveContextWindow_ConfigDefaultOverridesBuiltin(t *testing.T) {
	cfg := ProviderConfig{Kind: "anthropic"}
	overrides := map[string]int{"anthropic": 150000}
	assert.Equal(t, 150000, resolveContextWindow(cfg, overrides))
}

func TestResolveContextWindow_ConfigDefaultForCustomKind(t *testing.T) {
	cfg := ProviderConfig{Kind: "custom-llm"}
	overrides := map[string]int{"custom-llm": 64000}
	assert.Equal(t, 64000, resolveContextWindow(cfg, overrides))
}

func TestResolveContextWindow_ExplicitValueOverridesConfigDefault(t *testing.T) {
	cfg := ProviderConfig{Kind: "anthropic", ContextWindow: intPtr(100000)}
	overrides := map[string]int{"anthropic": 150000}
	assert.Equal(t, 100000, resolveContextWindow(cfg, overrides))
}
