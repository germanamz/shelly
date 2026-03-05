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
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Skills         []string `json:"skills,omitempty"`
	EstimatedCost  string   `json:"estimated_cost,omitempty"`
	MaxConcurrency int      `json:"max_concurrency,omitempty"`
}

// Registry is a thread-safe directory of agent factories. It allows agents
// to discover and spawn other agents at runtime.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	entries   map[string]Entry
	counters  map[string]int
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		entries:   make(map[string]Entry),
		counters:  make(map[string]int),
	}
}

// NextID increments and returns a monotonic counter for the given config name.
// It is used to generate unique instance names for spawned agents.
func (r *Registry) NextID(configName string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counters[configName]++
	return r.counters[configName]
}

// Register adds an agent factory to the registry. If an agent with the same
// name already exists, it is replaced.
func (r *Registry) Register(name, description string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[name] = factory
	r.entries[name] = Entry{Name: name, Description: description}
}

// RegisterEntry adds an agent factory to the registry using a full Entry.
// If an agent with the same name already exists, it is replaced.
func (r *Registry) RegisterEntry(entry Entry, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[entry.Name] = factory
	r.entries[entry.Name] = entry
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
	agent.configName = name

	return agent, true
}
