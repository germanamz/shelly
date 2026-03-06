package usage

import (
	"embed"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed pricing.yaml
var defaultPricingYAML embed.FS

// ModelPricing holds per-token prices in USD per 1M tokens for a model.
type ModelPricing struct {
	InputPer1M         float64 // USD per 1M input tokens.
	OutputPer1M        float64 // USD per 1M output tokens.
	CacheReadPer1M     float64 // USD per 1M cache-read input tokens.
	CacheCreationPer1M float64 // USD per 1M cache-creation input tokens.
}

// CalculateCost returns the estimated USD cost for the given token counts.
func CalculateCost(tc TokenCount, p ModelPricing) float64 {
	input := float64(tc.InputTokens) * p.InputPer1M / 1_000_000
	output := float64(tc.OutputTokens) * p.OutputPer1M / 1_000_000
	cacheRead := float64(tc.CacheReadInputTokens) * p.CacheReadPer1M / 1_000_000
	cacheCreate := float64(tc.CacheCreationInputTokens) * p.CacheCreationPer1M / 1_000_000
	return input + output + cacheRead + cacheCreate
}

// pricingYAMLEntry is the YAML representation of a pricing entry.
type pricingYAMLEntry struct {
	Provider      string  `yaml:"provider"`
	Prefix        string  `yaml:"prefix"`
	InputPer1M    float64 `yaml:"input"`
	OutputPer1M   float64 `yaml:"output"`
	CacheReadPM   float64 `yaml:"cache_read"`
	CacheCreatePM float64 `yaml:"cache_creation"`
}

// pricingEntry maps a model prefix to its pricing.
type pricingEntry struct {
	provider string
	prefix   string
	pricing  ModelPricing
}

var (
	pricingOnce  sync.Once
	pricingTable []pricingEntry
	overridePath string
)

// SetOverridePath sets the path to an optional user pricing.yaml that will be
// merged on top of the embedded defaults. Must be called before the first
// LookupPricing call. Typically set to .shelly/local/pricing.yaml.
func SetOverridePath(path string) {
	overridePath = path
}

// loadPricing loads the embedded default table, then merges any user override.
func loadPricing() {
	pricingTable = loadEmbedded()
	if overridePath != "" {
		if overrides := loadFile(overridePath); len(overrides) > 0 {
			pricingTable = mergeEntries(pricingTable, overrides)
		}
	}
}

func loadEmbedded() []pricingEntry {
	data, err := defaultPricingYAML.ReadFile("pricing.yaml")
	if err != nil {
		return nil
	}
	return parseYAML(data)
}

func loadFile(path string) []pricingEntry {
	data, err := os.ReadFile(path) //nolint:gosec // user-configured path
	if err != nil {
		return nil
	}
	return parseYAML(data)
}

func parseYAML(data []byte) []pricingEntry {
	var entries []pricingYAMLEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil
	}
	result := make([]pricingEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, pricingEntry{
			provider: strings.ToLower(e.Provider),
			prefix:   strings.ToLower(e.Prefix),
			pricing: ModelPricing{
				InputPer1M:         e.InputPer1M,
				OutputPer1M:        e.OutputPer1M,
				CacheReadPer1M:     e.CacheReadPM,
				CacheCreationPer1M: e.CacheCreatePM,
			},
		})
	}
	return result
}

// mergeEntries returns a new slice where override entries replace defaults
// with the same provider+prefix, and new overrides are appended.
func mergeEntries(defaults, overrides []pricingEntry) []pricingEntry {
	type key struct{ provider, prefix string }
	overrideMap := make(map[key]pricingEntry, len(overrides))
	for _, o := range overrides {
		overrideMap[key{o.provider, o.prefix}] = o
	}

	result := make([]pricingEntry, 0, len(defaults)+len(overrides))
	seen := make(map[key]bool, len(defaults))
	for _, d := range defaults {
		k := key{d.provider, d.prefix}
		if o, ok := overrideMap[k]; ok {
			result = append(result, o)
		} else {
			result = append(result, d)
		}
		seen[k] = true
	}
	// Append new entries from overrides that aren't in defaults.
	for _, o := range overrides {
		k := key{o.provider, o.prefix}
		if !seen[k] {
			result = append(result, o)
		}
	}
	return result
}

// LookupPricing returns the pricing for the given provider kind and model ID.
// It matches the longest prefix in the pricing table. The bool is false if no
// match is found. The pricing table is loaded lazily on first call.
func LookupPricing(providerKind, modelID string) (ModelPricing, bool) {
	pricingOnce.Do(loadPricing)

	providerKind = strings.ToLower(providerKind)
	modelID = strings.ToLower(modelID)

	var best pricingEntry
	bestLen := -1
	for _, e := range pricingTable {
		if e.provider != providerKind {
			continue
		}
		if strings.HasPrefix(modelID, e.prefix) && len(e.prefix) > bestLen {
			best = e
			bestLen = len(e.prefix)
		}
	}
	if bestLen < 0 {
		return ModelPricing{}, false
	}
	return best.pricing, true
}

// ResetForTest resets the pricing singleton so tests can call SetOverridePath
// and LookupPricing again with fresh state. Not for production use.
func ResetForTest() {
	pricingOnce = sync.Once{}
	pricingTable = nil
	overridePath = ""
}
