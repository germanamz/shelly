package usage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupPricing(t *testing.T) {
	ResetForTest()

	tests := []struct {
		provider string
		model    string
		wantOK   bool
		wantIn   float64 // expected InputPer1M (0 when !wantOK)
	}{
		// Anthropic
		{"anthropic", "claude-opus-4-6", true, 5.0},
		{"anthropic", "claude-opus-4-5-20251101", true, 5.0},
		{"anthropic", "claude-opus-4-1-20250805", true, 15.0},
		{"anthropic", "claude-opus-4-20250514", true, 15.0},
		{"anthropic", "claude-sonnet-4-6", true, 3.0},
		{"anthropic", "claude-sonnet-4-5-20250929", true, 3.0},
		{"anthropic", "claude-sonnet-4-20250514", true, 3.0},
		{"anthropic", "claude-haiku-4-5-20251001", true, 1.0},
		{"anthropic", "claude-haiku-3-5-20241022", true, 0.80},
		{"anthropic", "claude-3-haiku-20240307", true, 0.25},
		// OpenAI
		{"openai", "gpt-5.4-latest", true, 2.50},
		{"openai", "gpt-5.2", true, 1.75},
		{"openai", "gpt-5-mini", true, 0.25},
		{"openai", "gpt-5-nano", true, 0.05},
		{"openai", "gpt-5", true, 1.25},
		{"openai", "gpt-4.1-mini-2025-04-14", true, 0.40},
		{"openai", "gpt-4.1", true, 2.0},
		{"openai", "gpt-4o-mini", true, 0.15},
		{"openai", "o3", true, 2.0},
		{"openai", "o4-mini", true, 1.10},
		// Gemini
		{"gemini", "gemini-3.1-pro-preview", true, 2.0},
		{"gemini", "gemini-3-flash-preview", true, 0.50},
		{"gemini", "gemini-2.5-pro-preview", true, 1.25},
		{"gemini", "gemini-2.5-flash-lite", true, 0.10},
		{"gemini", "gemini-2.5-flash", true, 0.30},
		// Grok
		{"grok", "grok-4-1-fast-reasoning", true, 0.20},
		{"grok", "grok-4-fast-reasoning", true, 0.20},
		{"grok", "grok-4-0709", true, 3.0},
		{"grok", "grok-code-fast-1", true, 0.20},
		{"grok", "grok-3-mini-fast", true, 0.30},
		{"grok", "grok-3", true, 3.0},
		// Unknown
		{"anthropic", "unknown-model", false, 0},
		{"unknown", "claude-sonnet-4", false, 0},
		{"", "", false, 0},
	}
	for _, tt := range tests {
		p, ok := LookupPricing(tt.provider, tt.model)
		assert.Equal(t, tt.wantOK, ok, "%s/%s", tt.provider, tt.model)
		if tt.wantOK {
			assert.InDelta(t, tt.wantIn, p.InputPer1M, 0.001, "%s/%s InputPer1M", tt.provider, tt.model)
		}
	}
}

func TestLookupPricingCaseInsensitive(t *testing.T) {
	ResetForTest()

	p, ok := LookupPricing("Anthropic", "Claude-Opus-4-6")
	assert.True(t, ok)
	assert.InDelta(t, 5.0, p.InputPer1M, 0.001)
}

func TestLookupPricingLongestPrefixWins(t *testing.T) {
	ResetForTest()

	// "gpt-4.1-mini" should match the mini entry, not the "gpt-4.1" entry.
	p, ok := LookupPricing("openai", "gpt-4.1-mini-2025-04-14")
	assert.True(t, ok)
	assert.InDelta(t, 0.40, p.InputPer1M, 0.001)
}

func TestCalculateCost(t *testing.T) {
	pricing := ModelPricing{
		InputPer1M:         3.0,
		OutputPer1M:        15.0,
		CacheReadPer1M:     0.30,
		CacheCreationPer1M: 3.75,
	}

	tc := TokenCount{
		InputTokens:              1_000_000,
		OutputTokens:             500_000,
		CacheReadInputTokens:     200_000,
		CacheCreationInputTokens: 100_000,
	}

	cost := CalculateCost(tc, pricing)
	// 3.0 + 7.5 + 0.06 + 0.375 = 10.935
	assert.InDelta(t, 10.935, cost, 0.001)
}

func TestCalculateCostZero(t *testing.T) {
	cost := CalculateCost(TokenCount{}, ModelPricing{})
	assert.InDelta(t, 0.0, cost, 0.0001)
}

func TestOverrideReplacesDefault(t *testing.T) {
	ResetForTest()

	dir := t.TempDir()
	overrideFile := filepath.Join(dir, "pricing.yaml")
	// Override claude-opus-4-6 with custom pricing.
	err := os.WriteFile(overrideFile, []byte(`
- provider: anthropic
  prefix: claude-opus-4-6
  input: 99.0
  output: 199.0
  cache_read: 9.0
  cache_creation: 19.0
`), 0o600)
	require.NoError(t, err)

	SetOverridePath(overrideFile)
	p, ok := LookupPricing("anthropic", "claude-opus-4-6")
	assert.True(t, ok)
	assert.InDelta(t, 99.0, p.InputPer1M, 0.001)
	assert.InDelta(t, 199.0, p.OutputPer1M, 0.001)

	// Non-overridden entry still works from embedded defaults.
	p2, ok := LookupPricing("openai", "gpt-5.4")
	assert.True(t, ok)
	assert.InDelta(t, 2.50, p2.InputPer1M, 0.001)
}

func TestOverrideAddsNewEntry(t *testing.T) {
	ResetForTest()

	dir := t.TempDir()
	overrideFile := filepath.Join(dir, "pricing.yaml")
	err := os.WriteFile(overrideFile, []byte(`
- provider: custom
  prefix: my-model
  input: 1.0
  output: 2.0
  cache_read: 0.1
  cache_creation: 0.2
`), 0o600)
	require.NoError(t, err)

	SetOverridePath(overrideFile)
	p, ok := LookupPricing("custom", "my-model-v1")
	assert.True(t, ok)
	assert.InDelta(t, 1.0, p.InputPer1M, 0.001)
}

func TestOverrideMissingFileUsesDefaults(t *testing.T) {
	ResetForTest()
	SetOverridePath("/nonexistent/pricing.yaml")

	p, ok := LookupPricing("anthropic", "claude-opus-4-6")
	assert.True(t, ok)
	assert.InDelta(t, 5.0, p.InputPer1M, 0.001)
}
