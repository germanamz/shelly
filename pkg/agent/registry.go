package agent

import (
	"sort"
	"sync"
)

// Factory creates a fresh Agent instance for delegation. Each call should
// return a new agent with a clean chat to avoid state leakage between
// delegations.
type Factory func() *Agent

// Entry describes a registered agent in the directory.
type Entry struct {
	Name        string
	Description string
}

// Registry is a thread-safe directory of agent factories. It allows agents
// to discover and spawn other agents at runtime.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	entries   map[string]Entry
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		entries:   make(map[string]Entry),
	}
}

// Register adds an agent factory to the registry. If an agent with the same
// name already exists, it is replaced.
func (r *Registry) Register(name, description string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[name] = factory
	r.entries[name] = Entry{Name: name, Description: description}
}

// Get returns the factory for the named agent and true, or nil and false if
// not found.
func (r *Registry) Get(name string) (Factory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	f, ok := r.factories[name]
	return f, ok
}

// List returns all registry entries sorted by name.
func (r *Registry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, e)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}

// Spawn creates a fresh agent instance from the named factory and sets its
// delegation depth. Returns nil and false if the name is not registered.
func (r *Registry) Spawn(name string, depth int) (*Agent, bool) {
	r.mu.RLock()
	f, ok := r.factories[name]
	r.mu.RUnlock()

	if !ok {
		return nil, false
	}

	agent := f()
	agent.depth = depth

	return agent, true
}
