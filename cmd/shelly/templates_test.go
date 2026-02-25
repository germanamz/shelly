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

	cfg := applyTemplate(tmpl, providers, nil, slotMapping)

	assert.Len(t, cfg.Agents, 1)
	assert.Equal(t, "assistant", cfg.Agents[0].Name)
	assert.Equal(t, "anthropic", cfg.Agents[0].Provider)
	assert.Equal(t, "assistant", cfg.EntryAgent)
	assert.Contains(t, cfg.Agents[0].Toolboxes, "filesystem")
	assert.Contains(t, cfg.Agents[0].Toolboxes, "exec")
	assert.Contains(t, cfg.Agents[0].Toolboxes, "state")
	assert.Empty(t, cfg.Agents[0].Skills, "simple assistant should have no skills filter")
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

	cfg := applyTemplate(tmpl, providers, nil, slotMapping)

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

	// Verify per-agent skills.
	assert.Equal(t, []string{"orchestrator-workflow"}, agentByName["orchestrator"].Skills)
	assert.Equal(t, []string{"planner-workflow"}, agentByName["planner"].Skills)
	assert.Equal(t, []string{"coder-workflow"}, agentByName["coder"].Skills)

	// Verify coder has trim_tool_results + compact effects.
	assert.Len(t, agentByName["coder"].Effects, 2)
	assert.Equal(t, "trim_tool_results", agentByName["coder"].Effects[0].Kind)
	assert.Equal(t, "compact", agentByName["coder"].Effects[1].Kind)
	assert.InDelta(t, 0.8, agentByName["coder"].Effects[1].Params["threshold"], 0.001)
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

	cfg := applyTemplate(tmpl, providers, nil, slotMapping)
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
	assert.Len(t, coderAgent.Effects, 2)
	assert.Equal(t, "trim_tool_results", coderAgent.Effects[0].Kind)
	assert.Equal(t, "compact", coderAgent.Effects[1].Kind)
	assert.InDelta(t, 0.8, coderAgent.Effects[1].Params["threshold"], 0.001)

	// Verify agents without effects have no effects key.
	for _, a := range parsed.Agents {
		if a.Name != "coder" {
			assert.Empty(t, a.Effects, "agent %q should have no effects", a.Name)
		}
	}

	// Verify per-agent skills survive the YAML round-trip.
	agentByName := make(map[string]agentYAML, len(parsed.Agents))
	for _, a := range parsed.Agents {
		agentByName[a.Name] = a
	}
	assert.Equal(t, []string{"orchestrator-workflow"}, agentByName["orchestrator"].Skills)
	assert.Equal(t, []string{"planner-workflow"}, agentByName["planner"].Skills)
	assert.Equal(t, []string{"coder-workflow"}, agentByName["coder"].Skills)
}

//nolint:gosec // env var references in test data, not secrets
func TestApplyTemplate_DevTeamSkillFiles(t *testing.T) {
	tmpl := findTemplate("dev-team")
	assert.NotNil(t, tmpl)

	providers := []wizardProvider{
		{Kind: "anthropic", Name: "claude", APIKey: "${ANTHROPIC_API_KEY}", Model: "claude-sonnet-4-20250514"},
	}
	slotMapping := map[string]string{
		"primary": "claude",
		"fast":    "claude",
	}

	cfg := applyTemplate(tmpl, providers, nil, slotMapping)

	// Verify skill files are carried through.
	assert.Len(t, cfg.SkillFiles, 3)

	skillByName := make(map[string]skillFile, len(cfg.SkillFiles))
	for _, sf := range cfg.SkillFiles {
		skillByName[sf.Name] = sf
	}

	assert.Contains(t, skillByName, "orchestrator-workflow")
	assert.Contains(t, skillByName, "planner-workflow")
	assert.Contains(t, skillByName, "coder-workflow")

	// Each skill file should have frontmatter with a description.
	for _, name := range []string{"orchestrator-workflow", "planner-workflow", "coder-workflow"} {
		assert.Contains(t, skillByName[name].Content, "---\ndescription:", "skill %q should have frontmatter", name)
	}
}

func TestApplyTemplate_SimpleAssistantNoSkillFiles(t *testing.T) {
	tmpl := findTemplate("simple-assistant")
	assert.NotNil(t, tmpl)

	providers := []wizardProvider{
		{Kind: "anthropic", Name: "anthropic", APIKey: "${ANTHROPIC_API_KEY}", Model: "claude-sonnet-4-20250514"}, //nolint:gosec // env var reference, not a secret
	}
	slotMapping := map[string]string{"primary": "anthropic"}

	cfg := applyTemplate(tmpl, providers, nil, slotMapping)
	assert.Empty(t, cfg.SkillFiles, "simple-assistant should not produce skill files")
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

			// All agent skills must reference a skill file in the template.
			skillFileNames := make(map[string]struct{}, len(tmpl.SkillFiles))
			for _, sf := range tmpl.SkillFiles {
				skillFileNames[sf.Name] = struct{}{}
			}
			for _, a := range tmpl.Agents {
				for _, s := range a.Skills {
					assert.Contains(t, skillFileNames, s, "agent %q references skill %q not in SkillFiles", a.Name, s)
				}
			}
		})
	}
}
