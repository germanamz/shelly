package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Store holds loaded skills and exposes a load_skill tool for on-demand
// retrieval of skill content by agents.
type Store struct {
	skills map[string]Skill
}

// NewStore creates a Store from the given skills.
func NewStore(skills []Skill) *Store {
	m := make(map[string]Skill, len(skills))
	for _, s := range skills {
		m[s.Name] = s
	}
	return &Store{skills: m}
}

// Get returns the skill with the given name and whether it was found.
func (st *Store) Get(name string) (Skill, bool) {
	s, ok := st.skills[name]
	return s, ok
}

// Skills returns all skills sorted by name.
func (st *Store) Skills() []Skill {
	result := make([]Skill, 0, len(st.skills))
	for _, s := range st.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Tools returns a ToolBox containing the load_skill tool.
func (st *Store) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(toolbox.Tool{
		Name:        "load_skill",
		Description: "Load the full content of a skill by name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Name of the skill to load"}},"required":["name"]}`),
		Handler:     st.handleLoadSkill,
	})
	return tb
}

type loadSkillInput struct {
	Name string `json:"name"`
}

func (st *Store) handleLoadSkill(_ context.Context, input json.RawMessage) (string, error) {
	var in loadSkillInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	s, ok := st.skills[in.Name]
	if !ok {
		return "", fmt.Errorf("skill not found: %s", in.Name)
	}

	return s.Content, nil
}
