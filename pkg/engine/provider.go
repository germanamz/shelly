package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/providers/anthropic"
	"github.com/germanamz/shelly/pkg/providers/gemini"
	"github.com/germanamz/shelly/pkg/providers/grok"
	"github.com/germanamz/shelly/pkg/providers/openai"
)

// BuiltinContextWindows maps well-known provider kinds to their default context
// window sizes in tokens. Used when ContextWindow is nil (omitted in YAML).
var BuiltinContextWindows = map[string]int{
	"anthropic": 200000,
	"openai":    128000,
	"grok":      131072,
	"gemini":    1048576,
}

// resolveContextWindow returns the effective context window for a provider.
// If ContextWindow is explicitly set (including 0), that value is used.
// Otherwise, configDefaults is checked first (user overrides), then the
// built-in defaultContextWindows. Returns 0 for unknown kinds with no override.
func resolveContextWindow(cfg ProviderConfig, configDefaults map[string]int) int {
	if cfg.ContextWindow != nil {
		return *cfg.ContextWindow
	}
	if v, ok := configDefaults[cfg.Kind]; ok {
		return v
	}
	return BuiltinContextWindows[cfg.Kind]
}

// ProviderFactory creates a Completer from a ProviderConfig.
type ProviderFactory func(cfg ProviderConfig) (modeladapter.Completer, error)

var (
	factoryMu sync.RWMutex
	factories = map[string]ProviderFactory{}
)

func init() {
	factories["anthropic"] = newAnthropic
	factories["openai"] = newOpenAI
	factories["grok"] = newGrok
	factories["gemini"] = newGemini
}

// RegisterProvider registers a custom provider factory under the given kind.
// It can be called before New to extend the engine with additional providers.
func RegisterProvider(kind string, factory ProviderFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()

	factories[kind] = factory
}

// getFactory returns the factory for the given kind.
func getFactory(kind string) (ProviderFactory, bool) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	f, ok := factories[kind]
	return f, ok
}

func newAnthropic(cfg ProviderConfig) (modeladapter.Completer, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	a := anthropic.New(baseURL, cfg.APIKey, cfg.Model)
	if cfg.MaxTokens != nil {
		a.Config.MaxTokens = *cfg.MaxTokens
	}
	return a, nil
}

func newOpenAI(cfg ProviderConfig) (modeladapter.Completer, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	a := openai.New(baseURL, cfg.APIKey, cfg.Model)
	if cfg.MaxTokens != nil {
		a.Config.MaxTokens = *cfg.MaxTokens
	}
	return a, nil
}

func newGrok(cfg ProviderConfig) (modeladapter.Completer, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = grok.DefaultBaseURL
	}

	a := grok.New(baseURL, cfg.APIKey, cfg.Model, nil)
	if cfg.MaxTokens != nil {
		a.Config.MaxTokens = *cfg.MaxTokens
	}

	return a, nil
}

func newGemini(cfg ProviderConfig) (modeladapter.Completer, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	a := gemini.New(baseURL, cfg.APIKey, cfg.Model)
	if cfg.MaxTokens != nil {
		a.Config.MaxTokens = *cfg.MaxTokens
	}
	return a, nil
}

// BatchSubmitterFactory creates a batch.Submitter from a ProviderConfig and
// the already-built completer (used for request building).
type BatchSubmitterFactory func(cfg ProviderConfig, completer modeladapter.Completer) (batch.Submitter, error)

var (
	batchFactoryMu sync.RWMutex
	batchFactories = map[string]BatchSubmitterFactory{
		"anthropic": newAnthropicBatchSubmitter,
		"openai":    newOpenAIBatchSubmitter,
		"grok":      newGrokBatchSubmitter,
		// Gemini is excluded: its REST API does not offer a batch endpoint.
		// Gemini batch is only available via Vertex AI Batch Prediction.
	}
)

// RegisterBatchSubmitter registers a custom batch submitter factory under the
// given provider kind. It can be called before New to extend the engine with
// batch support for additional providers.
func RegisterBatchSubmitter(kind string, factory BatchSubmitterFactory) {
	batchFactoryMu.Lock()
	defer batchFactoryMu.Unlock()

	batchFactories[kind] = factory
}

