package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

type templateProviderSlot struct {
	Name        string
	Description string
}

type templateEffect struct {
	Kind   string
	Params map[string]any
}

type templateAgent struct {
	Name               string
	Description        string
	Instructions       string
	ProviderSlot       string
	MaxIterations      int
	MaxDelegationDepth int
	Toolboxes          []string
	Skills             []string
	Effects            []templateEffect
}

type configTemplate struct {
	Name        string
	Description string
	Slots       []templateProviderSlot
	Agents      []templateAgent
	EntryAgent  string
	SkillFiles  []skillFile // Skill files to create during init.
}

var simpleAssistantTemplate = configTemplate{
	Name:        "simple-assistant",
	Description: "Single agent with all toolboxes — great starting point",
	Slots: []templateProviderSlot{
		{Name: "primary", Description: "The LLM provider for the assistant"},
	},
	Agents: []templateAgent{
		{
			Name:               "assistant",
			Description:        "A helpful assistant",
			Instructions:       "You are a helpful assistant. Be concise and accurate.",
			ProviderSlot:       "primary",
			MaxIterations:      10,
			MaxDelegationDepth: 2,
			Toolboxes:          []string{"filesystem", "exec", "search", "git", "http", "state", "tasks", "notes"},
		},
	},
	EntryAgent: "assistant",
}

