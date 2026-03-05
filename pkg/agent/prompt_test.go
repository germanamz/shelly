package agent

import (
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/skill"
	"github.com/stretchr/testify/assert"
)

func TestPromptBuilderIdentity(t *testing.T) {
	pb := promptBuilder{Name: "bot", Description: "A helpful bot"}
	prompt := pb.build()

	assert.Contains(t, prompt, "<identity>")
	assert.Contains(t, prompt, "You are bot. A helpful bot")
	assert.Contains(t, prompt, "</identity>")
}

func TestPromptBuilderIdentityNoDescription(t *testing.T) {
	pb := promptBuilder{Name: "bot"}
	prompt := pb.build()

	assert.Contains(t, prompt, "You are bot.")
	assert.NotContains(t, prompt, "You are bot. ")
}

func TestPromptBuilderCompletionProtocol(t *testing.T) {
	pb := promptBuilder{Name: "worker", Depth: 1}
	prompt := pb.build()

	assert.Contains(t, prompt, "<completion_protocol>")
	assert.Contains(t, prompt, "task_complete")
}

func TestPromptBuilderNoCompletionProtocolAtTopLevel(t *testing.T) {
	pb := promptBuilder{Name: "bot", Depth: 0}
	prompt := pb.build()

	assert.NotContains(t, prompt, "<completion_protocol>")
}

func TestPromptBuilderNotesProtocol(t *testing.T) {
	pb := promptBuilder{Name: "bot", HasNotesTools: true}
	prompt := pb.build()

	assert.Contains(t, prompt, "<notes_protocol>")
	assert.Contains(t, prompt, "shared notes system")
}

func TestPromptBuilderNoNotesProtocol(t *testing.T) {
	pb := promptBuilder{Name: "bot", HasNotesTools: false}
	prompt := pb.build()

	assert.NotContains(t, prompt, "<notes_protocol>")
}

func TestPromptBuilderInstructions(t *testing.T) {
	pb := promptBuilder{Name: "bot", Instructions: "Be helpful."}
	prompt := pb.build()

	assert.Contains(t, prompt, "<instructions>")
	assert.Contains(t, prompt, "Be helpful.")
}

func TestPromptBuilderBehavioralConstraints(t *testing.T) {
	pb := promptBuilder{Name: "bot"}
	prompt := pb.build()

	assert.Contains(t, prompt, "<behavioral_constraints>")
}

func TestPromptBuilderBehavioralConstraintsDisabled(t *testing.T) {
	pb := promptBuilder{Name: "bot", DisableBehavioralHints: true}
	prompt := pb.build()

	assert.NotContains(t, prompt, "<behavioral_constraints>")
}

func TestPromptBuilderContext(t *testing.T) {
	pb := promptBuilder{Name: "bot", Context: "Go project."}
	prompt := pb.build()

	assert.Contains(t, prompt, "<project_context>")
	assert.Contains(t, prompt, "Go project.")
}

func TestPromptBuilderInlineSkills(t *testing.T) {
	pb := promptBuilder{
		Name:   "bot",
		Skills: []skill.Skill{{Name: "review", Content: "check tests"}},
	}
	prompt := pb.build()

	assert.Contains(t, prompt, "<skills>")
	assert.Contains(t, prompt, "### review")
	assert.Contains(t, prompt, "check tests")
	assert.NotContains(t, prompt, "<available_skills>")
}

func TestPromptBuilderOnDemandSkills(t *testing.T) {
	pb := promptBuilder{
		Name:   "bot",
		Skills: []skill.Skill{{Name: "deploy", Description: "Deployment", Content: "1. Build\n2. Deploy"}},
	}
	prompt := pb.build()

	assert.Contains(t, prompt, "<available_skills>")
	assert.Contains(t, prompt, "**deploy**: Deployment")
	// On-demand skill content should NOT be in the prompt.
	assert.NotContains(t, prompt, "1. Build")
}