func newAnthropicBatchSubmitter(_ ProviderConfig, completer modeladapter.Completer) (batch.Submitter, error) {
	adapter, ok := completer.(*anthropic.Adapter)
	if !ok {
		return nil, fmt.Errorf("engine: anthropic batch: completer is not *anthropic.Adapter")
	}
	return anthropic.NewBatchSubmitter(adapter), nil
}

func newOpenAIBatchSubmitter(_ ProviderConfig, completer modeladapter.Completer) (batch.Submitter, error) {
	adapter, ok := completer.(*openai.Adapter)
	if !ok {
		return nil, fmt.Errorf("engine: openai batch: completer is not *openai.Adapter")
	}
	return openai.NewBatchSubmitter(adapter), nil
}

func newGrokBatchSubmitter(_ ProviderConfig, completer modeladapter.Completer) (batch.Submitter, error) {
	adapter, ok := completer.(*grok.GrokAdapter)
	if !ok {
		return nil, fmt.Errorf("engine: grok batch: completer is not *grok.GrokAdapter")
	}
	return grok.NewBatchSubmitter(adapter), nil
}

func buildBatchSubmitter(cfg ProviderConfig, completer modeladapter.Completer) (batch.Submitter, error) {
	batchFactoryMu.RLock()
	factory, ok := batchFactories[cfg.Kind]
	batchFactoryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("engine: batch not supported for provider kind %q", cfg.Kind)
	}
	return factory(cfg, completer)
}

func parseBatchOpts(cfg BatchConfig) (batch.CollectorOpts, error) {
	var opts batch.CollectorOpts
	if cfg.CollectWindow != "" {
		d, err := time.ParseDuration(cfg.CollectWindow)
		if err != nil {
			return opts, fmt.Errorf("batch collect_window: %w", err)
		}
		opts.CollectWindow = d
	}
	if cfg.PollInterval != "" {
		d, err := time.ParseDuration(cfg.PollInterval)
		if err != nil {
			return opts, fmt.Errorf("batch poll_interval: %w", err)
		}
		opts.PollInterval = d
	}
	if cfg.Timeout != "" {
		d, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return opts, fmt.Errorf("batch timeout: %w", err)
		}
		opts.Timeout = d
	}
	if cfg.MaxBatchSize > 0 {
		opts.MaxBatchSize = cfg.MaxBatchSize
	}
	return opts, nil
}

// buildCompleter creates a Completer from a ProviderConfig using the registered
// factory for its Kind. If rate limiting is configured, the completer is wrapped
// with a RateLimitedCompleter. If batch mode is enabled, the completer is
// wrapped with a batch Collector before rate limiting.
func buildCompleter(cfg ProviderConfig) (modeladapter.Completer, error) {
	factory, ok := getFactory(cfg.Kind)
	if !ok {
		return nil, fmt.Errorf("engine: unknown provider kind %q", cfg.Kind)
	}

	c, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	// Wrap with batch collector if enabled. Rate limiting (below) wraps the
	// Collector, so it throttles entry into Complete() but does not gate the
	// background SubmitBatch/PollBatch HTTP calls made by the Collector.
	if cfg.Batch.Enabled {
		submitter, batchErr := buildBatchSubmitter(cfg, c)
		if batchErr != nil {
			return nil, batchErr
		}
		opts, optsErr := parseBatchOpts(cfg.Batch)
		if optsErr != nil {
			return nil, fmt.Errorf("engine: provider %q: %w", cfg.Name, optsErr)
		}
		c = batch.NewCollector(c, submitter, opts)
	}

	rl := cfg.RateLimit
	if rl.InputTPM > 0 || rl.OutputTPM > 0 || rl.RPM > 0 || rl.MaxRetries > 0 || rl.BaseDelay != "" {
		var baseDelay time.Duration
		if rl.BaseDelay != "" {
			var parseErr error
			baseDelay, parseErr = time.ParseDuration(rl.BaseDelay)
			if parseErr != nil {
				return nil, fmt.Errorf("engine: provider %q: invalid base_delay %q: %w", cfg.Name, rl.BaseDelay, parseErr)
			}
		}

		c = modeladapter.NewRateLimitedCompleter(c, modeladapter.RateLimitOpts{
			InputTPM:   rl.InputTPM,
			OutputTPM:  rl.OutputTPM,
			RPM:        rl.RPM,
			MaxRetries: rl.MaxRetries,
			BaseDelay:  baseDelay,
		})
	}

	return c, nil
}