var devTeamSkillFiles = []skillFile{
	{
		Name: "orchestrator-workflow",
		Content: `---
description: "Task orchestration protocol: decomposition, delegation, verification, failure recovery"
---
# Orchestrator Workflow

## Before Delegating

1. Create a task on the shared task board (` + "`shared_tasks_create`" + `) for each unit of work.
2. Write a structured task spec as a note (` + "`write_note`" + `) containing:
   - **Objective**: one-sentence goal
   - **Relevant files**: paths the agent should read first
   - **Constraints**: style, performance, or compatibility requirements
   - **Acceptance criteria**: concrete conditions that define "done"
3. Include the note name in the delegation context so the child agent knows where to find it.

## Delegation

- Use ` + "`delegate`" + ` with one or more tasks. All tasks run concurrently. For a single sequential delegation, pass one task; for parallel work, pass multiple.
- Always provide rich ` + "`context`" + ` — include prior decisions, relevant file contents, and the note name.
- Pass ` + "`task_id`" + ` on each task to automatically claim the task for the child agent and update its status based on the child's ` + "`task_complete`" + ` result.

## After Delegation

1. Check the delegation result. If the child called ` + "`task_complete`" + `, the result is structured JSON with ` + "`status`" + `, ` + "`summary`" + `, ` + "`files_modified`" + `, ` + "`tests_run`" + `, and ` + "`caveats`" + ` fields.
2. Read any result notes written by the child agent for additional detail.
3. Verify the result against the acceptance criteria from the task spec.
4. If ` + "`task_id`" + ` was provided during delegation, the task board is auto-updated based on the child's ` + "`task_complete`" + ` result. Otherwise, manually mark the task as completed on the board.

## On Failure or Iteration Exhaustion

Iteration exhaustion returns a structured ` + "`CompletionResult`" + ` with ` + "`status: \"failed\"`" + ` — not an error. This is the same format as a normal ` + "`task_complete`" + ` call.

1. Check the ` + "`caveats`" + ` field for exhaustion details and read any progress notes the child may have written.
2. Diagnose the failure: was the task spec unclear? Was the scope too large?
3. Refine the task spec, split into smaller tasks if needed, and re-delegate with a narrower scope.
4. If repeated failures occur, report the blocker to the user with details.

## State Persistence

- Use notes (` + "`write_note`" + `, ` + "`read_note`" + `, ` + "`list_notes`" + `) to persist cross-delegation state.
- Use the task board to track overall progress across multiple delegations.
`,
	},
	{
		Name: "planner-workflow",
		Content: `---
description: "Planning protocol: analysis, structured plans, note-based handoff"
---
# Planner Workflow

## First Actions

1. Run ` + "`list_notes`" + ` to check for existing context, prior plans, or constraints.
2. Read any relevant notes before starting analysis.

## Creating a Plan

Write the implementation plan as a note (` + "`write_note`" + `) with a descriptive name (e.g., ` + "`plan-add-auth-middleware`" + `). Structure the note as:

- **Goal**: what the change achieves
- **Files to modify**: specific paths and what changes in each
- **Steps**: ordered implementation steps, each concrete and actionable
- **Edge cases**: potential issues, error scenarios, backward compatibility concerns
- **Testing**: what tests to write or run to verify the change

## Complex Multi-Step Plans

For large tasks, create entries on the task board (` + "`shared_tasks_create`" + `) for each major step. This lets the orchestrator track progress and delegate steps independently.

## Handoff

The plan note is the primary handoff artifact. Ensure it contains enough detail that a coder agent can implement without further clarification.
`,
	},
	{
		Name: "coder-workflow",
		Content: `---
description: "Coding protocol: plan consumption, task lifecycle, result reporting"
---
# Coder Workflow

## First Actions

1. Run ` + "`list_notes`" + ` and read any implementation plan or context notes.
2. If not already auto-claimed via ` + "`task_id`" + ` delegation, claim your task on the task board (` + "`shared_tasks_claim`" + `).

## Implementation

- Follow the plan from the notes. If the plan is missing or unclear, write a note explaining the gap before proceeding.
- Make focused, incremental changes. Verify each step works before moving on.
- Run tests after making changes to catch regressions early.

## Result Reporting

After completing work:

1. Write a result note (` + "`write_note`" + `) named descriptively (e.g., ` + "`result-add-auth-middleware`" + `) containing details of what was done.
2. Call ` + "`task_complete`" + ` with:
   - **status**: ` + "`\"completed\"`" + ` or ` + "`\"failed\"`" + `
   - **summary**: concise description of what was done
   - **files_modified**: list of changed files
   - **tests_run**: which tests were executed
   - **caveats**: any known limitations or follow-up work needed

## Task Lifecycle

- When delegated with a ` + "`task_id`" + `, the task board is managed automatically — claiming and status updates happen based on your ` + "`task_complete`" + ` call. You do not need to call ` + "`shared_tasks_claim`" + ` or ` + "`shared_tasks_update`" + ` manually.
- If not delegated with a ` + "`task_id`" + `, mark your task as completed (` + "`shared_tasks_update`" + ` with status ` + "`completed`" + `) when done.
- Always call ` + "`task_complete`" + ` as your final action — do not simply stop responding.

## Approaching Iteration Limit

If you are running low on iterations and cannot finish:

1. Write a progress note documenting what was completed and what remains.
2. Call ` + "`task_complete`" + ` with ` + "`status: \"failed\"`" + `, include a summary of what's done, and describe remaining work in ` + "`caveats`" + `. This gives the orchestrator structured data to decide how to proceed.
`,
	},
}

var devTeamTemplate = configTemplate{
	Name:        "dev-team",
	Description: "Orchestrator + planner + coder — multi-agent dev workflow",
	Slots: []templateProviderSlot{
		{Name: "primary", Description: "Main LLM for orchestration and planning"},
		{Name: "fast", Description: "Fast LLM for coding tasks"},
	},
	Agents: []templateAgent{
		{
			Name:               "orchestrator",
			Description:        "Delegates tasks to planner and coder",
			Instructions:       "You are a task orchestrator. Break down user requests into planning and coding tasks, then delegate to the appropriate agent.",
			ProviderSlot:       "primary",
			MaxIterations:      15,
			MaxDelegationDepth: 3,
			Toolboxes:          []string{"state", "tasks", "notes"},
			Skills:             []string{"orchestrator-workflow"},
		},
		{
			Name:               "planner",
			Description:        "Analyzes code and creates implementation plans",
			Instructions:       "You are a code planner. Analyze the codebase, understand the architecture, and create detailed implementation plans.",
			ProviderSlot:       "primary",
			MaxIterations:      10,
			MaxDelegationDepth: 0,
			Toolboxes:          []string{"filesystem", "search", "git", "state", "tasks", "notes"},
			Skills:             []string{"planner-workflow"},
		},
		{
			Name:               "coder",
			Description:        "Implements code changes",
			Instructions:       "You are a coder. Implement changes based on plans, write clean code, and run tests to verify your work.",
			ProviderSlot:       "fast",
			MaxIterations:      20,
			MaxDelegationDepth: 0,
			Toolboxes:          []string{"filesystem", "exec", "search", "git", "http", "state", "tasks", "notes"},
			Skills:             []string{"coder-workflow"},
			Effects: []templateEffect{
				{Kind: "trim_tool_results"},
				{Kind: "compact", Params: map[string]any{"threshold": 0.8}},
			},
		},
	},
	EntryAgent: "orchestrator",
	SkillFiles: devTeamSkillFiles,
}

