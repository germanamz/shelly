// Package modeladapter defines the interface and types for LLM completion adapters.
//
// It contains:
//   - [Completer] interface for LLM completions
//   - [Client] type with HTTP and WebSocket helpers, auth, and custom headers
//   - [ModelConfig] struct for model-specific settings (name, temperature, max tokens)
//   - [RateLimitedCompleter] wrapper with proactive TPM/RPM throttling and 429 retry
//   - [github.com/germanamz/shelly/pkg/modeladapter/usage] — thread-safe token usage tracker
//
// This package contains no provider-specific code — concrete adapters live in
// separate packages that compose a Client and implement Completer.
package modeladapter