func TestPromptBuilderAgentDirectory(t *testing.T) {
	pb := promptBuilder{
		Name:        "orch",
		ConfigName:  "orch",
		CanDelegate: true,
		RegistryEntries: []Entry{
			{Name: "worker", Description: "Does work"},
			{Name: "orch", Description: "Self"},
		},
	}
	prompt := pb.build()

	assert.Contains(t, prompt, "<available_agents>")
	assert.Contains(t, prompt, "**worker**")
	assert.NotContains(t, prompt, "**orch**")
}

func TestPromptBuilderAgentDirectoryWithSkillsAndCost(t *testing.T) {
	pb := promptBuilder{
		Name:        "orch",
		ConfigName:  "orch",
		CanDelegate: true,
		RegistryEntries: []Entry{
			{
				Name:          "coder",
				Description:   "Writes code",
				Skills:        []string{"coding", "testing"},
				EstimatedCost: "medium",
			},
		},
	}
	prompt := pb.build()

	assert.Contains(t, prompt, "[skills: coding, testing]")
	assert.Contains(t, prompt, "[cost: medium]")
}

func TestPromptBuilderAgentDirectoryWithSchemas(t *testing.T) {
	pb := promptBuilder{
		Name:        "orch",
		ConfigName:  "orch",
		CanDelegate: true,
		RegistryEntries: []Entry{
			{
				Name:         "coder",
				Description:  "Writes code",
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"},"files":{"type":"array","items":{"type":"string"}}}}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}}}`),
			},
		},
	}
	prompt := pb.build()

	assert.Contains(t, prompt, "Input: {files: string[], task: string}")
	assert.Contains(t, prompt, "Output: {summary: string}")
}

func TestPromptBuilderAgentDirectoryWithManyProperties(t *testing.T) {
	// >5 properties should truncate with "..."
	pb := promptBuilder{
		Name:        "orch",
		ConfigName:  "orch",
		CanDelegate: true,
		RegistryEntries: []Entry{
			{
				Name:        "complex",
				Description: "Complex agent",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"},"c":{"type":"string"},"d":{"type":"string"},"e":{"type":"string"},"f":{"type":"string"}}}`),
			},
		},
	}
	prompt := pb.build()

	assert.Contains(t, prompt, "...")
	assert.Contains(t, prompt, "Input: {a: string, b: string, c: string, d: string, e: string, ...}")
}

func TestPromptBuilderAgentDirectoryNonObjectSchema(t *testing.T) {
	pb := promptBuilder{
		Name:        "orch",
		ConfigName:  "orch",
		CanDelegate: true,
		RegistryEntries: []Entry{
			{
				Name:        "worker",
				Description: "Does work",
				InputSchema: json.RawMessage(`{"type":"string"}`),
			},
		},
	}
	prompt := pb.build()

	// Non-object schemas fall back to raw JSON.
	assert.Contains(t, prompt, `Input: {"type":"string"}`)
}

func TestPromptBuilderAgentDirectoryNoSchemas(t *testing.T) {
	pb := promptBuilder{
		Name:        "orch",
		ConfigName:  "orch",
		CanDelegate: true,
		RegistryEntries: []Entry{
			{Name: "worker", Description: "Does work"},
		},
	}
	prompt := pb.build()

	assert.Contains(t, prompt, "**worker**: Does work")
	assert.NotContains(t, prompt, "Input:")
	assert.NotContains(t, prompt, "Output:")
	assert.NotContains(t, prompt, "[skills:")
	assert.NotContains(t, prompt, "[cost:")
}

func TestPromptBuilderNoAgentDirectoryWhenCannotDelegate(t *testing.T) {
	pb := promptBuilder{
		Name:        "bot",
		CanDelegate: false,
		RegistryEntries: []Entry{
			{Name: "worker", Description: "Does work"},
		},
	}
	prompt := pb.build()

	assert.NotContains(t, prompt, "<available_agents>")
}