var templates = []configTemplate{simpleAssistantTemplate, devTeamTemplate}

func findTemplate(name string) *configTemplate {
	for i := range templates {
		if templates[i].Name == name {
			return &templates[i]
		}
	}

	return nil
}

func listTemplates() []configTemplate {
	return templates
}

func printTemplateList() {
	fmt.Println("Available templates:")
	fmt.Println()

	for _, t := range templates {
		fmt.Printf("  %s\n", t.Name)
		fmt.Printf("    %s\n", t.Description)

		fmt.Printf("    Slots:  ")
		for i, s := range t.Slots {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s (%s)", s.Name, s.Description)
		}
		fmt.Println()

		fmt.Printf("    Agents: ")
		for i, a := range t.Agents {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(a.Name)
		}
		fmt.Println()
		fmt.Println()
	}
}

func wizardSlotMapping(slots []templateProviderSlot, providerNames []string) (map[string]string, error) {
	mapping := make(map[string]string, len(slots))

	// Auto-map when there's exactly 1 slot and 1 provider.
	if len(slots) == 1 && len(providerNames) == 1 {
		mapping[slots[0].Name] = providerNames[0]
		return mapping, nil
	}

	opts := make([]huh.Option[string], len(providerNames))
	for i, n := range providerNames {
		opts[i] = huh.NewOption(n, n)
	}

	for _, slot := range slots {
		var selected string
		if len(providerNames) > 0 {
			selected = providerNames[0]
		}

		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Provider for %q slot (%s)", slot.Name, slot.Description)).
				Options(opts...).
				Value(&selected),
		)).Run(); err != nil {
			return nil, err
		}

		mapping[slot.Name] = selected
	}

	return mapping, nil
}

func applyTemplate(tmpl *configTemplate, providers []wizardProvider, slotMapping map[string]string) wizardConfig {
	cfg := wizardConfig{
		Providers:  providers,
		EntryAgent: tmpl.EntryAgent,
		SkillFiles: tmpl.SkillFiles,
	}

	for _, ta := range tmpl.Agents {
		a := wizardAgent{
			Name:               ta.Name,
			Description:        ta.Description,
			Instructions:       ta.Instructions,
			Provider:           slotMapping[ta.ProviderSlot],
			MaxIterations:      ta.MaxIterations,
			MaxDelegationDepth: ta.MaxDelegationDepth,
			Toolboxes:          ta.Toolboxes,
			Skills:             ta.Skills,
		}

		for _, te := range ta.Effects {
			a.Effects = append(a.Effects, wizardEffect(te))
		}

		cfg.Agents = append(cfg.Agents, a)
	}

	return cfg
}

func runTemplateWizard(tmpl *configTemplate) (wizardResult, error) {
	var cfg wizardConfig

	if err := wizardProviders(&cfg); err != nil {
		return wizardResult{}, err
	}

	providerNames := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		providerNames[i] = p.Name
	}

	slotMapping, err := wizardSlotMapping(tmpl.Slots, providerNames)
	if err != nil {
		return wizardResult{}, err
	}

	applied := applyTemplate(tmpl, cfg.Providers, slotMapping)

	data, err := marshalWizardConfig(applied)
	if err != nil {
		return wizardResult{}, err
	}

	return wizardResult{
		ConfigYAML: data,
		SkillFiles: applied.SkillFiles,
	}, nil
}
