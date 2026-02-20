// Package chats provides a provider-agnostic data model for LLM chat interactions.
//
// It is organized into sub-packages:
//   - [github.com/germanamz/shelly/pkg/chats/role] — conversation roles (system, user, assistant, tool)
//   - [github.com/germanamz/shelly/pkg/chats/content] — multi-modal content parts (text, image, tool call/result)
//   - [github.com/germanamz/shelly/pkg/chats/message] — messages composed of a role, sender, and content parts
//   - [github.com/germanamz/shelly/pkg/chats/chat] — mutable conversation container
//
// No provider or API code is included — chats is a foundation layer
// that adapters can build on.
package chats
