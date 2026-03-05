package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register("worker", "Does work", func() *Agent {
		return &Agent{name: "worker", configName: "worker"}
	})

	f, ok := r.Get("worker")

	require.True(t, ok)
	assert.Equal(t, "worker", f().name)
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("nonexistent")

	assert.False(t, ok)
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register("charlie", "Third", func() *Agent { return &Agent{} })
	r.Register("alpha", "First", func() *Agent { return &Agent{} })
	r.Register("bravo", "Second", func() *Agent { return &Agent{} })

	entries := r.List()

	require.Len(t, entries, 3)
	assert.Equal(t, "alpha", entries[0].Name)
	assert.Equal(t, "First", entries[0].Description)
	assert.Equal(t, "bravo", entries[1].Name)
	assert.Equal(t, "charlie", entries[2].Name)
}

func TestRegistryListEmpty(t *testing.T) {
	r := NewRegistry()

	entries := r.List()

	assert.Empty(t, entries)
}

func TestRegistrySpawn(t *testing.T) {
	r := NewRegistry()
	r.Register("worker", "Does work", func() *Agent {
		return &Agent{name: "worker", configName: "worker"}
	})

	agent, ok := r.Spawn("worker", 3)

	require.True(t, ok)
	assert.Equal(t, "worker", agent.name)
	assert.Equal(t, "worker", agent.configName)
	assert.Equal(t, 3, agent.depth)
}

func TestRegistrySpawnMissing(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Spawn("nonexistent", 0)

	assert.False(t, ok)
}

func TestRegistrySpawnFreshInstances(t *testing.T) {
	r := NewRegistry()
	r.Register("worker", "Does work", func() *Agent {
		return &Agent{name: "worker", configName: "worker"}
	})

	a1, _ := r.Spawn("worker", 0)
	a2, _ := r.Spawn("worker", 0)

	assert.NotSame(t, a1, a2)
}

func TestRegistryReplace(t *testing.T) {
	r := NewRegistry()
	r.Register("worker", "Version 1", func() *Agent {
		return &Agent{name: "worker-v1", configName: "worker-v1"}
	})
	r.Register("worker", "Version 2", func() *Agent {
		return &Agent{name: "worker-v2", configName: "worker-v2"}
	})

	entries := r.List()
	require.Len(t, entries, 1)
	assert.Equal(t, "Version 2", entries[0].Description)

	agent, ok := r.Spawn("worker", 0)
	require.True(t, ok)
	assert.Equal(t, "worker-v2", agent.name)
}

func TestRegistryNextID(t *testing.T) {
	r := NewRegistry()

	// First call for "coder" returns 1.
	assert.Equal(t, 1, r.NextID("coder"))
	// Second call increments.
	assert.Equal(t, 2, r.NextID("coder"))
	// Different config name starts at 1.
	assert.Equal(t, 1, r.NextID("reviewer"))
	// Original keeps incrementing.
	assert.Equal(t, 3, r.NextID("coder"))
}

func TestRegistryRegisterEntry(t *testing.T) {
	r := NewRegistry()
	entry := Entry{
		Name:           "coder",
		Description:    "Writes code",
		Skills:         []string{"coding", "testing"},
		InputSchema:    json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"}}}`),
		OutputSchema:   json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}}}`),
		EstimatedCost:  "medium",
		MaxConcurrency: 3,
	}
	r.RegisterEntry(entry, func() *Agent {
		return &Agent{name: "coder", configName: "coder"}
	})

	entries := r.List()
	require.Len(t, entries, 1)
	assert.Equal(t, "coder", entries[0].Name)
	assert.Equal(t, "Writes code", entries[0].Description)
	assert.Equal(t, []string{"coding", "testing"}, entries[0].Skills)
	assert.JSONEq(t, `{"type":"object","properties":{"task":{"type":"string"}}}`, string(entries[0].InputSchema))
	assert.JSONEq(t, `{"type":"object","properties":{"summary":{"type":"string"}}}`, string(entries[0].OutputSchema))
	assert.Equal(t, "medium", entries[0].EstimatedCost)
	assert.Equal(t, 3, entries[0].MaxConcurrency)

	f, ok := r.Get("coder")
	require.True(t, ok)
	assert.Equal(t, "coder", f().name)
}

func TestRegistryRegisterEntryMinimal(t *testing.T) {
	r := NewRegistry()
	entry := Entry{Name: "worker", Description: "Does work"}
	r.RegisterEntry(entry, func() *Agent {
		return &Agent{name: "worker", configName: "worker"}
	})

	entries := r.List()
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].Skills)
	assert.Nil(t, entries[0].InputSchema)
	assert.Nil(t, entries[0].OutputSchema)
	assert.Empty(t, entries[0].EstimatedCost)
	assert.Zero(t, entries[0].MaxConcurrency)
}

func TestRegistrySpawnSetsConfigName(t *testing.T) {
	r := NewRegistry()
	// Factory returns agent with a different name than the registry key.
	r.Register("coder", "Writes code", func() *Agent {
		return &Agent{name: "coder", configName: "coder"}
	})

	agent, ok := r.Spawn("coder", 1)
	require.True(t, ok)
	// Spawn should set configName to the registry key.
	assert.Equal(t, "coder", agent.configName)
}
