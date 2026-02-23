package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestFindTemplate(t *testing.T) {
	t.Run("finds simple-assistant", func(t *testing.T) {
		tmpl := findTemplate("simple-assistant")
		assert.NotNil(t, tmpl)
		assert.Equal(t, "simple-assistant", tmpl.Name)
	})

	t.Run("finds dev-team", func(t *testing.T) {
		tmpl := findTemplate("dev-team")
		assert.NotNil(t, tmpl)
		assert.Equal(t, "dev-team", tmpl.Name)
	})

	t.Run("returns nil for unknown", func(t *testing.T) {
		assert.Nil(t, findTemplate("nonexistent"))
	})
}

func TestApplyTemplate_SimpleAssistant(t *testing.T) {
	tmpl := findTemplate("simple-assistant")
	assert.NotNil(t, tmpl)

	providers := []wizardProvider{
		{Kind: "anthropic", Name: "anthropic", APIKey: "${ANTHROPIC_API_KEY}", Model: "claude-sonnet-4-20250514"}, //nolint:gosec // env var reference, not a secret
	}
	slotMapping := map[string]string{"primary": "anthropic"}

	cfg := applyTemplate(tmpl, providers, slotMapping)

	assert.Len(t, cfg.Agents, 1)
	assert.Equal(t, "assistant", cfg.Agents[0].Name)
	assert.Equal(t, "anthropic", cfg.Agents[0].Provider)
	assert.Equal(t, "assistant", cfg.EntryAgent)
	assert.Contains(t, cfg.Agents[0].Toolboxes, "filesystem")
	assert.Contains(t, cfg.Agents[0].Toolboxes, "exec")
	assert.Contains(t, cfg.Agents[0].Toolboxes, "state")
}

//nolint:gosec // env var references in test data, not secrets
func TestApplyTemplate_DevTeam(t *testing.T) {
	tmpl := findTemplate("dev-team")
	assert.NotNil(t, tmpl)

	providers := []wizardProvider{
		{Kind: "anthropic", Name: "claude", APIKey: "${ANTHROPIC_API_KEY}", Model: "claude-sonnet-4-20250514"},
		{Kind: "openai", Name: "gpt", APIKey: "${OPENAI_API_KEY}", Model: "gpt-4o-mini"},
	}
	slotMapping := map[string]string{
		"primary": "claude",
		"fast":    "gpt",
	}

	cfg := applyTemplate(tmpl, providers, slotMapping)

	assert.Len(t, cfg.Agents, 3)
	assert.Equal(t, "orchestrator", cfg.EntryAgent)

	// Verify slot→provider mapping.
	agentByName := make(map[string]wizardAgent, len(cfg.Agents))
	for _, a := range cfg.Agents {
		agentByName[a.Name] = a
	}

	assert.Equal(t, "claude", agentByName["orchestrator"].Provider)
	assert.Equal(t, "claude", agentByName["planner"].Provider)
	assert.Equal(t, "gpt", agentByName["coder"].Provider)

	// Verify coder has compact effect.
	assert.Len(t, agentByName["coder"].Effects, 1)
	assert.Equal(t, "compact", agentByName["coder"].Effects[0].Kind)
	assert.InDelta(t, 0.8, agentByName["coder"].Effects[0].Params["threshold"], 0.001)
}

//nolint:gosec // env var references in test data, not secrets
func TestApplyTemplate_EffectsCarryThrough(t *testing.T) {
	tmpl := findTemplate("dev-team")
	assert.NotNil(t, tmpl)

	providers := []wizardProvider{
		{Kind: "anthropic", Name: "claude", APIKey: "${ANTHROPIC_API_KEY}", Model: "claude-sonnet-4-20250514"},
		{Kind: "openai", Name: "gpt", APIKey: "${OPENAI_API_KEY}", Model: "gpt-4o-mini"},
	}
	slotMapping := map[string]string{
		"primary": "claude",
		"fast":    "gpt",
	}

	cfg := applyTemplate(tmpl, providers, slotMapping)
	data, err := marshalWizardConfig(cfg)
	assert.NoError(t, err)

	// Unmarshal the YAML and verify the effects block survived.
	var parsed configYAML
	assert.NoError(t, yaml.Unmarshal(data, &parsed))

	// Find the coder agent.
	var coderAgent *agentYAML
	for i := range parsed.Agents {
		if parsed.Agents[i].Name == "coder" {
			coderAgent = &parsed.Agents[i]
			break
		}
	}

	assert.NotNil(t, coderAgent, "coder agent should exist in YAML output")
	assert.Len(t, coderAgent.Effects, 1)
	assert.Equal(t, "compact", coderAgent.Effects[0].Kind)
	assert.InDelta(t, 0.8, coderAgent.Effects[0].Params["threshold"], 0.001)

	// Verify agents without effects have no effects key.
	for _, a := range parsed.Agents {
		if a.Name != "coder" {
			assert.Empty(t, a.Effects, "agent %q should have no effects", a.Name)
		}
	}
}

func TestTemplateConsistency(t *testing.T) {
	for _, tmpl := range listTemplates() {
		t.Run(tmpl.Name, func(t *testing.T) {
			// Entry agent must match an agent name.
			agentNames := make(map[string]struct{}, len(tmpl.Agents))
			for _, a := range tmpl.Agents {
				agentNames[a.Name] = struct{}{}
			}
			assert.Contains(t, agentNames, tmpl.EntryAgent, "entry_agent must reference a defined agent")

			// All ProviderSlots must reference a defined slot.
			slotNames := make(map[string]struct{}, len(tmpl.Slots))
			for _, s := range tmpl.Slots {
				slotNames[s.Name] = struct{}{}
			}
			for _, a := range tmpl.Agents {
				assert.Contains(t, slotNames, a.ProviderSlot, "agent %q references unknown slot %q", a.Name, a.ProviderSlot)
			}

			// No duplicate agent names.
			seen := make(map[string]struct{})
			for _, a := range tmpl.Agents {
				assert.NotContains(t, seen, a.Name, "duplicate agent name %q", a.Name)
				seen[a.Name] = struct{}{}
			}
		})
	}
}
