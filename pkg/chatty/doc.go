// Package chatty provides a provider-agnostic data model for LLM chat interactions.
//
// It is organized into sub-packages:
//   - [github.com/germanamz/shelly/pkg/chatty/role] — conversation roles (system, user, assistant, tool)
//   - [github.com/germanamz/shelly/pkg/chatty/content] — multi-modal content parts (text, image, tool call/result)
//   - [github.com/germanamz/shelly/pkg/chatty/message] — messages composed of a role, sender, and content parts
//   - [github.com/germanamz/shelly/pkg/chatty/chat] — mutable conversation container
//
// No provider or API code is included — chatty is a foundation layer
// that adapters can build on.
package chatty
