// Package engine is the composition root that assembles all Shelly framework
// components from configuration and exposes them through a frontend-agnostic
// API. Frontends (CLI, web, desktop) interact with Engine and Session types,
// observe activity through an EventBus, and never import lower-level packages
// directly.
package engine
