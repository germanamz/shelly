package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/germanamz/shelly/pkg/skill"
)

// promptBuilder assembles a system prompt from agent configuration fields.
// It is a pure value type with no mutation or side effects.
type promptBuilder struct {
	Name, Description, Instructions, Context, ConfigName string
	Depth                                                int
	Skills                                               []skill.Skill
	DisableBehavioralHints                               bool
	HasNotesTools                                        bool
	CanDelegate                                          bool
	RegistryEntries                                      []Entry
}

// build constructs the system prompt string.
//
// Sections are ordered for prompt-cache friendliness: static content first
// (identity, instructions), semi-static content next (project context, skills),
// and dynamic content last (agent directory). Each section uses XML tags so
// LLMs can attend to boundaries without relying on prose structure.
func (pb *promptBuilder) build() string {
	var b strings.Builder

	// --- Static content (rarely changes, cacheable prefix) ---

	// Identity.
	b.WriteString("<identity>\n")
	fmt.Fprintf(&b, "You are %s.", pb.Name)
	if pb.Description != "" {
		fmt.Fprintf(&b, " %s", pb.Description)
	}
	b.WriteString("\n</identity>\n")

	// Completion protocol (sub-agents only).
	if pb.Depth > 0 {
		b.WriteString("\n<completion_protocol>\n")
		b.WriteString("You are a sub-agent executing a delegated task. ")
		b.WriteString("When you finish, you MUST call the task_complete tool with:\n")
		b.WriteString("- status: \"completed\" or \"failed\"\n")
		b.WriteString("- summary: concise description of what was done\n")
		b.WriteString("- files_modified, tests_run, caveats: as applicable\n")
		b.WriteString("Do NOT simply stop responding — always call task_complete.\n")
		b.WriteString("If you sense you are running low on iterations and cannot finish, ")
		b.WriteString("call task_complete with status \"failed\", summarize what was done, ")
		b.WriteString("and describe remaining work in caveats. Write a progress note first.\n")
		b.WriteString("</completion_protocol>\n")
	}

	// Notes protocol (only when notes tools are available).
	if pb.HasNotesTools {
		b.WriteString("\n<notes_protocol>\n")
		b.WriteString("A shared notes system is available for durable cross-agent communication.\n")
		b.WriteString("Notes persist across agent boundaries and context compaction.\n")
		b.WriteString("When you expect context from another agent (plans, task specs, prior results), ")
		b.WriteString("use list_notes and read_note to retrieve it.\n")
		b.WriteString("When you complete significant work, use write_note to document results ")
		b.WriteString("so other agents can pick up where you left off.\n")
		b.WriteString("</notes_protocol>\n")
	}

	// Instructions.
	if pb.Instructions != "" {
		b.WriteString("\n<instructions>\n")
		b.WriteString(pb.Instructions)
		b.WriteString("\n</instructions>\n")
	}

	// Behavioral constraints (default on, can be disabled).
	if !pb.DisableBehavioralHints {
		b.WriteString("\n<behavioral_constraints>\n")
		b.WriteString("- When a file operation fails, verify the path exists before retrying.\n")
		b.WriteString("- After a tool failure, analyze the error and change your approach before retrying. Do not repeat the same action more than twice expecting different results.\n")
		b.WriteString("- If you have made 5+ tool calls without visible progress, stop and reassess your approach.\n")
		b.WriteString("- Read files before editing them. Search before assuming file locations.\n")
		b.WriteString("- When a command errors, read the error message carefully and address the root cause.\n")
		b.WriteString("- Prefer targeted edits over full file rewrites to minimize unintended changes.\n")
		b.WriteString("- Before starting a multi-step task, briefly outline your plan and the order of steps.\n")
		b.WriteString("- When you have multiple tools that could work, prefer the most specific one for the task.\n")
		b.WriteString("- When approaching your iteration limit, prioritize completing the most critical remaining work and write a progress note.\n")
		b.WriteString("</behavioral_constraints>\n")
	}

	// --- Semi-static content (loaded once at startup) ---

	// Project context.
	if pb.Context != "" {
		b.WriteString("\n<project_context>\n")
		b.WriteString("The following is context about the project you are working in. ")
		b.WriteString("Treat this as your own knowledge — do not say you lack context about the project. ")
		b.WriteString("Use this information to guide your responses and actions.\n\n")
		b.WriteString(pb.Context)
		b.WriteString("\n</project_context>\n")
	}

	// Skills — split into inline (no description) and on-demand (has description).
	var inline, onDemand []skill.Skill
	for _, s := range pb.Skills {
		if s.HasDescription() {
			onDemand = append(onDemand, s)
		} else {
			inline = append(inline, s)
		}
	}

	if len(inline) > 0 {
		b.WriteString("\n<skills>\n")
		for _, s := range inline {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n", s.Name, s.Content)
		}
		b.WriteString("</skills>\n")
	}

	if len(onDemand) > 0 {
		b.WriteString("\n<available_skills>\n")
		b.WriteString("Use the load_skill tool to retrieve the full content of a skill when needed.\n")
		for _, s := range onDemand {
			fmt.Fprintf(&b, "- **%s**: %s\n", s.Name, s.Description)
		}
		b.WriteString("</available_skills>\n")
	}

	// --- Dynamic content (changes per session, not cacheable) ---

	// Agent directory from registry (only when delegation is possible).
	if pb.CanDelegate {
		var others []Entry
		for _, e := range pb.RegistryEntries {
			if e.Name != pb.ConfigName {
				others = append(others, e)
			}
		}

		if len(others) > 0 {
			b.WriteString("\n<available_agents>\n")
			for _, e := range others {
				fmt.Fprintf(&b, "- **%s**: %s", e.Name, e.Description)
				if len(e.Skills) > 0 {
					fmt.Fprintf(&b, " [skills: %s]", strings.Join(e.Skills, ", "))
				}
				if e.EstimatedCost != "" {
					fmt.Fprintf(&b, " [cost: %s]", e.EstimatedCost)
				}
				b.WriteString("\n")
				if e.InputSchema != nil {
					fmt.Fprintf(&b, "  Input: %s\n", compactSchemaString(e.InputSchema))
				}
				if e.OutputSchema != nil {
					fmt.Fprintf(&b, "  Output: %s\n", compactSchemaString(e.OutputSchema))
				}
			}
			b.WriteString("</available_agents>\n")
		}
	}

	return b.String()
}

// compactSchemaString renders a JSON Schema as a compact type signature.
// For object schemas with properties, it shows "{prop: type, ...}".
// For schemas with >5 properties, only the first 5 are shown with "...".
// Falls back to the raw JSON for non-object or unparseable schemas.
func compactSchemaString(raw json.RawMessage) string {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return string(raw)
	}

	typ, _ := schema["type"].(string)
	if typ != "object" {
		s := string(raw)
		const maxFallback = 200
		if len(s) > maxFallback {
			s = s[:maxFallback] + "..."
		}
		return s
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	const maxProps = 5
	var parts []string
	for i, k := range keys {
		if i >= maxProps {
			parts = append(parts, "...")
			break
		}
		propType := "any"
		if propMap, ok := props[k].(map[string]any); ok {
			if t, ok := propMap["type"].(string); ok {
				if t == "array" {
					if items, ok := propMap["items"].(map[string]any); ok {
						if it, ok := items["type"].(string); ok {
							t = it + "[]"
						}
					}
				}
				propType = t
			}
		}
		parts = append(parts, fmt.Sprintf("%s: %s", k, propType))
	}

	return "{" + strings.Join(parts, ", ") + "}"
}
