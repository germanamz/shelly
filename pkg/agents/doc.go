// Package agents provides agent orchestration for Shelly. It defines the
// Agent interface for polymorphic usage and the Base struct that concrete
// agent types embed to inherit shared functionality (Complete, CallTools, Tools).
//
// The react sub-package implements the ReAct (Reason + Act) loop as a concrete
// Agent implementation that embeds Base.
package agents
