// Package modeladapter defines the interface and types for LLM completion adapters.
//
// It contains:
//   - [Completer] interface and embeddable [ModelAdapter] base struct with HTTP and WebSocket helpers, auth, and custom headers
//   - [github.com/germanamz/shelly/pkg/modeladapter/usage] — thread-safe token usage tracker
//
// Model configuration (name, temperature, max tokens) is inlined directly on
// the ModelAdapter struct. This package contains no provider-specific code — concrete
// adapters live in separate packages that import modeladapter.
package modeladapter
