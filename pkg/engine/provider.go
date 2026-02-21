package engine

import (
	"fmt"
	"sync"

	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/providers/anthropic"
	"github.com/germanamz/shelly/pkg/providers/grok"
	"github.com/germanamz/shelly/pkg/providers/openai"
)

// ProviderFactory creates a Completer from a ProviderConfig.
type ProviderFactory func(cfg ProviderConfig) (modeladapter.Completer, error)

var (
	factoryMu   sync.RWMutex
	factories   = map[string]ProviderFactory{}
	defaultsReg sync.Once
)

func ensureDefaults() {
	defaultsReg.Do(func() {
		factories["anthropic"] = newAnthropic
		factories["openai"] = newOpenAI
		factories["grok"] = newGrok
	})
}

// RegisterProvider registers a custom provider factory under the given kind.
// It can be called before New to extend the engine with additional providers.
func RegisterProvider(kind string, factory ProviderFactory) {
	ensureDefaults()

	factoryMu.Lock()
	defer factoryMu.Unlock()

	factories[kind] = factory
}

// getFactory returns the factory for the given kind.
func getFactory(kind string) (ProviderFactory, bool) {
	ensureDefaults()

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

	return anthropic.New(baseURL, cfg.APIKey, cfg.Model), nil
}

func newOpenAI(cfg ProviderConfig) (modeladapter.Completer, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	return openai.New(baseURL, cfg.APIKey, cfg.Model), nil
}

func newGrok(cfg ProviderConfig) (modeladapter.Completer, error) {
	a := grok.New(cfg.APIKey, nil)
	if cfg.BaseURL != "" {
		a.BaseURL = cfg.BaseURL
	}
	if cfg.Model != "" {
		a.Name = cfg.Model
	}

	return a, nil
}

// buildCompleter creates a Completer from a ProviderConfig using the registered
// factory for its Kind.
func buildCompleter(cfg ProviderConfig) (modeladapter.Completer, error) {
	factory, ok := getFactory(cfg.Kind)
	if !ok {
		return nil, fmt.Errorf("engine: unknown provider kind %q", cfg.Kind)
	}

	return factory(cfg)
}
