// Package providers defines the interface and types for LLM completion providers.
//
// It is organized into sub-packages:
//   - [github.com/germanamz/shelly/pkg/providers/model] — embeddable configuration shared by all providers (name, temperature, max tokens)
//   - [github.com/germanamz/shelly/pkg/providers/provider] — interface that concrete implementations (OpenAI, Anthropic, etc.) must satisfy
//
// This package contains no provider-specific code — concrete adapters live in
// separate packages that import providers.
package providers
